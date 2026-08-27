package mux

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/developer-nagi/codex-subscription-router-win/internal/backend"
	"github.com/developer-nagi/codex-subscription-router-win/internal/protocol"
	"github.com/developer-nagi/codex-subscription-router-win/internal/state"
)

const requestTimeout = 30 * time.Second

// Moving a chat reads its history twice over - once to count its turns where it is,
// once to rebuild them where it is going. It runs in the background and holds nothing
// up, so it is given room to finish rather than a request-sized budget.
const handoverTimeout = 15 * time.Minute

type Options struct {
	RealExecutable string
	RealArgs       []string
	Environment    []string
	Store          *state.Store
	Output         io.Writer
}

type externalRoute struct {
	accountID string
	method    string
	message   protocol.Message
	excluded  map[string]struct{}
}

type serverRequestRoute struct {
	accountID string
	original  json.RawMessage
}

type Event struct {
	Type      string `json:"type"`
	AccountID string `json:"accountId,omitempty"`
	Message   string `json:"message,omitempty"`
	Data      any    `json:"data,omitempty"`
}

// Multiplexer presents one app-server connection to ChatGPT.app while owning
// one real app-server process per ChatGPT subscription.
type Multiplexer struct {
	realExecutable string
	realArgs       []string
	environment    []string
	store          *state.Store
	output         io.Writer

	childrenMu sync.RWMutex
	children   map[string]*backend.Child
	inbound    chan backend.Inbound

	initializationMu sync.RWMutex
	initializeParams json.RawMessage
	initialized      bool

	externalMu          sync.Mutex
	externalRoutes      map[string]externalRoute
	turnsMu             sync.Mutex
	inFlightTurns       map[string]inFlightTurn
	withheldCompletions map[string][]byte
	turnHosts           map[string]string
	serverMu            sync.Mutex
	serverRoutes        map[string]serverRequestRoute
	serverSequence      atomic.Uint64

	outputMu sync.Mutex
	eventsMu sync.RWMutex
	events   map[chan Event]struct{}

	profileMu     sync.Mutex
	profileClient *http.Client
	profileCache  map[string]profileCacheEntry
	now           func() time.Time

	resetCreditsMu       sync.Mutex
	resetCreditsCache    map[string]resetCreditsCacheEntry
	resetCreditsEndpoint string

	previewMu        sync.RWMutex
	rateLimitPreview *RateLimitPreview

	resetPreviewMu sync.RWMutex
	resetPreviews  map[string]ResetCreditsPreview
}

func New(options Options) (*Multiplexer, error) {
	if options.RealExecutable == "" || options.Store == nil || options.Output == nil {
		return nil, errors.New("real executable, store, and output are required")
	}
	return &Multiplexer{
		realExecutable:       options.RealExecutable,
		realArgs:             append([]string(nil), options.RealArgs...),
		environment:          append([]string(nil), options.Environment...),
		store:                options.Store,
		output:               options.Output,
		children:             make(map[string]*backend.Child),
		inbound:              make(chan backend.Inbound, 1024),
		externalRoutes:       make(map[string]externalRoute),
		inFlightTurns:        make(map[string]inFlightTurn),
		withheldCompletions:  make(map[string][]byte),
		turnHosts:            make(map[string]string),
		serverRoutes:         make(map[string]serverRequestRoute),
		events:               make(map[chan Event]struct{}),
		profileClient:        &http.Client{Timeout: 10 * time.Second},
		profileCache:         make(map[string]profileCacheEntry),
		now:                  time.Now,
		resetCreditsCache:    make(map[string]resetCreditsCacheEntry),
		resetCreditsEndpoint: rateLimitResetCreditsURL,
		resetPreviews:        make(map[string]ResetCreditsPreview),
	}, nil
}

func (m *Multiplexer) Start(ctx context.Context) error {
	for _, account := range m.store.Accounts() {
		if _, err := m.startChild(ctx, account); err != nil {
			fmt.Fprintf(os.Stderr, "codex-mux: start account %s: %v\n", account.ID, err)
		}
	}
	if len(m.childEntries()) == 0 {
		return errors.New("no Codex app-server process could be started")
	}
	go m.inboundLoop(ctx)
	go m.syncManagedConfigLoop(ctx)
	return nil
}

