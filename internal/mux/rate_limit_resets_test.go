package mux

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/developer-nagi/codex-subscription-router-ja/internal/state"
)

func TestFetchRateLimitResetCreditsUsesSelectedAccountCredentials(t *testing.T) {
	home := t.TempDir()
	writeResetTestAuth(t, home)

	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", request.Method)
		}
		if got := request.Header.Get("Authorization"); got != "Bearer secret-token" {
			t.Fatalf("authorization = %q", got)
		}
		if got := request.Header.Get("ChatGPT-Account-ID"); got != "chatgpt-account" {
			t.Fatalf("ChatGPT-Account-ID = %q", got)
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"available_count":1,"credits":[]}`))
	}))
	defer server.Close()

	result, err := fetchRateLimitResetCredits(context.Background(), server.Client(), server.URL, state.Account{CodexHome: home})
	if err != nil {
		t.Fatal(err)
	}
	if string(result) != `{"available_count":1,"credits":[]}` {
		t.Fatalf("unexpected response: %s", result)
	}
}

func TestConsumeRateLimitResetCreditsForwardsOnlyExpectedPayload(t *testing.T) {
	home := t.TempDir()
	writeResetTestAuth(t, home)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["credit_id"] != "credit-1" || body["redeem_request_id"] != "request-1" || len(body) != 2 {
			t.Fatalf("unexpected body: %#v", body)
		}
		_, _ = response.Write([]byte(`{"code":"reset","credit":{"id":"credit-1"}}`))
	}))
	defer server.Close()

	creditID := "credit-1"
	body, _ := json.Marshal(consumeResetCreditInput{CreditID: &creditID, RedeemRequestID: "request-1"})
	result, err := requestRateLimitResetCredits(
		context.Background(), server.Client(), server.URL, http.MethodPost,
		state.Account{CodexHome: home}, body,
	)
	if err != nil {
		t.Fatal(err)
	}
	if string(result) != `{"code":"reset","credit":{"id":"credit-1"}}` {
		t.Fatalf("unexpected response: %s", result)
	}
}

func TestPreviewResetCreditsUsesNativeCreditShape(t *testing.T) {
	result := previewResetCredits(ResetCreditsPreview{AccountID: "primary", AvailableCount: 2})
	var decoded struct {
		AvailableCount int `json:"available_count"`
		Credits        []struct {
			ID        string `json:"id"`
			Status    string `json:"status"`
			Title     string `json:"title"`
			ExpiresAt string `json:"expires_at"`
		} `json:"credits"`
	}
	if err := json.Unmarshal(result, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.AvailableCount != 2 || len(decoded.Credits) != 2 {
		t.Fatalf("unexpected preview: %#v", decoded)
	}
	for _, credit := range decoded.Credits {
		if credit.ID == "" || credit.Status != "available" || credit.Title == "" || credit.ExpiresAt == "" {
			t.Fatalf("incomplete native credit: %#v", credit)
		}
	}
}

func TestDecodeResetCreditMetadataUsesApplicableCountAndEarliestExpiry(t *testing.T) {
	metadata, err := decodeResetCreditMetadata(json.RawMessage(`{
		"available_count": 3,
		"applicable_available_count": 2,
		"credits": [
			{"status":"available","expires_at":"2026-09-10T12:00:00Z"},
			{"status":"consumed","expires_at":"2026-08-01T12:00:00Z"},
			{"status":"available","expires_at":"2026-09-01T12:00:00Z"}
		]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	wantExpiry := time.Date(2026, time.September, 1, 12, 0, 0, 0, time.UTC).Unix()
	if !metadata.Known || metadata.AvailableCount != 2 || metadata.EarliestExpiry == nil || *metadata.EarliestExpiry != wantExpiry {
		t.Fatalf("unexpected reset metadata: %#v", metadata)
	}
}

func TestRoutingResetCreditsCachesSuccessfulResponse(t *testing.T) {
	root := t.TempDir()
	primaryHome := filepath.Join(root, "primary")
	if err := os.MkdirAll(primaryHome, 0o700); err != nil {
		t.Fatal(err)
	}
	writeResetTestAuth(t, primaryHome)
	store, err := state.Open(filepath.Join(root, "mux"), primaryHome)
	if err != nil {
		t.Fatal(err)
	}
	account, ok := store.Account("primary")
	if !ok {
		t.Fatal("primary account was not created")
	}

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requests++
		_, _ = response.Write([]byte(`{"available_count":1,"applicable_available_count":1,"credits":[]}`))
	}))
	defer server.Close()

	multiplexer, err := New(Options{
		RealExecutable: "codex", Store: store, Output: io.Discard,
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.August, 16, 12, 0, 0, 0, time.UTC)
	multiplexer.now = func() time.Time { return now }
	multiplexer.profileClient = server.Client()
	multiplexer.resetCreditsEndpoint = server.URL

	first := multiplexer.routingResetCredits(context.Background(), account)
	second := multiplexer.routingResetCredits(context.Background(), account)
	if !first.Known || first.AvailableCount != 1 || second.AvailableCount != 1 || requests != 1 {
		t.Fatalf("cache miss: first=%#v second=%#v requests=%d", first, second, requests)
	}

	now = now.Add(resetCreditsCacheTTL + time.Second)
	third := multiplexer.routingResetCredits(context.Background(), account)
	if third.AvailableCount != 1 || requests != 2 {
		t.Fatalf("expired cache was not refreshed: third=%#v requests=%d", third, requests)
	}
}

func writeResetTestAuth(t *testing.T, home string) {
	t.Helper()
	payload := []byte(`{"tokens":{"access_token":"secret-token","account_id":"chatgpt-account"}}`)
	if err := os.WriteFile(filepath.Join(home, "auth.json"), payload, 0o600); err != nil {
		t.Fatal(err)
	}
}
