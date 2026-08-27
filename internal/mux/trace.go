package mux

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/developer-nagi/codex-subscription-router-win/internal/protocol"
)

// The multiplexer is silent by design, which makes a routing question hard to answer
// from a running installation. Setting CODEX_MUX_TRACE to a file path records the
// method names crossing the multiplexer, and nothing else: no prompt text, no results,
// no credentials. Only a thread id, an error name, and a subscription id are written.
type tracer struct {
	mu   sync.Mutex
	file *os.File
}

var trace = newTracer()

func newTracer() *tracer {
	path := strings.TrimSpace(os.Getenv("CODEX_MUX_TRACE"))
	if path == "" {
		return &tracer{}
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return &tracer{}
	}
	return &tracer{file: file}
}

func (t *tracer) enabled() bool { return t != nil && t.file != nil }

func (t *tracer) write(direction, accountID, method, detail string) {
	if !t.enabled() {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	line := fmt.Sprintf(
		"%s %-3s %-18s %s", time.Now().Format("15:04:05.000"), direction, accountID, method,
	)
	if detail != "" {
		line += " " + detail
	}
	fmt.Fprintln(t.file, line)
}

// traceInbound records a message coming back from a subscription. Params are inspected
// only for the fields that explain routing, and an error is recorded because a request
// that fails looks identical to one that succeeds otherwise.
func (t *tracer) traceInbound(accountID string, message protocol.Message) {
	if !t.enabled() {
		return
	}
	method, params := message.Method, message.Params
	label := method
	if label == "" {
		label = "<response>"
	}
	detail := ""
	if len(message.ID) > 0 {
		detail = "id=" + strings.Trim(string(message.ID), `"`)
	}
	if message.Error != nil {
		detail += fmt.Sprintf(" ERROR code=%d message=%q data=%s",
			message.Error.Code, message.Error.Message,
			truncate(string(message.Error.Data), 300))
	}
	// An "error" notification is how a subscription reports that a turn failed, and its
	// reason is the useful part. It is decoded here rather than anywhere else because
	// nothing else needs it: a chat is never moved between subscriptions.
	if method == "error" {
		var notice struct {
			ThreadID  string `json:"threadId"`
			TurnID    string `json:"turnId"`
			WillRetry bool   `json:"willRetry"`
			Error     struct {
				CodexErrorInfo json.RawMessage `json:"codexErrorInfo"`
			} `json:"error"`
		}
		if json.Unmarshal(params, &notice) == nil {
			detail = fmt.Sprintf(
				"thread=%s turn=%s willRetry=%t info=%s",
				notice.ThreadID, notice.TurnID, notice.WillRetry,
				strings.Trim(string(notice.Error.CodexErrorInfo), `"`),
			)
		}
	}
	t.write("in", accountID, label, detail)
}

// traceOutbound records a request the multiplexer sends to a subscription.
func (t *tracer) traceOutbound(accountID, method string, params json.RawMessage) {
	if !t.enabled() {
		return
	}
	detail := ""
	if threadID := threadIDFromParams(params); threadID != "" {
		detail = "thread=" + threadID
	}
	t.write("out", accountID, method, detail)
}

func (t *tracer) note(accountID, what, detail string) {
	t.write("--", accountID, what, detail)
}

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "..."
}