func (m *Multiplexer) syncManagedConfigLoop(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := m.store.SyncManagedConfig(); err != nil {
				fmt.Fprintf(os.Stderr, "codex-mux: sync shared plugin config: %v\n", err)
			}
		}
	}
}

func (m *Multiplexer) Close() {
	for _, entry := range m.childEntries() {
		_ = entry.child.Close()
	}
}

func (m *Multiplexer) HandleClient(message protocol.Message) {
	if message.Method == "" && len(message.ID) > 0 {
		m.handleServerRequestResponse(message)
		return
	}
	if message.Method == "initialize" && len(message.ID) > 0 {
		go m.initialize(message)
		return
	}
	if len(message.ID) == 0 {
		m.handleClientNotification(message)
		return
	}

	switch message.Method {
	case "thread/list":
		go m.aggregateThreadList(message)
	case "thread/start":
		go m.routeNewThread(message)
	case "account/rateLimits/read":
		go m.routeAggregatedRateLimits(message)
	default:
		m.routeExistingRequest(message)
	}
}

func (m *Multiplexer) initialize(message protocol.Message) {
	m.initializationMu.Lock()
	m.initializeParams = append(json.RawMessage(nil), message.Params...)
	m.initializationMu.Unlock()

	var firstResult json.RawMessage
	var firstErr error
	for _, entry := range m.childEntries() {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		response, err := entry.child.Request(ctx, "initialize", message.Params)
		cancel()
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if firstResult == nil {
			firstResult = response.Result
		}
	}
	if firstResult == nil {
		m.write(protocol.Failure(message.ID, -32000, fmt.Sprintf("failed to initialize account pool: %v", firstErr)))
		return
	}
	m.write(protocol.Success(message.ID, firstResult))
}

func (m *Multiplexer) handleClientNotification(message protocol.Message) {
	if message.Method == "initialized" {
		m.initializationMu.Lock()
		m.initialized = true
		m.initializationMu.Unlock()
		for _, entry := range m.childEntries() {
			_ = entry.child.Send(message)
		}
		return
	}
	if controller, ok := m.controllerChild(); ok {
		_ = controller.Send(message)
	}
}

func (m *Multiplexer) routeNewThread(message protocol.Message) {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	if controller, ok := m.store.Controller(); ok && accountBypassesChatGPTQuota(message.Params, controller) {
		if err := m.forward(controller.ID, message); err != nil {
			m.write(protocol.Failure(message.ID, -32021, err.Error()))
		}
		return
	}
	account, reason, err := m.chooseAccount(ctx)
	if err != nil {
		if errors.Is(err, errNoSubscriptionCapacity) {
			m.write(m.allSubscriptionsDepleted(ctx, message.ID))
			return
		}
		m.write(protocol.Failure(message.ID, -32020, err.Error()))
		return
	}
	if err := m.forward(account.ID, message); err != nil {
		m.write(protocol.Failure(message.ID, -32021, err.Error()))
		return
	}
	m.publish(Event{
		Type:      "thread-routed",
		AccountID: account.ID,
		Message:   fmt.Sprintf("New chat pinned to %s", account.Label),
		Data:      reason,
	})
}

