package mux

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
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
// only for the fields that explain routing.
func (t *tracer) traceInbound(accountID string, method string, hasID bool, params json.RawMessage) {
	if !t.enabled() {
		return
	}
	label := method
	if label == "" {
		label = "<response>"
	}
	detail := ""
	if hasID {
		detail = "id=yes"
	}
	switch method {
	case "error":
		var notice errorNotification
		if json.Unmarshal(params, &notice) == nil {
			detail = fmt.Sprintf(
				"thread=%s turn=%s willRetry=%t info=%s",
				notice.ThreadID, notice.TurnID, notice.WillRetry,
				strings.Trim(string(notice.Error.CodexErrorInfo), `"`),
			)
		}
	case "turn/completed", "turn/started":
		if notice, ok := decodeTurnCompleted(params); ok {
			detail = fmt.Sprintf("thread=%s turn=%s status=%s",
				notice.ThreadID, notice.Turn.ID, notice.Turn.Status)
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
