package mux

import (
	"encoding/json"
	"testing"
)

func TestUsageLimitNotificationClassifiesErrorNotifications(t *testing.T) {
	cases := []struct {
		name     string
		params   string
		expected bool
	}{
		{
			name: "usage limit stops the turn",
			params: `{"threadId":"t1","turnId":"u1","willRetry":false,` +
				`"error":{"message":"You've hit your usage limit.","codexErrorInfo":"usageLimitExceeded"}}`,
			expected: true,
		},
		{
			name: "a retryable failure recovers without changing subscription",
			params: `{"threadId":"t1","turnId":"u1","willRetry":true,` +
				`"error":{"message":"You've hit your usage limit.","codexErrorInfo":"usageLimitExceeded"}}`,
			expected: false,
		},
		{
			name: "an unrelated failure is left alone",
			params: `{"threadId":"t1","turnId":"u1","willRetry":false,` +
				`"error":{"message":"server overloaded","codexErrorInfo":"serverOverloaded"}}`,
			expected: false,
		},
		{
			name: "a transport failure reports an object rather than a name",
			params: `{"threadId":"t1","turnId":"u1","willRetry":false,` +
				`"error":{"message":"lost the stream","codexErrorInfo":{"responseStreamDisconnected":{}}}}`,
			expected: false,
		},
		{
			name:     "a notification without a thread cannot be moved",
			params:   `{"willRetry":false,"error":{"codexErrorInfo":"usageLimitExceeded"}}`,
			expected: false,
		},
		{
			name:     "empty params are not a failure",
			params:   ``,
			expected: false,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			notice, ok := usageLimitNotification(json.RawMessage(testCase.params))
			if ok != testCase.expected {
				t.Fatalf("usageLimitNotification = %v, want %v", ok, testCase.expected)
			}
			if ok && notice.ThreadID != "t1" {
				t.Fatalf("thread id = %q, want %q", notice.ThreadID, "t1")
			}
		})
	}
}

func TestDecodeTurnCompletedReadsTopLevelThreadID(t *testing.T) {
	notice, ok := decodeTurnCompleted(json.RawMessage(
		`{"threadId":"t1","turn":{"id":"u1","status":"failed"}}`,
	))
	if !ok {
		t.Fatal("expected turn/completed to decode")
	}
	if notice.ThreadID != "t1" || notice.Turn.Status != "failed" {
		t.Fatalf("decoded %+v, want thread t1 with status failed", notice)
	}
	if _, ok := decodeTurnCompleted(json.RawMessage(`{"thread":{"id":"t1"}}`)); ok {
		t.Fatal("a thread notification must not decode as a turn completion")
	}
}

func newFailoverTestMultiplexer() *Multiplexer {
	return &Multiplexer{
		inFlightTurns:       make(map[string]inFlightTurn),
		withheldCompletions: make(map[string][]byte),
	}
}

func TestInFlightTurnBelongsToTheSubscriptionRunningIt(t *testing.T) {
	multiplexer := newFailoverTestMultiplexer()
	params := json.RawMessage(`{"threadId":"t1","input":[]}`)
	multiplexer.rememberInFlightTurn("t1", "primary", "turn/start", params, map[string]struct{}{"gone": {}})

	if _, ok := multiplexer.takeInFlightTurn("t1", "secondary"); ok {
		t.Fatal("another subscription must not take the turn")
	}
	turn, ok := multiplexer.takeInFlightTurn("t1", "primary")
	if !ok {
		t.Fatal("the subscription running the turn must be able to take it")
	}
	if turn.method != "turn/start" {
		t.Fatalf("method = %q, want the request that started the turn", turn.method)
	}
	if string(turn.params) != string(params) {
		t.Fatalf("params = %s, want %s", turn.params, params)
	}
	if _, excluded := turn.excluded["gone"]; !excluded {
		t.Fatal("the exclusion set must survive, so a chat never returns to a depleted subscription")
	}
	if _, ok := multiplexer.takeInFlightTurn("t1", "primary"); ok {
		t.Fatal("a taken turn must not be handed out twice")
	}
}

func TestRememberInFlightTurnIgnoresATurnWithoutAThread(t *testing.T) {
	multiplexer := newFailoverTestMultiplexer()
	multiplexer.rememberInFlightTurn("", "primary", "turn/start", json.RawMessage(`{}`), nil)
	if len(multiplexer.inFlightTurns) != 0 {
		t.Fatalf("recorded %d turns, want none", len(multiplexer.inFlightTurns))
	}
}

func TestForgetInFlightTurnOnlyClearsItsOwnSubscription(t *testing.T) {
	multiplexer := newFailoverTestMultiplexer()
	multiplexer.rememberInFlightTurn("t1", "primary", "turn/start", json.RawMessage(`{}`), nil)
	multiplexer.forgetInFlightTurn("t1", "secondary")
	if len(multiplexer.inFlightTurns) != 1 {
		t.Fatal("a completion from another subscription must not clear the turn")
	}
	multiplexer.forgetInFlightTurn("t1", "primary")
	if len(multiplexer.inFlightTurns) != 0 {
		t.Fatal("the running subscription's completion must clear the turn")
	}
}

func TestAWithheldCompletionIsKeptSoItCanBeReleased(t *testing.T) {
	multiplexer := newFailoverTestMultiplexer()
	multiplexer.withholdTurnCompleted("t1", "primary")

	if multiplexer.keepWithheldTurnCompleted("t1", "secondary", []byte("{}")) {
		t.Fatal("the completion of the subscription the chat moved to must reach the chat")
	}
	if !multiplexer.keepWithheldTurnCompleted("t1", "primary", []byte(`{"turn":1}`)) {
		t.Fatal("the abandoned turn's completion must be held back")
	}
	// If the chat does not move after all, the turn really did end, and the chat must be
	// told: otherwise it waits on a turn nobody will finish.
	raw, held := multiplexer.takeWithheldTurnCompleted("t1", "primary")
	if !held || string(raw) != `{"turn":1}` {
		t.Fatalf("held=%v raw=%s, want the completion back verbatim", held, raw)
	}
	if _, held := multiplexer.takeWithheldTurnCompleted("t1", "primary"); held {
		t.Fatal("only one completion belongs to the abandoned turn")
	}
}

func TestStartsTurnCoversEveryRequestThatRunsATurn(t *testing.T) {
	if !startsTurn("turn/start") {
		t.Fatal("turn/start runs a turn and must be treated as a turn start")
	}
	for _, method := range []string{
		"turn/interrupt", "thread/goal/get", "thread/goal/clear", "thread/turns/list", "",
	} {
		if startsTurn(method) {
			t.Fatalf("%s does not run a turn", method)
		}
	}
}

func TestAnExistingChatIsNotMovedUnlessAskedFor(t *testing.T) {
	// Moving a chat left it opened on a subscription that could not show its history, and
	// dropped its goal. Losing a conversation is worse than the limit the move works
	// around, so the move waits for an opt-in.
	t.Setenv("CODEX_MUX_THREAD_HANDOVER", "")
	if threadHandoverEnabled() {
		t.Fatal("an existing chat must stay where it is by default")
	}
	t.Setenv("CODEX_MUX_THREAD_HANDOVER", "1")
	if !threadHandoverEnabled() {
		t.Fatal("the opt-in must allow the move")
	}
}

func TestSettingAGoalIsStillRecognisedAsATurn(t *testing.T) {
	// The gate is on the move, not on what counts as a turn: a goal runs one, and that
	// stays true so the pre-flight and the in-flight record both cover it.
	if !startsTurn("thread/goal/set") {
		t.Fatal("setting a goal runs a turn")
	}
}
