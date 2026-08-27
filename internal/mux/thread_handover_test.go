package mux

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestResumeDeadlineFollowsTheHistoryItHasToRead(t *testing.T) {
	directory := t.TempDir()
	small := filepath.Join(directory, "small.jsonl")
	if err := os.WriteFile(small, make([]byte, 1<<10), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	shortWait := resumeDeadline(small)
	if shortWait < 30*time.Second || shortWait > 31*time.Second {
		t.Fatalf("a small history waits %s, want about the base wait", shortWait)
	}

	// Rebuilding a chat's turns re-reads the whole history, so a long chat needs more
	// than a fixed request budget. Abandoning it half way is what left a chat opened on
	// a subscription that could not show it.
	large := filepath.Join(directory, "large.jsonl")
	if err := os.WriteFile(large, make([]byte, 64<<20), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	longWait := resumeDeadline(large)
	if longWait <= shortWait {
		t.Fatalf("a 64 MB history waits %s, no longer than a 1 KB one", longWait)
	}
	if longWait > 10*time.Minute {
		t.Fatalf("wait of %s exceeds the ceiling", longWait)
	}
}

func TestResumeDeadlineIsCappedAndSurvivesAMissingFile(t *testing.T) {
	if got := resumeDeadline(filepath.Join(t.TempDir(), "absent.jsonl")); got != 30*time.Second {
		t.Fatalf("a history that cannot be measured waits %s, want the base wait", got)
	}
	huge := filepath.Join(t.TempDir(), "huge.jsonl")
	file, err := os.Create(huge)
	if err != nil {
		t.Fatalf("create file: %v", err)
	}
	// Sparse, so the ceiling can be exercised without writing gigabytes.
	if err := file.Truncate(8 << 30); err != nil {
		file.Close()
		t.Skipf("cannot size a sparse file here: %v", err)
	}
	file.Close()
	if got := resumeDeadline(huge); got != 10*time.Minute {
		t.Fatalf("an 8 GB history waits %s, want the ceiling", got)
	}
}
