package mux

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/developer-nagi/codex-subscription-router-win/internal/state"
)

var errNoSubscriptionCapacity = errors.New("no enabled ChatGPT subscription has capacity")

const (
	routingFallbackWindow      = 7 * 24 * time.Hour
	routingMinimumWindow       = time.Minute
	routingResetBonusPerCredit = 0.15
	routingResetBonusCreditCap = 3
)

type RateLimitWindow struct {
	UsedPercent        float64 `json:"usedPercent"`
	WindowDurationMins *int64  `json:"windowDurationMins"`
	ResetsAt           *int64  `json:"resetsAt"`
}

type RateLimits struct {
	Primary              *RateLimitWindow `json:"primary"`
	Secondary            *RateLimitWindow `json:"secondary"`
	RateLimitReachedType any              `json:"rateLimitReachedType"`
}

type AccountSnapshot struct {
	ID              string          `json:"id"`
	Label           string          `json:"label"`
	Enabled         bool            `json:"enabled"`
	Controller      bool            `json:"controller"`
	Connected       bool            `json:"connected"`
	Email           string          `json:"email,omitempty"`
	PlanType        string          `json:"planType,omitempty"`
	PlanLabel       string          `json:"planLabel,omitempty"`
	AuthType        string          `json:"authType,omitempty"`
	ProfileImageURL string          `json:"profileImageUrl,omitempty"`
	RateLimits      *RateLimits     `json:"rateLimits,omitempty"`
	ThreadCount     int             `json:"threadCount"`
	Error           string          `json:"error,omitempty"`
	CreatedAt       int64           `json:"createdAt"`
	RawAccount      json.RawMessage `json:"-"`
}

type RouteReason struct {
	WeeklyUsedPercent    *float64 `json:"weeklyUsedPercent"`
	WeeklyResetsAt       *int64   `json:"weeklyResetsAt,omitempty"`
	ShortUsedPercent     *float64 `json:"shortUsedPercent"`
	BankedResetCount     *int     `json:"bankedResetCount,omitempty"`
	ResetCreditExpiresAt *int64   `json:"resetCreditExpiresAt,omitempty"`
	UrgencyScore         *float64 `json:"urgencyScore,omitempty"`
	ThreadCount          int      `json:"threadCount"`
}

func (m *Multiplexer) Accounts(ctx context.Context) []AccountSnapshot {
	return m.accountSnapshots(ctx, true)
}

func (m *Multiplexer) accountSnapshots(ctx context.Context, includeProfile bool) []AccountSnapshot {
	accounts := m.store.Accounts()
	results := make(chan AccountSnapshot, len(accounts))
	for _, account := range accounts {
		go func(account state.Account) {
			snapshot, err := m.accountSnapshotWithProfile(ctx, account.ID, includeProfile)
			if err != nil {
				snapshot = AccountSnapshot{
					ID: account.ID, Label: account.Label, Enabled: account.Enabled,
					Controller: account.Controller, CreatedAt: account.CreatedAt, Error: err.Error(),
				}
			}
			results <- snapshot
		}(account)
	}
	snapshots := make([]AccountSnapshot, 0, len(accounts))
	for range accounts {
		select {
		case snapshot := <-results:
			snapshots = append(snapshots, snapshot)
		case <-ctx.Done():
			return snapshots
		}
	}
	sort.SliceStable(snapshots, func(i, j int) bool {
		if snapshots[i].Controller != snapshots[j].Controller {
			return snapshots[i].Controller
		}
		return snapshots[i].CreatedAt < snapshots[j].CreatedAt
	})
	return snapshots
}

func (m *Multiplexer) AddAccount(ctx context.Context, label string) (AccountSnapshot, error) {
	account, err := m.store.AddAccount(label)
	if err != nil {
		return AccountSnapshot{}, err
	}
	if _, err := m.startChild(ctx, account); err != nil {
		return AccountSnapshot{}, err
	}
	return m.accountSnapshot(ctx, account.ID)
}

