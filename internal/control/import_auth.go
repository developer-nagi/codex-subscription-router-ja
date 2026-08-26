package control

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/developer-nagi/codex-subscription-router-win/internal/state"
)

func (s *Server) importAccount(response http.ResponseWriter, request *http.Request) {
	if !s.authorized(request) {
		writeJSON(response, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
		return
	}
	if request.Method != http.MethodPost {
		methodNotAllowed(response)
		return
	}
	request.Body = http.MaxBytesReader(response, request.Body, state.MaxImportedAuthBytes+4096)
	var input struct {
		Label string          `json:"label"`
		Auth  json.RawMessage `json:"auth"`
	}
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeJSON(response, http.StatusBadRequest, map[string]any{"error": "invalid auth.json import request"})
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeJSON(response, http.StatusBadRequest, map[string]any{"error": "invalid trailing import data"})
		return
	}
	if len(input.Auth) == 0 {
		writeJSON(response, http.StatusBadRequest, map[string]any{"error": "auth.json is required"})
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 45*time.Second)
	defer cancel()
	account, err := s.mux.ImportAccount(ctx, input.Label, input.Auth)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, state.ErrDuplicateChatGPTAccount) {
			status = http.StatusConflict
		}
		writeJSON(response, status, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(response, http.StatusCreated, map[string]any{"account": account})
}
