package mux

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/developer-nagi/codex-subscription-router-win/internal/state"
)

func newShareTestMultiplexer(t *testing.T) (*Multiplexer, string, string, string) {
	t.Helper()
	root := t.TempDir()
	sourceHome := filepath.Join(root, "primary")
	store, err := state.Open(filepath.Join(root, "mux"), sourceHome)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	added, err := store.AddAccount("Second subscription")
	if err != nil {
		t.Fatalf("add account: %v", err)
	}
	return &Multiplexer{store: store}, sourceHome, added.CodexHome, added.ID
}

func writeRollout(t *testing.T, home, name, content string) string {
	t.Helper()
	path := filepath.Join(home, "sessions", "2026", "08", "25", name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create session directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write rollout: %v", err)
	}
	return path
}

func TestShareRolloutKeepsTheHistoryInPlaceUnderTheTargetHome(t *testing.T) {
	multiplexer, sourceHome, targetHome, secondID := newShareTestMultiplexer(t)
	sourcePath := writeRollout(t, sourceHome, "rollout-thread.jsonl", "one\n")

	shared, err := multiplexer.shareRolloutWithAccount(sourcePath, "primary", secondID)
	if err != nil {
		t.Fatalf("share rollout: %v", err)
	}
	expected := filepath.Join(targetHome, "sessions", "2026", "08", "25", "rollout-thread.jsonl")
	if shared != expected {
		t.Fatalf("shared path = %q, want %q", shared, expected)
	}
	content, err := os.ReadFile(shared)
	if err != nil {
		t.Fatalf("read shared rollout: %v", err)
	}
	if string(content) != "one\n" {
		t.Fatalf("shared content = %q, want %q", content, "one\n")
	}
}

func TestShareRolloutDoesNotDuplicateTheHistory(t *testing.T) {
	multiplexer, sourceHome, _, secondID := newShareTestMultiplexer(t)
	sourcePath := writeRollout(t, sourceHome, "rollout-thread.jsonl", "one\n")

	shared, err := multiplexer.shareRolloutWithAccount(sourcePath, "primary", secondID)
	if err != nil {
		t.Fatalf("share rollout: %v", err)
	}
	// A history reaches hundreds of megabytes, so the handover must not copy it. One file
	// under two names also keeps the turn the target appends visible to both accounts.
	sourceInfo, err := os.Stat(sourcePath)
	if err != nil {
		t.Fatalf("stat source: %v", err)
	}
	sharedInfo, err := os.Stat(shared)
	if err != nil {
		t.Fatalf("stat shared: %v", err)
	}
	if !os.SameFile(sourceInfo, sharedInfo) {
		t.Fatal("the shared history must be the same file, not a second copy")
	}
	if err := os.WriteFile(sourcePath, []byte("one\ntwo\n"), 0o600); err != nil {
		t.Fatalf("append to source: %v", err)
	}
	content, err := os.ReadFile(shared)
	if err != nil {
		t.Fatalf("read shared rollout: %v", err)
	}
	if string(content) != "one\ntwo\n" {
		t.Fatalf("shared content = %q, want the source's content", content)
	}
}

func TestShareRolloutIsRepeatable(t *testing.T) {
	multiplexer, sourceHome, _, secondID := newShareTestMultiplexer(t)
	sourcePath := writeRollout(t, sourceHome, "rollout-thread.jsonl", "one\n")

	first, err := multiplexer.shareRolloutWithAccount(sourcePath, "primary", secondID)
	if err != nil {
		t.Fatalf("first share: %v", err)
	}
	second, err := multiplexer.shareRolloutWithAccount(sourcePath, "primary", secondID)
	if err != nil {
		t.Fatalf("a chat handed over twice must not fail: %v", err)
	}
	if first != second {
		t.Fatalf("second share = %q, want %q", second, first)
	}
}

func TestShareRolloutRefusesAHistoryOutsideItsSubscription(t *testing.T) {
	multiplexer, _, _, secondID := newShareTestMultiplexer(t)
	outside := filepath.Join(t.TempDir(), "elsewhere.jsonl")
	if err := os.WriteFile(outside, []byte("one\n"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if _, err := multiplexer.shareRolloutWithAccount(outside, "primary", secondID); err == nil {
		t.Fatal("a history outside the subscription's home must not be placed by guesswork")
	}
}

func TestRelativeToHomeAcceptsTheFormsTheAppServerReports(t *testing.T) {
	// The app-server reports a history as an extended-length path, and Windows components
	// disagree about the case of the same directory. Both named the same file as living
	// outside its own subscription, which stopped a chat from being handed over.
	cases := []struct {
		name     string
		home     string
		path     string
		expected string
	}{
		{
			name:     "extended-length path",
			home:     `C:\Users\info\.codex`,
			path:     `\\?\C:\Users\info\.codex\sessions\2026\rollout.jsonl`,
			expected: filepath.Join("sessions", "2026", "rollout.jsonl"),
		},
		{
			name:     "extended-length home",
			home:     `\\?\C:\Users\info\.codex`,
			path:     `C:\Users\info\.codex\sessions\rollout.jsonl`,
			expected: filepath.Join("sessions", "rollout.jsonl"),
		},
		{
			name:     "the same directory in another case",
			home:     `C:\Users\info\.codex`,
			path:     `c:\users\info\.codex\sessions\rollout.jsonl`,
			expected: filepath.Join("sessions", "rollout.jsonl"),
		},
		{
			name:     "plain path",
			home:     `C:\Users\info\.codex`,
			path:     `C:\Users\info\.codex\sessions\rollout.jsonl`,
			expected: filepath.Join("sessions", "rollout.jsonl"),
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			relative, err := relativeToHome(testCase.home, testCase.path)
			if err != nil {
				t.Fatalf("relativeToHome: %v", err)
			}
			if relative != testCase.expected {
				t.Fatalf("relative = %q, want %q", relative, testCase.expected)
			}
		})
	}
}

func TestNormalizeExtendedPathLeavesOrdinaryPathsAlone(t *testing.T) {
	cases := map[string]string{
		`\\?\C:\Users\info\file.jsonl`:    `C:\Users\info\file.jsonl`,
		`\\?\UNC\server\share\file.jsonl`: `\\server\share\file.jsonl`,
		`C:\Users\info\file.jsonl`:        `C:\Users\info\file.jsonl`,
		`/home/info/file.jsonl`:           `/home/info/file.jsonl`,
	}
	for input, expected := range cases {
		if got := normalizeExtendedPath(input); got != expected {
			t.Fatalf("normalizeExtendedPath(%q) = %q, want %q", input, got, expected)
		}
	}
}

func TestRelativeToHomeRejectsEscapes(t *testing.T) {
	if _, err := relativeToHome(`C:\home`, `C:\home`); err == nil {
		t.Fatal("the home itself is not a history file")
	}
	if _, err := relativeToHome(`C:\home`, `C:\home\..\other\rollout.jsonl`); err == nil {
		t.Fatal("a path escaping the home must be refused")
	}
	relative, err := relativeToHome(`C:\home`, `C:\home\sessions\2026\rollout.jsonl`)
	if err != nil {
		t.Fatalf("relativeToHome: %v", err)
	}
	if relative != filepath.Join("sessions", "2026", "rollout.jsonl") {
		t.Fatalf("relative = %q", relative)
	}
}
