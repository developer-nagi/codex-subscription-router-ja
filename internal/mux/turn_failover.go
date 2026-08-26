package mux

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/developer-nagi/codex-subscription-router-win/internal/backend"
)

// usageLimitErrorInfo is the value the app-server reports when a subscription runs out
// of weekly allowance while a turn is already running.
const usageLimitErrorInfo = "usageLimitExceeded"

// inFlightTurn is a turn a subscription has already accepted.
//
// turn/start answers as soon as the turn is accepted, so a usage limit reached while
// the turn runs never appears in that response. It arrives afterwards as an "error"
// notification instead. Keeping the request lets the chat continue on another
// subscription rather than stopping at the limit.
type inFlightTurn struct {
	accountID string
	params    json.RawMessage
	excluded  map[string]struct{}
}

// errorNotification is the app-server's "error" notification.
type errorNotification struct {
	ThreadID  string `json:"threadId"`
	TurnID    string `json:"turnId"`
	WillRetry bool   `json:"willRetry"`
	Error     struct {
		Message string `json:"message"`
		// The field is a string for a plain failure and an object for transport
		// errors, so it is decoded loosely and compared as a string.
		CodexErrorInfo json.RawMessage `json:"codexErrorInfo"`
	} `json:"error"`
}

// usageLimitNotification reports whether params describe a turn that stopped because
// its subscription is out of allowance. A retryable failure is left alone: the client
// recovers from it without changing subscription.
func usageLimitNotification(params json.RawMessage) (errorNotification, bool) {
	var notice errorNotification
	if len(params) == 0 || json.Unmarshal(params, &notice) != nil {
		return errorNotification{}, false
	}
	if notice.ThreadID == "" || notice.WillRetry {
		return errorNotification{}, false
	}
	var info string
	if json.Unmarshal(notice.Error.CodexErrorInfo, &info) != nil {
		return errorNotification{}, false
	}
	if info != usageLimitErrorInfo {
		return errorNotification{}, false
	}
	return notice, true
}

// turnCompletedNotification is the app-server's "turn/completed" notification. It
// carries the thread id at the top level, unlike the thread notifications.
type turnCompletedNotification struct {
	ThreadID string `json:"threadId"`
	Turn     struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	} `json:"turn"`
}

func decodeTurnCompleted(params json.RawMessage) (turnCompletedNotification, bool) {
	var notice turnCompletedNotification
	if len(params) == 0 || json.Unmarshal(params, &notice) != nil {
		return turnCompletedNotification{}, false
	}
	if notice.ThreadID == "" {
		return turnCompletedNotification{}, false
	}
	return notice, true
}

func (m *Multiplexer) rememberInFlightTurn(
	threadID, accountID string, params json.RawMessage, excluded map[string]struct{},
) {
	if threadID == "" {
		return
	}
	m.turnsMu.Lock()
	defer m.turnsMu.Unlock()
	m.inFlightTurns[threadID] = inFlightTurn{
		accountID: accountID,
		params:    append(json.RawMessage(nil), params...),
		excluded:  cloneAccountSet(excluded),
	}
}

// takeInFlightTurn removes and returns the turn accountID is running on threadID. A
// turn recorded for another subscription is left in place, so a late notification from
// a subscription the chat already moved off cannot take the turn away again.
func (m *Multiplexer) takeInFlightTurn(threadID, accountID string) (inFlightTurn, bool) {
	m.turnsMu.Lock()
	defer m.turnsMu.Unlock()
	turn, ok := m.inFlightTurns[threadID]
	if !ok || turn.accountID != accountID {
		return inFlightTurn{}, false
	}
	delete(m.inFlightTurns, threadID)
	return turn, true
}

func (m *Multiplexer) forgetInFlightTurn(threadID, accountID string) {
	m.turnsMu.Lock()
	defer m.turnsMu.Unlock()
	if turn, ok := m.inFlightTurns[threadID]; ok && turn.accountID == accountID {
		delete(m.inFlightTurns, threadID)
	}
}

