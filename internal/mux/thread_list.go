package mux

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/developer-nagi/codex-subscription-router-win/internal/protocol"
)

func (m *Multiplexer) aggregateThreadList(request protocol.Message) {
	entries := m.childEntries()
	type result struct {
		accountID string
		threads   []map[string]any
	}
	results := make(chan result, len(entries))
	var wait sync.WaitGroup
	for _, entry := range entries {
		wait.Add(1)
		go func(entry childEntry) {
			defer wait.Done()
			results <- result{accountID: entry.account.ID, threads: m.listAllThreads(entry, request.Params)}
		}(entry)
	}
	wait.Wait()
	close(results)

	// A chat handed to another subscription is readable from both homes, because the
	// handover shares one history file rather than copying it. List it once, as the
	// subscription that owns it, and never let the listing decide ownership for a chat
	// that already has an owner: the accounts answer concurrently, so that would reassign
	// the chat at random and undo the handover.
	threads := make([]map[string]any, 0)
	position := make(map[string]int)
	for accountResult := range results {
		for _, thread := range accountResult.threads {
			threadID, ok := thread["id"].(string)
			if !ok {
				threads = append(threads, thread)
				continue
			}
			owner, known := m.store.ThreadOwner(threadID)
			if !known {
				_ = m.store.SetThreadOwner(threadID, accountResult.accountID)
				owner = accountResult.accountID
			}
			if index, seen := position[threadID]; seen {
				if owner == accountResult.accountID {
					threads[index] = thread
				}
				continue
			}
			position[threadID] = len(threads)
			threads = append(threads, thread)
		}
	}
	sortThreads(threads)
	encoded, err := json.Marshal(map[string]any{"data": threads, "nextCursor": nil})
	if err != nil {
		m.write(protocol.Failure(request.ID, -32603, "failed to merge thread list"))
		return
	}
	m.write(protocol.Success(request.ID, encoded))
}

func (m *Multiplexer) listAllThreads(entry childEntry, originalParams json.RawMessage) []map[string]any {
	var params map[string]any
	if json.Unmarshal(originalParams, &params) != nil {
		params = make(map[string]any)
	}
	params["limit"] = 500
	threads := make([]map[string]any, 0)
	seenCursors := make(map[string]struct{})
	var cursor string
	for {
		if cursor == "" {
			params["cursor"] = nil
		} else {
			params["cursor"] = cursor
		}
		encodedParams, _ := json.Marshal(params)
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		response, err := entry.child.Request(ctx, "thread/list", encodedParams)
		cancel()
		if err != nil {
			return threads
		}
		var decoded struct {
			Data       []map[string]any `json:"data"`
			NextCursor *string          `json:"nextCursor"`
		}
		if json.Unmarshal(response.Result, &decoded) != nil {
			return threads
		}
		threads = append(threads, decoded.Data...)
		if decoded.NextCursor == nil || *decoded.NextCursor == "" {
			return threads
		}
		cursor = *decoded.NextCursor
		if _, repeated := seenCursors[cursor]; repeated {
			return threads
		}
		seenCursors[cursor] = struct{}{}
	}
}