func (m *Multiplexer) routeExistingRequest(message protocol.Message) {
	accountID := ""
	if scopedAccountID, cleanedParams, ok := scopedPluginRequest(message.Method, message.Params); ok {
		if account, exists := m.store.Account(scopedAccountID); exists && account.Enabled {
			message.Params = cleanedParams
			if err := m.forward(scopedAccountID, message); err != nil {
				m.write(protocol.Failure(message.ID, -32023, err.Error()))
			}
			return
		}
	}
	threadID := threadIDFromParams(message.Params)
	if threadID != "" {
		accountID, _ = m.store.ThreadOwner(threadID)
	}
	if accountID == "" {
		if controller, ok := m.store.Controller(); ok {
			accountID = controller.ID
		}
	}
	if accountID == "" {
		m.write(protocol.Failure(message.ID, -32022, "no controller account is configured"))
		return
	}
	// Reading a chat stays with the subscription that can show it. Its work goes to
	// whichever subscription took over when this one ran out.
	if followsTurn(message.Method) && threadID != "" {
		if host, ok := m.turnHost(threadID); ok && host != accountID {
			trace.note(host, "turn-host", "thread="+threadID)
			if err := m.forward(host, message); err == nil {
				return
			}
			m.forgetTurnHost(threadID)
		}
	}
	if err := m.forward(accountID, message); err != nil {
		m.write(protocol.Failure(message.ID, -32023, err.Error()))
	}
}

func (m *Multiplexer) forward(accountID string, message protocol.Message) error {
	return m.forwardWithExclusions(accountID, message, nil)
}

func (m *Multiplexer) forwardWithExclusions(accountID string, message protocol.Message, excluded map[string]struct{}) error {
	child, ok := m.child(accountID)
	if !ok {
		return fmt.Errorf("account %s is unavailable", accountID)
	}
	key := protocol.RequestIDKey(message.ID)
	m.externalMu.Lock()
	m.externalRoutes[key] = externalRoute{
		accountID: accountID,
		method:    message.Method,
		message:   message,
		excluded:  cloneAccountSet(excluded),
	}
	m.externalMu.Unlock()
	if err := child.Send(message); err != nil {
		m.externalMu.Lock()
		delete(m.externalRoutes, key)
		m.externalMu.Unlock()
		return err
	}
	if startsTurn(message.Method) {
		m.rememberInFlightTurn(
			threadIDFromParams(message.Params), accountID, message.Method, message.Params, excluded,
		)
	}
	trace.traceOutbound(accountID, message.Method, message.Params)
	return nil
}

func (m *Multiplexer) routeAggregatedRateLimits(message protocol.Message) {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	rateLimits, err := m.AggregatedRateLimits(ctx)
	if err != nil {
		m.write(protocol.Failure(message.ID, -32024, err.Error()))
		return
	}
	result, err := json.Marshal(map[string]any{"rateLimits": rateLimits})
	if err != nil {
		m.write(protocol.Failure(message.ID, -32025, err.Error()))
		return
	}
	m.write(protocol.Success(message.ID, result))
}

func (m *Multiplexer) failoverTurn(
	ctx context.Context,
	message protocol.Message,
	threadID string,
	sourceAccountID string,
	excluded map[string]struct{},
) {
	fallback, _, err := m.chooseAccountExcluding(ctx, excluded)
	if err != nil {
		trace.note(sourceAccountID, "failover-no-capacity", fmt.Sprintf("thread=%s err=%v", threadID, err))
		m.write(m.allSubscriptionsDepleted(ctx, message.ID))
		return
	}
	trace.note(fallback.ID, "failover-target", fmt.Sprintf("thread=%s from=%s", threadID, sourceAccountID))
	if err := m.resumeThreadOnAccount(ctx, threadID, sourceAccountID, fallback.ID); err != nil {
		trace.note(fallback.ID, "failover-resume-failed", fmt.Sprintf("thread=%s err=%v", threadID, err))
		m.write(protocol.Failure(message.ID, -32027, fmt.Sprintf("move chat to %s: %v", fallback.Label, err)))
		return
	}
	if err := m.store.SetThreadOwner(threadID, fallback.ID); err != nil {
		trace.note(fallback.ID, "failover-owner-failed", fmt.Sprintf("thread=%s err=%v", threadID, err))
		m.write(protocol.Failure(message.ID, -32028, err.Error()))
		return
	}
	if err := m.forwardWithExclusions(fallback.ID, message, excluded); err != nil {
		trace.note(fallback.ID, "failover-forward-failed", fmt.Sprintf("thread=%s err=%v", threadID, err))
		m.write(protocol.Failure(message.ID, -32023, err.Error()))
		return
	}
	trace.note(fallback.ID, "failover-done", "thread="+threadID)
	m.publish(Event{
		Type:      "thread-failed-over",
		AccountID: fallback.ID,
		Message:   fmt.Sprintf("Chat continued with %s", fallback.Label),
		Data:      map[string]any{"threadId": threadID, "previousAccountId": sourceAccountID},
	})
}

