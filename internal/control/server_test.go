package control

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/developer-nagi/codex-subscription-router-ja/internal/mux"
	"github.com/developer-nagi/codex-subscription-router-ja/internal/state"
)

func newRemoveTestServer(t *testing.T) (url string, token string, accountID string) {
	t.Helper()
	root := t.TempDir()
	store, err := state.Open(filepath.Join(root, "mux"), filepath.Join(root, "primary"))
	if err != nil {
		t.Fatal(err)
	}
	added, err := store.AddAccount("Work")
	if err != nil {
		t.Fatal(err)
	}
	multiplexer, err := mux.New(mux.Options{
		RealExecutable: "/usr/bin/true",
		Store:          store,
		Output:         io.Discard,
	})
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	token = "test-token-0123456789abcdef"
	server := New(listener.Addr().String(), token, multiplexer, false)
	go func() { _ = server.Serve(listener) }()
	shutdownContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(func() { _ = server.Shutdown(shutdownContext); cancel() })
	return "http://" + listener.Addr().String(), token, added.ID
}

func TestDeleteAccountRemovesSecondarySubscription(t *testing.T) {
	url, token, accountID := newRemoveTestServer(t)

	deleteRequest, err := http.NewRequest(http.MethodDelete, url+"/v1/accounts/"+accountID, nil)
	if err != nil {
		t.Fatal(err)
	}
	deleteRequest.Header.Set("X-Codex-Mux-Token", token)
	response, err := http.DefaultClient.Do(deleteRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("delete returned %d, want 200", response.StatusCode)
	}

	accountsRequest, err := http.NewRequest(http.MethodGet, url+"/v1/accounts", nil)
	if err != nil {
		t.Fatal(err)
	}
	accountsRequest.Header.Set("X-Codex-Mux-Token", token)
	listResponse, err := http.DefaultClient.Do(accountsRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer listResponse.Body.Close()
	body, _ := io.ReadAll(listResponse.Body)
	var listed struct {
		Accounts []struct {
			ID string `json:"id"`
		} `json:"accounts"`
	}
	if err := json.Unmarshal(body, &listed); err != nil {
		t.Fatalf("decode accounts response: %v", err)
	}
	for _, account := range listed.Accounts {
		if account.ID == accountID {
			t.Fatalf("removed account still listed: %s", body)
		}
	}
}

func TestDeleteAccountRejectsPrimaryAndBadToken(t *testing.T) {
	url, token, _ := newRemoveTestServer(t)

	primaryRequest, err := http.NewRequest(http.MethodDelete, url+"/v1/accounts/primary", nil)
	if err != nil {
		t.Fatal(err)
	}
	primaryRequest.Header.Set("X-Codex-Mux-Token", token)
	response, err := http.DefaultClient.Do(primaryRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("primary delete returned %d, want 400", response.StatusCode)
	}

	unauthorized, err := http.NewRequest(http.MethodDelete, url+"/v1/accounts/primary", nil)
	if err != nil {
		t.Fatal(err)
	}
	unauthorized.Header.Set("X-Codex-Mux-Token", "wrong-token-0123456789")
	denied, err := http.DefaultClient.Do(unauthorized)
	if err != nil {
		t.Fatal(err)
	}
	defer denied.Body.Close()
	if denied.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated delete returned %d, want 401", denied.StatusCode)
	}
}

// 注入したレンダラーは全ての非公開経路へ X-Codex-Mux-Token を付ける。CORS の
// preflight でこのヘッダーを許可しないと、アカウント UI からの通信が全て失敗する。
// 上流 PR #21 はこのヘッダーを取りこぼしていたため、回帰を検知できるようにする。
func TestPreflightAllowsControlTokenHeader(t *testing.T) {
	url, _, _ := newRemoveTestServer(t)

	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		request, err := http.NewRequest(http.MethodOptions, url+"/v1/accounts", nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Origin", "app://-")
		request.Header.Set("Access-Control-Request-Method", method)
		request.Header.Set("Access-Control-Request-Headers", "x-codex-mux-token")

		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()

		allowedHeaders := response.Header.Get("Access-Control-Allow-Headers")
		if !strings.Contains(strings.ToLower(allowedHeaders), "x-codex-mux-token") {
			t.Fatalf("preflight for %s allows headers %q, want the control token header", method, allowedHeaders)
		}
		allowedMethods := response.Header.Get("Access-Control-Allow-Methods")
		if !strings.Contains(allowedMethods, method) {
			t.Fatalf("preflight allows methods %q, want %s", allowedMethods, method)
		}
		if origin := response.Header.Get("Access-Control-Allow-Origin"); origin != "app://-" {
			t.Fatalf("preflight allows origin %q, want the app origin", origin)
		}
	}
}
