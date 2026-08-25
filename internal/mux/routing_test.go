package mux

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/developer-nagi/codex-subscription-router-ja/internal/protocol"
)

func TestIsUsageLimitResponseRecognizesStructuredError(t *testing.T) {
	message := protocol.Message{Error: &protocol.RPCError{
		Code:    -32000,
		Message: "turn failed",
		Data:    json.RawMessage(`{"codexErrorInfo":"usage_limit_exceeded"}`),
	}}
	if !isUsageLimitResponse(message) {
		t.Fatal("expected usage-limit error to be recognized")
	}
}

func TestIsUsageLimitResponseIgnoresUnrelatedError(t *testing.T) {
	message := protocol.Message{Error: &protocol.RPCError{
		Code:    -32000,
		Message: "workspace folder is unavailable",
	}}
	if isUsageLimitResponse(message) {
		t.Fatal("unrelated error was misclassified as a usage limit")
	}
}

func TestAllSubscriptionsDepletedUsesActionableMessage(t *testing.T) {
	message := allSubscriptionsDepleted(json.RawMessage(`7`), nil)
	if message.Error == nil || message.Error.Code != -32026 {
		t.Fatalf("unexpected error response: %#v", message)
	}
	if message.Error.Message != "接続中のすべてのサブスクリプションの利用枠を使い切りました。別のサブスクリプションを追加するか、利用枠のリセットをお待ちください。" {
		t.Fatalf("unexpected depletion message: %q", message.Error.Message)
	}
}

func TestAllSubscriptionsDepletedShowsKnownResetTime(t *testing.T) {
	reset := time.Date(2026, time.August, 16, 10, 30, 0, 0, time.Local).Unix()
	message := allSubscriptionsDepleted(json.RawMessage(`7`), &reset)
	if message.Error == nil {
		t.Fatal("expected an error response")
	}
	want := "接続中のすべてのサブスクリプションの利用枠を使い切りました。8月16日(日) 10:30 に利用枠がリセットされます。"
	if message.Error.Message != want {
		t.Fatalf("unexpected reset message: %q", message.Error.Message)
	}
}