func (m *Multiplexer) resumeThreadOnAccount(ctx context.Context, threadID, sourceAccountID, targetAccountID string) error {
	source, ok := m.child(sourceAccountID)
	if !ok {
		return fmt.Errorf("source subscription is unavailable")
	}
	target, ok := m.child(targetAccountID)
	if !ok {
		return fmt.Errorf("target subscription is unavailable")
	}
	// Only the thread's identity and location are needed to resume it. Asking for the
	// turns as well reads the whole history, which on a long chat is slow enough to look
	// like the app has stopped.
	readParams, _ := json.Marshal(map[string]any{"threadId": threadID, "includeTurns": false})
	readResponse, err := source.Request(ctx, "thread/read", readParams)
	if err != nil {
		return fmt.Errorf("read existing chat: %w", err)
	}
	trace.note(sourceAccountID, "thread-read", fmt.Sprintf(
		"thread=%s bytes=%d", threadID, len(readResponse.Result),
	))
	var readResult struct {
		Thread struct {
			ID            string `json:"id"`
			Path          string `json:"path"`
			CWD           string `json:"cwd"`
			ModelProvider string `json:"modelProvider"`
		} `json:"thread"`
	}
	if err := json.Unmarshal(readResponse.Result, &readResult); err != nil {
		return fmt.Errorf("decode existing chat: %w", err)
	}
	trace.note(sourceAccountID, "thread-read-fields", fmt.Sprintf(
		"id=%q pathSet=%t cwdSet=%t provider=%q",
		readResult.Thread.ID, readResult.Thread.Path != "", readResult.Thread.CWD != "",
		readResult.Thread.ModelProvider,
	))
	if readResult.Thread.ID == "" || readResult.Thread.Path == "" {
		return errors.New("existing chat has no resumable history path")
	}
	sharedPath, err := m.shareRolloutWithAccount(
		readResult.Thread.Path, sourceAccountID, targetAccountID,
	)
	if err != nil {
		return err
	}

	// The goal has to be written before the resume: beforehand it is a local write and
	// the resume carries it over, afterwards it starts a turn of its own. A goal that
	// cannot be carried is not worth abandoning the chat's history for, so this only
	// warns.
	if goal, goalErr := m.readThreadGoal(ctx, sourceAccountID, threadID); goalErr != nil {
		trace.note(sourceAccountID, "goal-read-failed", fmt.Sprintf("thread=%s err=%v", threadID, goalErr))
	} else if goal != nil {
		if status, err := m.writeThreadGoal(ctx, targetAccountID, threadID, goal); err != nil {
			trace.note(targetAccountID, "goal-write-failed", fmt.Sprintf("thread=%s err=%v", threadID, err))
		} else {
			trace.note(targetAccountID, "goal-carried", fmt.Sprintf(
				"thread=%s was=%s now=%s", threadID, goal.Status, status,
			))
		}
	}

	resumeParams, _ := json.Marshal(map[string]any{
		"threadId":      threadID,
		"history":       nil,
		"path":          sharedPath,
		"cwd":           readResult.Thread.CWD,
		"model":         nil,
		"modelProvider": readResult.Thread.ModelProvider,
	})
	// Rebuilding a long chat's turns re-reads its whole history, so the resume gets a
	// deadline sized from that history rather than the generic request budget.
	resumeCtx, cancelResume := context.WithTimeout(context.Background(), resumeDeadline(sharedPath))
	defer cancelResume()
	started := time.Now()
	if _, err := target.Request(resumeCtx, "thread/resume", resumeParams); err != nil {
		trace.note(targetAccountID, "thread-resume-failed", fmt.Sprintf("thread=%s err=%v", threadID, err))
		return fmt.Errorf("resume existing chat: %w", err)
	}
	trace.note(targetAccountID, "thread-resume-ok", fmt.Sprintf(
		"thread=%s took=%s", threadID, time.Since(started).Round(time.Millisecond),
	))

	return nil
}

