package mux

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/developer-nagi/codex-subscription-router-win/internal/state"
)

const (
	profileURL      = "https://chatgpt.com/backend-api/wham/profiles/me"
	profileCacheTTL = 10 * time.Minute
	profileMaxBytes = 1 << 20
)

type profileCacheEntry struct {
	imageURL  string
	expiresAt time.Time
}

type authFile struct {
	Tokens struct {
		AccessToken string `json:"access_token"`
		AccountID   string `json:"account_id"`
	} `json:"tokens"`
}

type profileResponse struct {
	Profile struct {
		ProfilePictureURL string `json:"profile_picture_url"`
	} `json:"profile"`
}

func (m *Multiplexer) profileImageURL(ctx context.Context, account state.Account) string {
	now := time.Now()
	m.profileMu.Lock()
	cached, ok := m.profileCache[account.ID]
	m.profileMu.Unlock()
	if ok && now.Before(cached.expiresAt) {
		return cached.imageURL
	}

	imageURL, err := fetchProfileImageURL(
		ctx,
		m.profileClient,
		profileURL,
		filepath.Join(account.CodexHome, "auth.json"),
	)
	if err != nil {
		return ""
	}
	m.profileMu.Lock()
	m.profileCache[account.ID] = profileCacheEntry{
		imageURL:  imageURL,
		expiresAt: now.Add(profileCacheTTL),
	}
	m.profileMu.Unlock()
	return imageURL
}

func fetchProfileImageURL(
	ctx context.Context,
	client *http.Client,
	endpoint string,
	authPath string,
) (string, error) {
	credentials, err := readAuthFile(authPath)
	if err != nil {
		return "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("create profile request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+credentials.Tokens.AccessToken)
	if credentials.Tokens.AccountID != "" {
		request.Header.Set("ChatGPT-Account-ID", credentials.Tokens.AccountID)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "Codex Subscription Router")

	response, err := client.Do(request)
	if err != nil {
		return "", fmt.Errorf("fetch profile: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, profileMaxBytes))
		return "", fmt.Errorf("fetch profile: status %d", response.StatusCode)
	}
	var profile profileResponse
	decoder := json.NewDecoder(io.LimitReader(response.Body, profileMaxBytes))
	if err := decoder.Decode(&profile); err != nil {
		return "", fmt.Errorf("decode profile: %w", err)
	}
	return validatedProfileImageURL(profile.Profile.ProfilePictureURL)
}

func readAuthFile(path string) (authFile, error) {
	file, err := os.Open(path)
	if err != nil {
		return authFile{}, fmt.Errorf("open account credentials: %w", err)
	}
	defer file.Close()
	var credentials authFile
	decoder := json.NewDecoder(io.LimitReader(file, profileMaxBytes))
	if err := decoder.Decode(&credentials); err != nil {
		return authFile{}, fmt.Errorf("decode account credentials: %w", err)
	}
	if credentials.Tokens.AccessToken == "" {
		return authFile{}, errors.New("account access token is unavailable")
	}
	return credentials, nil
}

func validatedProfileImageURL(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return "", errors.New("profile image URL is not HTTPS")
	}
	return parsed.String(), nil
}