func (m *Multiplexer) UpdateAccount(ctx context.Context, id string, label *string, enabled *bool) (AccountSnapshot, error) {
	if _, err := m.store.UpdateAccount(id, label, enabled); err != nil {
		return AccountSnapshot{}, err
	}
	return m.accountSnapshot(ctx, id)
}

// RemoveAccount deletes a subscription from the routing pool: its app-server
// child is stopped, thread ownership is cleared, and the isolated Codex home
// is moved into a timestamped backup so the removal stays recoverable. The
// Primary subscription cannot be removed. Order matters: every step before
// the state commit is reversible, and the commit is what makes it permanent.
func (m *Multiplexer) RemoveAccount(ctx context.Context, id string) error {
	account, ok := m.store.Account(id)
	if !ok {
		return fmt.Errorf("account %q not found", id)
	}
	if account.Controller {
		return errors.New("the Primary subscription cannot be removed")
	}
	if child, ok := m.child(id); ok {
		_ = child.Close()
		// Windows cannot move a directory that a running process still holds.
		// Close only asks the process to stop, so wait until it actually exits.
		waitCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		_ = child.Wait(waitCtx)
		cancel()
		m.childrenMu.Lock()
		delete(m.children, id)
		m.childrenMu.Unlock()
	}
	if err := backupAccountHome(m.store.Root(), account); err != nil {
		return fmt.Errorf("back up subscription home: %w", err)
	}
	if _, err := m.store.RemoveAccount(id); err != nil {
		return err
	}
	m.publish(Event{Type: "account-removed", AccountID: id, Message: account.Label})
	return nil
}

// backupAccountHome moves accounts/<id> (the isolated Codex home and any
// sibling state) under backups/accounts/<id>-<unix-seconds>. Missing
// directories are ignored: nothing was created, so nothing is lost.
func backupAccountHome(root string, account state.Account) error {
	source := filepath.Dir(account.CodexHome)
	if _, err := os.Stat(source); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	backupRoot := filepath.Join(root, "backups", "accounts")
	if err := os.MkdirAll(backupRoot, 0o700); err != nil {
		return fmt.Errorf("create account backup root: %w", err)
	}
	destination := filepath.Join(backupRoot, fmt.Sprintf("%s-%d", account.ID, time.Now().Unix()))
	if err := renameWithRetry(source, destination); err != nil {
		return fmt.Errorf("move account home: %w", err)
	}
	return nil
}