func (m *Multiplexer) handleServerRequestResponse(message protocol.Message) {
	key := protocol.RequestIDKey(message.ID)
	m.serverMu.Lock()
	route, ok := m.serverRoutes[key]
	if ok {
		delete(m.serverRoutes, key)
	}
	m.serverMu.Unlock()
	if !ok {
		return
	}
	message.ID = route.original
	if child, exists := m.child(route.accountID); exists {
		_ = child.Send(message)
	}
}

func (m *Multiplexer) inboundLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case inbound := <-m.inbound:
			m.handleInbound(inbound)
		}
	}
}

func (m *Multiplexer) handleInbound(inbound backend.Inbound) {
	message := inbound.Message
	trace.traceInbound(inbound.AccountID, message)
	if message.Method == "" && len(message.ID) > 0 {
		key := protocol.RequestIDKey(message.ID)
		m.externalMu.Lock()
		route, ok := m.externalRoutes[key]
		if ok {
			delete(m.externalRoutes, key)
		}
		m.externalMu.Unlock()
		if ok {
			if startsTurn(route.method) && isUsageLimitResponse(message) {
				if owner, ok := m.store.Account(route.accountID); ok && accountBypassesChatGPTQuota(route.message.Params, owner) {
					message.ID = route.message.ID
					m.write(message)
					return
				}
				go m.retryTurnAfterUsageLimit(route, inbound.AccountID)
				return
			}
			m.learnThreadOwner(route, inbound.AccountID, message.Result)
			m.writeRaw(inbound.Raw)
		}
		return
	}
	if message.Method != "" && len(message.ID) > 0 {
		m.forwardServerRequest(inbound)
		return
	}
	if message.Method == "account/rateLimits/updated" {
		go m.forwardAggregatedRateLimitNotification(inbound.Raw)
		return
	}
	if message.Method == "error" {
		if notice, ok := usageLimitNotification(message.Params); ok &&
			m.beginTurnFailover(inbound, notice) {
			return
		}
	}
	if message.Method == "turn/completed" {
		if notice, ok := decodeTurnCompleted(message.Params); ok {
			if m.keepWithheldTurnCompleted(notice.ThreadID, inbound.AccountID, inbound.Raw) {
				go m.publishAccountRefresh(inbound.AccountID)
				return
			}
			// A failed turn keeps its record: the error notification that explains the
			// failure can arrive after the completion, and it is what moves the chat.
			if notice.Turn.Status != "failed" {
				m.forgetInFlightTurn(notice.ThreadID, inbound.AccountID)
			}
		}
	}
	if message.Method == "thread/started" {
		if threadID := threadIDFromNotification(message.Params); threadID != "" {
			_ = m.store.SetThreadOwner(threadID, inbound.AccountID)
		}
	}
	if message.Method == "turn/completed" ||
		message.Method == "account/login/completed" ||
		message.Method == "account/updated" {
		go m.publishAccountRefresh(inbound.AccountID)
	}
	if m.shouldForwardNotification(inbound.AccountID, message.Method) {
		m.writeRaw(inbound.Raw)
	}
}

func (m *Multiplexer) forwardAggregatedRateLimitNotification(fallback []byte) {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	rateLimits, err := m.AggregatedRateLimits(ctx)
	if err != nil {
		m.writeRaw(fallback)
		return
	}
	params, err := json.Marshal(map[string]any{"rateLimits": rateLimits})
	if err != nil {
		m.writeRaw(fallback)
		return
	}
	m.write(protocol.Message{Method: "account/rateLimits/updated", Params: params})
}