// The subscription that ran out still completes its turn as failed. That completion
// describes a turn the chat no longer uses, so it is withheld once.
func (m *Multiplexer) withholdTurnCompleted(threadID, accountID string) {
	m.turnsMu.Lock()
	defer m.turnsMu.Unlock()
	m.withheldCompletions[threadID+"\x00"+accountID] = struct{}{}
}

func (m *Multiplexer) turnCompletedIsWithheld(threadID, accountID string) bool {
	m.turnsMu.Lock()
	defer m.turnsMu.Unlock()
	key := threadID + "\x00" + accountID
	if _, ok := m.withheldCompletions[key]; !ok {
		return false
	}
	delete(m.withheldCompletions, key)
	return true
}

// beginTurnFailover moves a chat that hit its subscription's limit onto another
// subscription and starts the same turn there. It reports whether it took ownership of
// the notification: when it did, the notification is withheld and the failover reports
// the original error itself if the chat cannot continue anywhere.
func (m *Multiplexer) beginTurnFailover(inbound backend.Inbound, notice errorNotification) bool {
	turn, ok := m.takeInFlightTurn(notice.ThreadID, inbound.AccountID)
	if !ok {
		return false
	}
	if owner, exists := m.store.Account(inbound.AccountID); exists &&
		accountBypassesChatGPTQuota(turn.params, owner) {
		return false
	}
	m.withholdTurnCompleted(notice.ThreadID, inbound.AccountID)
	raw := append([]byte(nil), inbound.Raw...)
	go m.continueTurnOnAnotherAccount(notice.ThreadID, inbound.AccountID, turn, raw)
	return true
}

func (m *Multiplexer) continueTurnOnAnotherAccount(
	threadID, sourceAccountID string, turn inFlightTurn, raw []byte,
) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*requestTimeout)
	defer cancel()

	excluded := cloneAccountSet(turn.excluded)
	if excluded == nil {
		excluded = make(map[string]struct{})
	}
	excluded[sourceAccountID] = struct{}{}

	// Any failure here leaves the chat where it is, so the subscription's own error is
	// released instead of being swallowed.
	surrender := func(reason error) {
		m.turnCompletedIsWithheld(threadID, sourceAccountID)
		if reason != nil {
			m.publish(Event{
				Type:      "thread-failover-failed",
				AccountID: sourceAccountID,
				Message:   fmt.Sprintf("could not continue the chat elsewhere: %v", reason),
				Data:      map[string]any{"threadId": threadID},
			})
		}
		m.writeRaw(raw)
	}

	fallback, _, err := m.chooseAccountExcluding(ctx, excluded)
	if err != nil {
		surrender(nil)
		return
	}
	if err := m.resumeThreadOnAccount(ctx, threadID, sourceAccountID, fallback.ID); err != nil {
		surrender(err)
		return
	}
	if err := m.store.SetThreadOwner(threadID, fallback.ID); err != nil {
		surrender(err)
		return
	}
	child, ok := m.child(fallback.ID)
	if !ok {
		surrender(fmt.Errorf("subscription %s is unavailable", fallback.ID))
		return
	}
	// The original turn/start was answered long ago, so the retry is sent under the
	// multiplexer's own request id. The chat sees the new turn through the
	// notifications the fallback subscription emits.
	m.rememberInFlightTurn(threadID, fallback.ID, turn.params, excluded)
	if _, err := child.Request(ctx, "turn/start", turn.params); err != nil {
		m.forgetInFlightTurn(threadID, fallback.ID)
		surrender(err)
		return
	}
	m.publish(Event{
		Type:      "thread-failed-over",
		AccountID: fallback.ID,
		Message:   fmt.Sprintf("Chat continued with %s", fallback.Label),
		Data:      map[string]any{"threadId": threadID, "previousAccountId": sourceAccountID},
	})
}
