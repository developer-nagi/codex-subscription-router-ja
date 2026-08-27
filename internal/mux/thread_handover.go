package mux

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// A chat's history is one file, but everything a subscription knows *about* a chat lives
// in that subscription's own Codex home. Sharing the history is therefore only the first
// step of a handover; these are the rest, in the order they have to happen.
//
//   - The chat's record does not need carrying. Listing and reading a chat both rebuild
//     it from the history file, so the receiving subscription finds the chat on its own.
//   - A paginated chat's turns and items do. They live in the receiving subscription's
//     own store, and only thread/resume rebuilds them from the history. Until it has,
//     the chat opens empty there - which is what made a handed-over chat read as a lost
//     conversation.
//   - The goal does not travel at all, and setting it must come BEFORE the resume:
//     beforehand it is a local write and the resume then reports the goal as carried
//     over; afterwards it starts a turn of its own.
type threadGoal struct {
	Objective   string `json:"objective"`
	Status      string `json:"status"`
	TokenBudget *int   `json:"tokenBudget"`
}

// readThreadGoal returns the chat's goal, or nil when it has none.
func (m *Multiplexer) readThreadGoal(
	ctx context.Context, accountID, threadID string,
) (*threadGoal, error) {
	child, ok := m.child(accountID)
	if !ok {
		return nil, fmt.Errorf("subscription %s is unavailable", accountID)
	}
	params, _ := json.Marshal(map[string]any{"threadId": threadID})
	response, err := child.Request(ctx, "thread/goal/get", params)
	if err != nil {
		return nil, err
	}
	var result struct {
		Goal *threadGoal `json:"goal"`
	}
	if err := json.Unmarshal(response.Result, &result); err != nil {
		return nil, fmt.Errorf("decode goal: %w", err)
	}
	return result.Goal, nil
}

// writeThreadGoal puts the goal on the receiving subscription and reports the state it
// was given. Sent before the resume it writes the goal and starts nothing. The goal's
// age and its usage so far cannot be carried: the request has no fields for them.
func (m *Multiplexer) writeThreadGoal(
	ctx context.Context, accountID, threadID string, goal *threadGoal,
) (string, error) {
	child, ok := m.child(accountID)
	if !ok {
		return "", fmt.Errorf("subscription %s is unavailable", accountID)
	}
	// A goal the subscription stopped because it ran out is carried across as running:
	// the reason it stopped does not apply where it is going, and leaving it stopped
	// would make the move pointless. Any other state is passed through as it was.
	status := goal.Status
	if status == "usageLimited" || status == "budgetLimited" {
		status = "active"
	}
	params, _ := json.Marshal(map[string]any{
		"threadId":    threadID,
		"objective":   goal.Objective,
		"status":      status,
		"tokenBudget": goal.TokenBudget,
	})
	if _, err := child.Request(ctx, "thread/goal/set", params); err != nil {
		return status, err
	}
	return status, nil
}

// rememberTurnHost records which subscription is running a chat's work.
//
// It is deliberately not persisted. After a restart the chat's own subscription is tried
// again, which is right: whatever made it run out may have reset, and the subscription
// that took over no longer has the chat resumed.
func (m *Multiplexer) rememberTurnHost(threadID, accountID string) {
	if threadID == "" {
		return
	}
	m.turnsMu.Lock()
	defer m.turnsMu.Unlock()
	m.turnHosts[threadID] = accountID
}

func (m *Multiplexer) turnHost(threadID string) (string, bool) {
	m.turnsMu.Lock()
	defer m.turnsMu.Unlock()
	accountID, ok := m.turnHosts[threadID]
	return accountID, ok
}

func (m *Multiplexer) forgetTurnHost(threadID string) {
	m.turnsMu.Lock()
	defer m.turnsMu.Unlock()
	delete(m.turnHosts, threadID)
}

// resumeDeadline sizes the wait for a resume from the history it has to read.
//
// Rebuilding a paginated chat's turns re-reads the whole history, so the time it takes
// follows the file rather than any fixed request budget. A long chat is exactly the one
// worth moving, so it gets the time it needs instead of being abandoned half way.
func resumeDeadline(rolloutPath string) time.Duration {
	const base = 30 * time.Second
	const bytesPerSecond = 4 << 20
	const ceiling = 10 * time.Minute
	info, err := os.Stat(rolloutPath)
	if err != nil {
		return base
	}
	deadline := base + time.Duration(info.Size()/bytesPerSecond)*time.Second
	if deadline > ceiling {
		return ceiling
	}
	return deadline
}