func (m *Multiplexer) retryTurnAfterUsageLimit(route externalRoute, exhaustedAccountID string) {
	threadID := threadIDFromParams(route.message.Params)
	if threadID == "" {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		m.write(m.allSubscriptionsDepleted(ctx, route.message.ID))
		return
	}
	excluded := cloneAccountSet(route.excluded)
	if excluded == nil {
		excluded = make(map[string]struct{})
	}
	excluded[exhaustedAccountID] = struct{}{}
	ctx, cancel := context.WithTimeout(context.Background(), handoverTimeout)
	defer cancel()
	m.failoverTurn(ctx, route.message, threadID, exhaustedAccountID, excluded)
}

func (m *Multiplexer) forwardServerRequest(inbound backend.Inbound) {
	sequence := m.serverSequence.Add(1)
	newID := protocol.StringID(fmt.Sprintf("codex-mux:%s:%d", inbound.AccountID, sequence))
	key := protocol.RequestIDKey(newID)
	m.serverMu.Lock()
	m.serverRoutes[key] = serverRequestRoute{
		accountID: inbound.AccountID,
		original:  append(json.RawMessage(nil), inbound.Message.ID...),
	}
	m.serverMu.Unlock()
	inbound.Message.ID = newID
	m.write(inbound.Message)
}

func (m *Multiplexer) shouldForwardNotification(accountID, method string) bool {
	controller, ok := m.store.Controller()
	if ok && controller.ID == accountID {
		return true
	}
	if method == "error" {
		return true
	}
	return strings.HasPrefix(method, "thread/") ||
		strings.HasPrefix(method, "turn/") ||
		strings.HasPrefix(method, "item/") ||
		strings.HasPrefix(method, "hook/") ||
		strings.HasPrefix(method, "rawResponse")
}

func (m *Multiplexer) learnThreadOwner(route externalRoute, accountID string, result json.RawMessage) {
	switch route.method {
	case "thread/start", "thread/fork", "thread/resume", "thread/unarchive":
		if threadID := threadIDFromResult(result); threadID != "" {
			_ = m.store.SetThreadOwner(threadID, accountID)
		}
	}
}

func (m *Multiplexer) write(message protocol.Message) {
	encoded, err := protocol.Encode(message)
	if err != nil {
		fmt.Fprintf(os.Stderr, "codex-mux: encode response: %v\n", err)
		return
	}
	m.writeRaw(encoded)
}

func (m *Multiplexer) writeRaw(encoded []byte) {
	m.outputMu.Lock()
	defer m.outputMu.Unlock()
	_, _ = m.output.Write(append(encoded, '\n'))
}

type childEntry struct {
	account state.Account
	child   *backend.Child
}

func (m *Multiplexer) childEntries() []childEntry {
	accounts := m.store.Accounts()
	m.childrenMu.RLock()
	defer m.childrenMu.RUnlock()
	entries := make([]childEntry, 0, len(accounts))
	for _, account := range accounts {
		if child := m.children[account.ID]; child != nil {
			entries = append(entries, childEntry{account: account, child: child})
		}
	}
	return entries
}

func (m *Multiplexer) child(accountID string) (*backend.Child, bool) {
	m.childrenMu.RLock()
	defer m.childrenMu.RUnlock()
	child, ok := m.children[accountID]
	return child, ok
}

func (m *Multiplexer) controllerChild() (*backend.Child, bool) {
	controller, ok := m.store.Controller()
	if !ok {
		return nil, false
	}
	return m.child(controller.ID)
}

func (m *Multiplexer) startChild(ctx context.Context, account state.Account) (*backend.Child, error) {
	if child, ok := m.child(account.ID); ok {
		return child, nil
	}
	child, err := backend.Start(
		account.ID,
		account.CodexHome,
		m.realExecutable,
		m.realArgs,
		m.environment,
		m.inbound,
	)
	if err != nil {
		return nil, err
	}
	m.childrenMu.Lock()
	m.children[account.ID] = child
	m.childrenMu.Unlock()

	m.initializationMu.RLock()
	params := append(json.RawMessage(nil), m.initializeParams...)
	initialized := m.initialized
	m.initializationMu.RUnlock()
	if len(params) > 0 {
		requestCtx, cancel := context.WithTimeout(ctx, requestTimeout)
		_, err := child.Request(requestCtx, "initialize", params)
		cancel()
		if err != nil {
			return nil, err
		}
		if initialized {
			_ = child.Send(protocol.Message{Method: "initialized"})
		}
	}
	return child, nil
}

