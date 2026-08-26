package mux

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/developer-nagi/codex-subscription-router-win/internal/state"
)

const importedAuthFixture = `{
  "auth_mode": "chatgpt",
  "OPENAI_API_KEY": null,
  "tokens": {
    "id_token": "id-token-test",
    "access_token": "access-token-test",
    "refresh_token": "refresh-token-test",
    "account_id": "account-import-test"
  },
  "last_refresh": "2026-08-24T00:00:00Z"
}`

func TestImportAccountAuthenticatesOnFirstChildStart(t *testing.T) {
	root := t.TempDir()
	store, err := state.Open(filepath.Join(root, "mux"), filepath.Join(root, "primary"))
	if err != nil {
		t.Fatal(err)
	}
	multiplexer, err := New(Options{
		RealExecutable: os.Args[0],
		RealArgs:       []string{"-test.run=TestImportedAuthHelperProcess"},
		Environment:    append(os.Environ(), "CODEX_MUX_AUTH_HELPER=1"),
		Store:          store,
		Output:         io.Discard,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer multiplexer.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	snapshot, err := multiplexer.ImportAccount(ctx, "Imported", json.RawMessage(importedAuthFixture))
	if err != nil {
		t.Fatal(err)
	}
	if !snapshot.Connected || snapshot.AuthType != "chatgpt" || snapshot.PlanLabel != "Plus" {
		t.Fatalf("unexpected imported account snapshot: %#v", snapshot)
	}
	account, ok := store.Account(snapshot.ID)
	if !ok {
		t.Fatalf("imported account %q missing from store", snapshot.ID)
	}
	info, err := os.Stat(filepath.Join(account.CodexHome, "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	// Windows は POSIX のパーミッションビットを実装しない。os.Chmod は読み取り専用
	// 属性しか変えないため、ここでの保護は親ディレクトリの ACL が担う。
	// ACL はパッチャーが状態ルートへ適用する。
	if info.Mode().Perm() != 0o600 && runtime.GOOS != "windows" {
		t.Fatalf("auth.json mode = %#o, want 0600", info.Mode().Perm())
	}
}

func TestImportedAuthHelperProcess(t *testing.T) {
	if os.Getenv("CODEX_MUX_AUTH_HELPER") != "1" {
		return
	}
	scanner := bufio.NewScanner(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	for scanner.Scan() {
		var request struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if json.Unmarshal(scanner.Bytes(), &request) != nil {
			continue
		}
		result := any(map[string]any{})
		switch request.Method {
		case "account/read":
			authPath := filepath.Join(os.Getenv("CODEX_HOME"), "auth.json")
			if _, err := os.Stat(authPath); err != nil {
				result = map[string]any{"account": nil}
			} else {
				result = map[string]any{"account": map[string]any{
					"type": "chatgpt", "email": "import@example.com", "planType": "plus",
				}}
			}
		case "account/rateLimits/read":
			result = map[string]any{"rateLimits": map[string]any{
				"primary": map[string]any{"usedPercent": 10},
			}}
		}
		_ = encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
	}
	os.Exit(0)
}
