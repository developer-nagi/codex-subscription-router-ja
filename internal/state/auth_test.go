package state

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const validImportedAuth = `{
  "auth_mode": "chatgpt",
  "OPENAI_API_KEY": null,
  "tokens": {
    "id_token": "id-token-test",
    "access_token": "access-token-test",
    "refresh_token": "refresh-token-test",
    "account_id": "account-test"
  },
  "last_refresh": "2026-08-24T00:00:00Z"
}`

func TestValidateImportedAuthAcceptsNativeRefreshableBundle(t *testing.T) {
	normalized, accountID, err := validateImportedAuth([]byte(validImportedAuth))
	if err != nil {
		t.Fatal(err)
	}
	if accountID != "account-test" {
		t.Fatalf("account ID = %q", accountID)
	}
	if !strings.HasSuffix(string(normalized), "\n") || !strings.Contains(string(normalized), `"refresh_token": "refresh-token-test"`) {
		t.Fatalf("unexpected normalized auth: %s", normalized)
	}
}

func TestValidateImportedAuthRejectsUnsafeOrIncompleteShapes(t *testing.T) {
	tests := map[string]string{
		"api key":          strings.Replace(validImportedAuth, `"OPENAI_API_KEY": null`, `"OPENAI_API_KEY": "sk-test"`, 1),
		"wrong mode":       strings.Replace(validImportedAuth, `"chatgpt"`, `"apikey"`, 1),
		"no refresh":       strings.Replace(validImportedAuth, `"refresh_token": "refresh-token-test",`, ``, 1),
		"unknown root":     strings.Replace(validImportedAuth, `"last_refresh":`, `"extra": true, "last_refresh":`, 1),
		"unknown token":    strings.Replace(validImportedAuth, `"account_id":`, `"extra": true, "account_id":`, 1),
		"bad refresh time": strings.Replace(validImportedAuth, `"2026-08-24T00:00:00Z"`, `"yesterday"`, 1),
		"trailing JSON":    validImportedAuth + `{}`,
	}
	for name, contents := range tests {
		t.Run(name, func(t *testing.T) {
			if _, _, err := validateImportedAuth([]byte(contents)); err == nil {
				t.Fatal("invalid auth.json unexpectedly passed validation")
			}
		})
	}
}

func TestAddAccountWithAuthWritesPrivateFileAndRejectsDuplicate(t *testing.T) {
	root := t.TempDir()
	store, err := Open(filepath.Join(root, "mux"), filepath.Join(root, "primary"))
	if err != nil {
		t.Fatal(err)
	}
	account, err := store.AddAccountWithAuth("Imported", []byte(validImportedAuth))
	if err != nil {
		t.Fatal(err)
	}
	authPath := filepath.Join(account.CodexHome, "auth.json")
	info, err := os.Stat(authPath)
	if err != nil {
		t.Fatal(err)
	}
	// Windows は POSIX のパーミッションビットを実装しない。os.Chmod は読み取り専用
	// 属性しか変えないため、ここでの保護は親ディレクトリの ACL が担う。
	// ACL はパッチャーが状態ルートへ適用する。
	if mode := info.Mode().Perm(); runtime.GOOS != "windows" && mode != 0o600 {
		t.Fatalf("auth.json mode = %#o, want 0600", mode)
	}
	if got := authAccountID(authPath); got != "account-test" {
		t.Fatalf("stored account ID = %q", got)
	}
	if _, err := store.AddAccountWithAuth("Duplicate", []byte(validImportedAuth)); err == nil {
		t.Fatal("duplicate ChatGPT account unexpectedly imported")
	}
	if accounts := store.Accounts(); len(accounts) != 2 {
		t.Fatalf("account count = %d, want primary plus one imported", len(accounts))
	}
}

func TestAddAccountWithAuthRejectsPrimaryAccountID(t *testing.T) {
	root := t.TempDir()
	primary := filepath.Join(root, "primary")
	if err := os.MkdirAll(primary, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(primary, "auth.json"), []byte(validImportedAuth), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := Open(filepath.Join(root, "mux"), primary)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddAccountWithAuth("Duplicate Primary", []byte(validImportedAuth)); !errors.Is(err, ErrDuplicateChatGPTAccount) {
		t.Fatalf("duplicate primary error = %v, want ErrDuplicateChatGPTAccount", err)
	}
	if accounts := store.Accounts(); len(accounts) != 1 {
		t.Fatalf("account count = %d after duplicate primary import, want 1", len(accounts))
	}
}
