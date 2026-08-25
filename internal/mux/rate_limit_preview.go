package mux

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/developer-nagi/codex-subscription-router-ja/internal/protocol"
)

type RateLimitPreviewMode string

const (
	RateLimitPreviewClear            RateLimitPreviewMode = "clear"
	RateLimitPreviewSingleDepleted   RateLimitPreviewMode = "single_depleted"
	RateLimitPreviewAllDepleted      RateLimitPreviewMode = "all_depleted"
	RateLimitPreviewAllDepletedReset RateLimitPreviewMode = "all_depleted_reset"
)

type RateLimitPreview struct {
	Mode      RateLimitPreviewMode `json:"mode"`
	AccountID string               `json:"accountId,omitempty"`
	ResetsAt  *int64               `json:"resetsAt,omitempty"`
}

func (mode RateLimitPreviewMode) isAllDepleted() bool {
	return mode == RateLimitPreviewAllDepleted || mode == RateLimitPreviewAllDepletedReset
}

func (m *Multiplexer) SetRateLimitPreview(ctx context.Context, preview RateLimitPreview) error {
	switch preview.Mode {
	case RateLimitPreviewClear:
		m.previewMu.Lock()
		m.rateLimitPreview = nil
		m.previewMu.Unlock()
	case RateLimitPreviewSingleDepleted:
		if preview.AccountID == "" {
			return errors.New("accountId is required for single_depleted")
		}
		if _, ok := m.store.Account(preview.AccountID); !ok {
			return errors.New("preview account was not found")
		}
		m.setRateLimitPreview(preview)
	case RateLimitPreviewAllDepleted:
		preview.AccountID = ""
		preview.ResetsAt = nil
		m.setRateLimitPreview(preview)
	case RateLimitPreviewAllDepletedReset:
		preview.AccountID = ""
		if preview.ResetsAt == nil {
			reset := time.Now().Add(24 * time.Hour).Unix()
			preview.ResetsAt = &reset
		}
		m.setRateLimitPreview(preview)
	default:
		return errors.New("unsupported rate-limit preview mode")
	}

	limits, err := m.AggregatedRateLimits(ctx)
	if err != nil {
		return err
	}
	params, err := json.Marshal(map[string]any{"rateLimits": limits})
	if err != nil {
		return err
	}
	m.write(protocol.Message{Method: "account/rateLimits/updated", Params: params})
	m.publish(Event{Type: "account-updated", Message: "Rate-limit preview changed"})
	return nil
}

func (m *Multiplexer) setRateLimitPreview(preview RateLimitPreview) {
	copy := preview
	if preview.ResetsAt != nil {
		reset := *preview.ResetsAt
		copy.ResetsAt = &reset
	}
	m.previewMu.Lock()
	m.rateLimitPreview = &copy
	m.previewMu.Unlock()
}

func (m *Multiplexer) currentRateLimitPreview() *RateLimitPreview {
	m.previewMu.RLock()
	defer m.previewMu.RUnlock()
	if m.rateLimitPreview == nil {
		return nil
	}
	copy := *m.rateLimitPreview
	if copy.ResetsAt != nil {
		reset := *copy.ResetsAt
		copy.ResetsAt = &reset
	}
	return &copy
}

func (m *Multiplexer) applyRateLimitPreview(snapshot *AccountSnapshot) {
	preview := m.currentRateLimitPreview()
	if preview == nil || !snapshot.Connected || snapshot.AuthType != "chatgpt" {
		return
	}
	if preview.Mode == RateLimitPreviewSingleDepleted && snapshot.ID != preview.AccountID {
		return
	}
	if preview.Mode != RateLimitPreviewSingleDepleted && !preview.Mode.isAllDepleted() {
		return
	}

	shortDuration := int64(300)
	weeklyDuration := int64(10_080)
	limits := &RateLimits{
		Primary: &RateLimitWindow{
			UsedPercent:        100,
			WindowDurationMins: &shortDuration,
			ResetsAt:           preview.ResetsAt,
		},
		Secondary: &RateLimitWindow{
			UsedPercent:        100,
			WindowDurationMins: &weeklyDuration,
			ResetsAt:           preview.ResetsAt,
		},
	}
	if preview.Mode.isAllDepleted() {
		limits.RateLimitReachedType = "legacy_rate_limit_reached"
	}
	snapshot.RateLimits = limits
}