func (m *Multiplexer) SubscribeEvents() (<-chan Event, func()) {
	channel := make(chan Event, 32)
	m.eventsMu.Lock()
	m.events[channel] = struct{}{}
	m.eventsMu.Unlock()
	return channel, func() {
		m.eventsMu.Lock()
		if _, ok := m.events[channel]; ok {
			delete(m.events, channel)
			close(channel)
		}
		m.eventsMu.Unlock()
	}
}

func (m *Multiplexer) publish(event Event) {
	m.eventsMu.RLock()
	defer m.eventsMu.RUnlock()
	for channel := range m.events {
		select {
		case channel <- event:
		default:
		}
	}
}

func (m *Multiplexer) publishAccountRefresh(accountID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	snapshot, err := m.accountSnapshot(ctx, accountID)
	if err == nil {
		m.publish(Event{Type: "account-updated", AccountID: accountID, Data: snapshot})
	}
}

func threadIDFromParams(params json.RawMessage) string {
	if len(params) == 0 {
		return ""
	}
	var decoded map[string]any
	if json.Unmarshal(params, &decoded) != nil {
		return ""
	}
	for _, key := range []string{"threadId", "thread_id"} {
		if value, ok := decoded[key].(string); ok {
			return value
		}
	}
	return ""
}

func threadIDFromResult(result json.RawMessage) string {
	var decoded struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	if json.Unmarshal(result, &decoded) != nil {
		return ""
	}
	return decoded.Thread.ID
}

func threadIDFromNotification(params json.RawMessage) string {
	return threadIDFromResult(params)
}

func isUsageLimitResponse(message protocol.Message) bool {
	if message.Error == nil {
		return false
	}
	text := strings.ToLower(message.Error.Message + " " + string(message.Error.Data))
	return strings.Contains(text, "usage_limit") ||
		strings.Contains(text, "usage limit") ||
		strings.Contains(text, "rate_limit") ||
		strings.Contains(text, "rate limit") ||
		strings.Contains(text, "quota")
}

func (m *Multiplexer) allSubscriptionsDepleted(ctx context.Context, id json.RawMessage) protocol.Message {
	var resetsAt *int64
	if preview := m.currentRateLimitPreview(); preview != nil && preview.Mode.isAllDepleted() {
		resetsAt = preview.ResetsAt
	} else if limits, err := m.AggregatedRateLimits(ctx); err == nil {
		weekly, _ := longestAndShortestWindow(limits)
		if weekly != nil {
			resetsAt = weekly.ResetsAt
		}
	}
	return allSubscriptionsDepleted(id, resetsAt)
}

func allSubscriptionsDepleted(id json.RawMessage, resetsAt *int64) protocol.Message {
	message := "All connected subscriptions are depleted. Add another subscription or wait for usage to reset."
	if resetsAt != nil {
		reset := time.Unix(*resetsAt, 0).In(time.Local)
		message = fmt.Sprintf(
			"All connected subscriptions are depleted. Usage resets on %s.",
			reset.Format("Monday, 2 January at 3:04 PM"),
		)
	}
	return protocol.Failure(
		id,
		-32026,
		message,
	)
}

func cloneAccountSet(source map[string]struct{}) map[string]struct{} {
	if len(source) == 0 {
		return nil
	}
	clone := make(map[string]struct{}, len(source))
	for accountID := range source {
		clone[accountID] = struct{}{}
	}
	return clone
}

func sortThreads(threads []map[string]any) {
	sort.SliceStable(threads, func(i, j int) bool {
		return numericField(threads[i], "updatedAt", "createdAt") > numericField(threads[j], "updatedAt", "createdAt")
	})
}

func numericField(value map[string]any, keys ...string) float64 {
	for _, key := range keys {
		if number, ok := value[key].(float64); ok {
			return number
		}
	}
	return 0
}