// renameWithRetry works around Windows releasing file handles asynchronously
// after a process exits. A directory that a just-stopped Codex child owned can
// stay locked briefly, so retry before giving up. On other platforms the first
// attempt succeeds and the loop costs nothing.
func renameWithRetry(source, destination string) error {
	var err error
	for attempt := 0; attempt < 10; attempt++ {
		if err = os.Rename(source, destination); err == nil {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return err
}

// ThreadAccount reports the subscription a chat belongs to, which is the one that
// answers for it: a chat is never moved between subscriptions.
func (m *Multiplexer) ThreadAccount(ctx context.Context, threadID string) (AccountSnapshot, error) {
	accountID, ok := m.store.ThreadOwner(threadID)
	if !ok {
		return AccountSnapshot{}, fmt.Errorf("thread %q has no subscription assignment", threadID)
	}
	return m.accountSnapshotWithProfile(ctx, accountID, true)
}

func (m *Multiplexer) StartLogin(ctx context.Context, id, mode string) (json.RawMessage, error) {
	if mode != "chatgpt" && mode != "chatgptDeviceCode" {
		return nil, errors.New("login mode must be chatgpt or chatgptDeviceCode")
	}
	child, ok := m.child(id)
	if !ok {
		return nil, fmt.Errorf("account %q is unavailable", id)
	}
	params, _ := json.Marshal(map[string]any{"type": mode})
	response, err := child.Request(ctx, "account/login/start", params)
	if err != nil {
		return nil, err
	}
	return response.Result, nil
}

func (m *Multiplexer) Logout(ctx context.Context, id string) error {
	child, ok := m.child(id)
	if !ok {
		return fmt.Errorf("account %q is unavailable", id)
	}
	_, err := child.Request(ctx, "account/logout", nil)
	return err
}

func (m *Multiplexer) accountSnapshot(ctx context.Context, accountID string) (AccountSnapshot, error) {
	return m.accountSnapshotWithProfile(ctx, accountID, true)
}

func (m *Multiplexer) accountSnapshotWithProfile(ctx context.Context, accountID string, includeProfile bool) (AccountSnapshot, error) {
	account, ok := m.store.Account(accountID)
	if !ok {
		return AccountSnapshot{}, fmt.Errorf("account %q not found", accountID)
	}
	child, ok := m.child(accountID)
	if !ok {
		return AccountSnapshot{}, fmt.Errorf("account %q app-server is unavailable", accountID)
	}
	params := json.RawMessage(`{"refreshToken":false}`)
	accountResponse, err := child.Request(ctx, "account/read", params)
	if err != nil {
		return AccountSnapshot{}, err
	}
	var accountResult struct {
		Account json.RawMessage `json:"account"`
	}
	if err := json.Unmarshal(accountResponse.Result, &accountResult); err != nil {
		return AccountSnapshot{}, fmt.Errorf("decode account response: %w", err)
	}
	snapshot := AccountSnapshot{
		ID: account.ID, Label: account.Label, Enabled: account.Enabled,
		Controller: account.Controller, Connected: string(accountResult.Account) != "null" && len(accountResult.Account) > 0,
		CreatedAt: account.CreatedAt, RawAccount: accountResult.Account,
		ThreadCount: m.store.ThreadCounts()[account.ID],
	}
	if snapshot.Connected {
		var details struct {
			Type     string `json:"type"`
			Email    string `json:"email"`
			PlanType string `json:"planType"`
		}
		_ = json.Unmarshal(accountResult.Account, &details)
		snapshot.AuthType = details.Type
		snapshot.Email = details.Email
		snapshot.PlanType = details.PlanType
		snapshot.PlanLabel = planLabel(details.PlanType)
		if includeProfile {
			snapshot.ProfileImageURL = m.profileImageURL(ctx, account)
		}
		if details.Type == "chatgpt" {
			rateResponse, rateErr := child.Request(ctx, "account/rateLimits/read", nil)
			if rateErr == nil {
				var rateResult struct {
					RateLimits RateLimits `json:"rateLimits"`
				}
				if json.Unmarshal(rateResponse.Result, &rateResult) == nil {
					snapshot.RateLimits = &rateResult.RateLimits
				}
			}
		}
	}
	m.applyRateLimitPreview(&snapshot)
	return snapshot, nil
}

func planLabel(planType string) string {
	switch planType {
	case "free":
		return "Free"
	case "go":
		return "Go"
	case "plus":
		return "Plus"
	case "prolite":
		return "Pro 5x"
	case "pro":
		return "Pro 20x"
	case "team":
		return "Team"
	case "self_serve_business_prolite", "self_serve_business_usage_based", "business":
		return "Business"
	case "ent26", "enterprise_cbp_automation", "enterprise_cbp_usage_based", "enterprise":
		return "Enterprise"
	case "edu":
		return "Edu"
	default:
		return ""
	}
}

func (m *Multiplexer) chooseAccount(ctx context.Context) (state.Account, RouteReason, error) {
	return m.chooseAccountExcluding(ctx, nil)
}

func (m *Multiplexer) chooseAccountExcluding(ctx context.Context, excluded map[string]struct{}) (state.Account, RouteReason, error) {
	snapshots := m.accountSnapshots(ctx, false)
	type candidate struct {
		account      state.Account
		reason       RouteReason
		weekly       *RateLimitWindow
		weeklyUsed   float64
		shortUsed    float64
		resetCredits resetCreditMetadata
		urgency      float64
	}
	candidates := make([]candidate, 0, len(snapshots))
	for _, snapshot := range snapshots {
		if _, skip := excluded[snapshot.ID]; skip {
			continue
		}
		if !snapshot.Enabled || !snapshot.Connected || snapshot.AuthType != "chatgpt" {
			continue
		}
		account, ok := m.store.Account(snapshot.ID)
		if !ok {
			continue
		}
		weekly, short := longestAndShortestWindow(snapshot.RateLimits)
		if weekly != nil && weekly.UsedPercent >= 100 {
			continue
		}
		weeklyUsed := 1_000.0
		shortUsed := 1_000.0
		reason := RouteReason{ThreadCount: snapshot.ThreadCount}
		if weekly != nil {
			weeklyUsed = weekly.UsedPercent
			reason.WeeklyUsedPercent = &weekly.UsedPercent
			if weekly.ResetsAt != nil {
				value := *weekly.ResetsAt
				reason.WeeklyResetsAt = &value
			}
		}
		if short != nil {
			shortUsed = short.UsedPercent
			reason.ShortUsedPercent = &short.UsedPercent
		}
		candidates = append(candidates, candidate{
			account: account, reason: reason, weekly: weekly,
			weeklyUsed: weeklyUsed, shortUsed: shortUsed,
		})
	}
	if len(candidates) == 0 {
		return state.Account{}, RouteReason{}, errNoSubscriptionCapacity
	}

	type resetResult struct {
		index    int
		metadata resetCreditMetadata
	}
	resetResults := make(chan resetResult, len(candidates))
	for index := range candidates {
		go func(index int, account state.Account) {
			resetResults <- resetResult{
				index: index, metadata: m.routingResetCredits(ctx, account),
			}
		}(index, candidates[index].account)
	}

collectResetCredits:
	for received := 0; received < len(candidates); received++ {
		select {
		case result := <-resetResults:
			candidates[result.index].resetCredits = result.metadata
		case <-ctx.Done():
			break collectResetCredits
		}
	}

	now := m.now()
	for index := range candidates {
		entry := &candidates[index]
		entry.urgency = routeUrgencyScore(now, entry.weekly, entry.resetCredits)
		urgency := entry.urgency
		entry.reason.UrgencyScore = &urgency
		if entry.resetCredits.Known {
			count := entry.resetCredits.AvailableCount
			entry.reason.BankedResetCount = &count
		}
		if entry.resetCredits.EarliestExpiry != nil {
			expiresAt := *entry.resetCredits.EarliestExpiry
			entry.reason.ResetCreditExpiresAt = &expiresAt
		}
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		left, right := candidates[i], candidates[j]
		if math.Abs(left.urgency-right.urgency) > 0.000001 {
			return left.urgency > right.urgency
		}
		if math.Abs(left.shortUsed-right.shortUsed) > 0.001 {
			return left.shortUsed < right.shortUsed
		}
		if math.Abs(left.weeklyUsed-right.weeklyUsed) > 0.001 {
			return left.weeklyUsed < right.weeklyUsed
		}
		if left.reason.ThreadCount != right.reason.ThreadCount {
			return left.reason.ThreadCount < right.reason.ThreadCount
		}
		return left.account.CreatedAt < right.account.CreatedAt
	})
	return candidates[0].account, candidates[0].reason, nil
}

func routeUrgencyScore(now time.Time, weekly *RateLimitWindow, credits resetCreditMetadata) float64 {
	if weekly == nil {
		return -1
	}
	remaining := math.Max(0, math.Min(100, 100-weekly.UsedPercent))
	horizon := routingFallbackWindow
	if weekly.WindowDurationMins != nil && *weekly.WindowDurationMins > 0 {
		horizon = time.Duration(*weekly.WindowDurationMins) * time.Minute
	}
	if weekly.ResetsAt != nil {
		untilReset := time.Unix(*weekly.ResetsAt, 0).Sub(now)
		if untilReset > 0 {
			horizon = untilReset
		}
	}
	if horizon < routingMinimumWindow {
		horizon = routingMinimumWindow
	}
	urgency := remaining / horizon.Hours()
	if credits.Known && credits.AvailableCount > 0 {
		creditCount := min(credits.AvailableCount, routingResetBonusCreditCap)
		urgency *= 1 + float64(creditCount)*routingResetBonusPerCredit
	}
	return urgency
}

func (m *Multiplexer) AggregatedRateLimits(ctx context.Context) (*RateLimits, error) {
	limits, err := aggregateRateLimits(m.accountSnapshots(ctx, false))
	if err != nil {
		return nil, err
	}
	if preview := m.currentRateLimitPreview(); preview != nil && preview.Mode.isAllDepleted() {
		limits.RateLimitReachedType = "legacy_rate_limit_reached"
	}
	return limits, nil
}

func aggregateRateLimits(snapshots []AccountSnapshot) (*RateLimits, error) {
	primary := make([]*RateLimitWindow, 0, len(snapshots))
	secondary := make([]*RateLimitWindow, 0, len(snapshots))
	hasSubscription := false
	hasCapacity := false
	for _, snapshot := range snapshots {
		if !snapshot.Enabled || !snapshot.Connected || snapshot.AuthType != "chatgpt" {
			continue
		}
		hasSubscription = true
		if snapshot.RateLimits != nil {
			primary = append(primary, snapshot.RateLimits.Primary)
			secondary = append(secondary, snapshot.RateLimits.Secondary)
		}
		weekly, _ := longestAndShortestWindow(snapshot.RateLimits)
		if weekly == nil || weekly.UsedPercent < 100 {
			hasCapacity = true
		}
	}
	if !hasSubscription {
		return nil, errors.New("no enabled ChatGPT subscription is connected")
	}
	result := &RateLimits{
		Primary:   averageRateLimitWindow(primary),
		Secondary: averageRateLimitWindow(secondary),
	}
	if !hasCapacity {
		result.RateLimitReachedType = "rate_limit_reached"
	}
	return result, nil
}

func averageRateLimitWindow(windows []*RateLimitWindow) *RateLimitWindow {
	var used float64
	var count int
	var longestDuration *int64
	var earliestReset *int64
	for _, window := range windows {
		if window == nil {
			continue
		}
		used += window.UsedPercent
		count++
		if window.WindowDurationMins != nil &&
			(longestDuration == nil || *window.WindowDurationMins > *longestDuration) {
			value := *window.WindowDurationMins
			longestDuration = &value
		}
		if window.ResetsAt != nil && (earliestReset == nil || *window.ResetsAt < *earliestReset) {
			value := *window.ResetsAt
			earliestReset = &value
		}
	}
	if count == 0 {
		return nil
	}
	return &RateLimitWindow{
		UsedPercent:        used / float64(count),
		WindowDurationMins: longestDuration,
		ResetsAt:           earliestReset,
	}
}

func longestAndShortestWindow(limits *RateLimits) (*RateLimitWindow, *RateLimitWindow) {
	if limits == nil {
		return nil, nil
	}
	windows := make([]*RateLimitWindow, 0, 2)
	if limits.Primary != nil {
		windows = append(windows, limits.Primary)
	}
	if limits.Secondary != nil {
		windows = append(windows, limits.Secondary)
	}
	if len(windows) == 0 {
		return nil, nil
	}
	sort.SliceStable(windows, func(i, j int) bool {
		return duration(windows[i]) < duration(windows[j])
	})
	return windows[len(windows)-1], windows[0]
}

func duration(window *RateLimitWindow) int64 {
	if window.WindowDurationMins == nil {
		return 0
	}
	return *window.WindowDurationMins
}

func contextWithControlTimeout(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, 20*time.Second)
}
