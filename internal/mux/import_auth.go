package mux

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

func (m *Multiplexer) ImportAccount(ctx context.Context, label string, authJSON json.RawMessage) (AccountSnapshot, error) {
	account, err := m.store.AddAccountWithAuth(label, authJSON)
	if err != nil {
		return AccountSnapshot{}, err
	}
	if _, err := m.startChild(ctx, account); err != nil {
		return AccountSnapshot{}, fmt.Errorf("start imported account: %w", err)
	}
	snapshot, err := m.accountSnapshotWithProfile(ctx, account.ID, false)
	if err != nil {
		return AccountSnapshot{}, fmt.Errorf("verify imported account: %w", err)
	}
	if !snapshot.Connected || snapshot.AuthType != "chatgpt" {
		return AccountSnapshot{}, errors.New("imported auth.json did not produce a ChatGPT login")
	}
	m.publish(Event{Type: "account-updated", AccountID: account.ID, Data: snapshot})
	return snapshot, nil
}
