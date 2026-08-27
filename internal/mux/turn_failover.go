package mux

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/developer-nagi/codex-subscription-router-win/internal/backend"
)

// usageLimitErrorInfo is the value the app-server reports when a subscription runs out
// of weekly allowance while a turn is already running.
const usageLimitErrorInfo = "usageLimitExceeded"

// threadHandoverEnabled reports whether an existing chat may be moved to another
// subscription when the one it belongs to runs out.
//
// The move itself works: the history is shared with the receiving subscription and the
// chat resumes there. What does not survive is everything the receiving subscription
// does not already know about the chat. A goal comes back reported as cleared, and the
// chat's record lives in the original subscription's database, so handing the chat over
// left it opened on a subscription that could not show its history. Losing a
// conversation is worse than the limit the move was meant to work around, so the move is
// held back until a moved chat is demonstrably as usable as it was before.
//
// Routing a NEW chat away from a subscription that is out is unaffected and still works:
// nothing has to move, because the chat starts where there is room.
//
// Set CODEX_MUX_THREAD_HANDOVER=1 to take part in proving out the move.
func threadHandoverEnabled() bool {
	return strings.TrimSpace(os.Getenv("CODEX_MUX_THREAD_HANDOVER")) == "1"
}

// startsTurn reports whether a request makes a subscription run a turn.
//
// A method missing from this list is charged to the subscription that receives it and
// cannot be moved when that subscription runs out, so anything that makes a subscription
// work belongs here.
func startsTurn(method string) bool {
	switch method {
	case "turn/start", "thread/goal/set":
		return true
	default:
		return false
	}
}

// inFlightTurn is a turn a subscription has already accepted.
//
// turn/start answers as soon as the turn is accepted, so a usage limit reached while
// the turn runs never appears in that response. It arrives afterwards as an "error"
// notification instead. Keeping the request lets the chat continue on another
// subscription rather than stopping at the limit.
type inFlightTurn struct {
	accountID string
	method    string
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
	threadID, accountID, method string, params json.RawMessage, excluded map[string]struct{},
) {
	if threadID == "" {
		return
	}
	m.turnsMu.Lock()
	defer m.turnsMu.Unlock()
	m.inFlightTurns[threadID] = inFlightTurn{
		accountID: accountID,
		method:    method,
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

// The subscription that ran out still completes its turn as failed. While the chat is
// being moved that completion describes a turn the chat no longer uses, so it is held
// back - but it is kept, not discarded: if the chat does not move after all, the turn
// really did end, and a chat left waiting on a turn nobody will finish sits thinking
// forever.
func (m *Multiplexer) withholdTurnCompleted(threadID, accountID string) {
	m.turnsMu.Lock()
	defer m.turnsMu.Unlock()
	m.withheldCompletions[threadID+"\x00"+accountID] = nil
}

// takeWithheldTurnCompleted reports whether the completion is being held back, and hands
// back whatever has arrived so far so the caller can release it.
func (m *Multiplexer) takeWithheldTurnCompleted(threadID, accountID string) ([]byte, bool) {
	m.turnsMu.Lock()
	defer m.turnsMu.Unlock()
	key := threadID + "\x00" + accountID
	raw, ok := m.withheldCompletions[key]
	if !ok {
		return nil, false
	}
	delete(m.withheldCompletions, key)
	return raw, true
}

// keepWithheldTurnCompleted stores the completion that arrived while the chat was being
// moved, so it can be released if the move is refused.
func (m *Multiplexer) keepWithheldTurnCompleted(threadID, accountID string, raw []byte) bool {
	m.turnsMu.Lock()
	defer m.turnsMu.Unlock()
	key := threadID + "\x00" + accountID
	if _, ok := m.withheldCompletions[key]; !ok {
		return false
	}
	m.withheldCompletions[key] = append([]byte(nil), raw...)
	return true
}

// beginTurnFailover moves a chat that hit its subscription's limit onto another
// subscription and starts the same turn there. It reports whether it took ownership of
// the notification: when it did, the notification is withheld and the failover reports
// the original error itself if the chat cannot continue anywhere.
func (m *Multiplexer) beginTurnFailover(inbound backend.Inbound, notice errorNotification) bool {
	if !threadHandoverEnabled() {
		trace.note(inbound.AccountID, "failover-held-back", "thread="+notice.ThreadID)
		m.forgetInFlightTurn(notice.ThreadID, inbound.AccountID)
		return false
	}
	turn, ok := m.takeInFlightTurn(notice.ThreadID, inbound.AccountID)
	if !ok {
		trace.note(inbound.AccountID, "failover-skipped", "no recorded turn for thread="+notice.ThreadID)
		return false
	}
	if owner, exists := m.store.Account(inbound.AccountID); exists &&
		accountBypassesChatGPTQuota(turn.params, owner) {
		return false
	}
	trace.note(inbound.AccountID, "failover-start", "thread="+notice.ThreadID)
	m.withholdTurnCompleted(notice.ThreadID, inbound.AccountID)
	raw := append([]byte(nil), inbound.Raw...)
	go m.continueTurnOnAnotherAccount(notice.ThreadID, inbound.AccountID, turn, raw)
	return true
}

func (m *Multiplexer) continueTurnOnAnotherAccount(
	threadID, sourceAccountID string, turn inFlightTurn, raw []byte,
) {
	ctx, cancel := context.WithTimeout(context.Background(), handoverTimeout)
	defer cancel()

	excluded := cloneAccountSet(turn.excluded)
	if excluded == nil {
		excluded = make(map[string]struct{})
	}
	excluded[sourceAccountID] = struct{}{}

	// Any failure here leaves the chat where it is, so the subscription's own error is
	// released instead of being swallowed.
	surrender := func(reason error) {
		// The turn really did end, so its completion is released along with the error.
		// Without it the chat waits on a turn nobody will finish.
		if completed, held := m.takeWithheldTurnCompleted(threadID, sourceAccountID); held &&
			len(completed) > 0 {
			defer m.writeRaw(completed)
		}
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
	// The chat is NOT handed over - only the turn is.
	//
	// A chat's turns are rebuilt into whichever subscription's own store, from a history
	// that is read as the chat is used. On a long chat that rebuilding is nowhere near
	// done, so a subscription that has just been given the history still cannot show it.
	// Moving the chat there left it opening empty. The history file itself is shared, so
	// what the running subscription appends is visible to the one that owns the chat, and
	// there is nothing to gain by moving ownership as well: reading stays where it works,
	// and only the work moves.
	m.rememberTurnHost(threadID, fallback.ID)
	child, ok := m.child(fallback.ID)
	if !ok {
		surrender(fmt.Errorf("subscription %s is unavailable", fallback.ID))
		return
	}
	// The original turn/start was answered long ago, so the retry is sent under the
	// multiplexer's own request id. The chat sees the new turn through the
	// notifications the fallback subscription emits.
	m.rememberInFlightTurn(threadID, fallback.ID, turn.method, turn.params, excluded)
	if _, err := child.Request(ctx, turn.method, turn.params); err != nil {
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
