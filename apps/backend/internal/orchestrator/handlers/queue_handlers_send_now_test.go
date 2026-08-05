package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

type mockQueueSendNow struct {
	sentCount int
	err       error
	calls     int
	sessionID string
	scope     string
	entryID   string
}

func (m *mockQueueSendNow) DrainQueuedMessage(context.Context, string) (bool, error) {
	return false, nil
}

func (m *mockQueueSendNow) SendQueuedNow(_ context.Context, sessionID, scope, entryID string) (int, error) {
	m.calls++
	m.sessionID = sessionID
	m.scope = scope
	m.entryID = entryID
	return m.sentCount, m.err
}

func TestWsSendNowValidatesScopeAndReturnsSentCount(t *testing.T) {
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console", OutputPath: "stderr"})
	require.NoError(t, err)
	dispatcher := &mockQueueSendNow{sentCount: 3}
	events := &mockEventBus{}
	handlers := NewQueueHandlers(messagequeue.NewServiceMemory(log), events, log, dispatcher, allowQueueAccess{})

	response, err := handlers.wsSendNow(context.Background(), createTestMessage(t, ws.ActionMessageQueueSendNow, map[string]interface{}{
		"session_id": "session-1", "scope": orchestrator.QueueSendNowScopeAll,
	}))
	require.NoError(t, err)
	if response.Type != ws.MessageTypeResponse {
		t.Fatalf("response type = %q, want response", response.Type)
	}
	if dispatcher.calls != 1 || dispatcher.sessionID != "session-1" || dispatcher.scope != orchestrator.QueueSendNowScopeAll || dispatcher.entryID != "" {
		t.Fatalf("dispatcher call = %#v", dispatcher)
	}
	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(response.Payload, &payload))
	if payload["session_id"] != "session-1" || payload["dispatched"] != true || payload["sent_count"] != float64(3) {
		t.Fatalf("response payload = %#v", payload)
	}
	if events.published != 1 {
		t.Fatalf("published events = %d, want 1", events.published)
	}

	for _, tc := range []struct {
		name    string
		payload map[string]interface{}
	}{
		{name: "missing entry", payload: map[string]interface{}{"session_id": "s", "scope": orchestrator.QueueSendNowScopeEntry}},
		{name: "entry on all", payload: map[string]interface{}{"session_id": "s", "scope": orchestrator.QueueSendNowScopeAll, "entry_id": "q"}},
		{name: "unknown scope", payload: map[string]interface{}{"session_id": "s", "scope": "selection"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := dispatcher.calls
			response, err := handlers.wsSendNow(context.Background(), createTestMessage(t, ws.ActionMessageQueueSendNow, tc.payload))
			require.NoError(t, err)
			if response.Type != ws.MessageTypeError {
				t.Fatalf("response type = %q, want error", response.Type)
			}
			if dispatcher.calls != before {
				t.Fatalf("dispatcher called for invalid payload")
			}
		})
	}
}

func TestWsSendNowMapsStableErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		code string
	}{
		{name: "entry", err: orchestrator.ErrSendNowEntryNotFound, code: queueErrorCodeEntryNotFound},
		{name: "empty", err: orchestrator.ErrSendNowQueueEmpty, code: queueErrorCodeSendNowQueueEmpty},
		{name: "changed", err: orchestrator.ErrSendNowQueueChanged, code: queueErrorCodeSendNowQueueChanged},
		{name: "conflict", err: orchestrator.ErrSendNowConflict, code: queueErrorCodeSendNowConflict},
		{name: "turn", err: orchestrator.ErrSendNowTurnChanged, code: queueErrorCodeSendNowTurnChanged},
		{name: "attachments", err: messagequeue.ErrSendNowAttachmentOverflow, code: queueErrorCodeSendNowAttachmentOverflow},
		{name: "references", err: messagequeue.ErrSendNowReferenceOverflow, code: queueErrorCodeSendNowReferenceOverflow},
		{name: "not promptable", err: orchestrator.ErrSessionNotPromptable, code: queueErrorCodeNotPromptable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			log, logErr := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console", OutputPath: "stderr"})
			require.NoError(t, logErr)
			dispatcher := &mockQueueSendNow{err: tc.err}
			handlers := NewQueueHandlers(messagequeue.NewServiceMemory(log), &mockEventBus{}, log, dispatcher, allowQueueAccess{})
			response, callErr := handlers.wsSendNow(context.Background(), createTestMessage(t, ws.ActionMessageQueueSendNow, map[string]interface{}{
				"session_id": "session-1", "scope": orchestrator.QueueSendNowScopeAll,
			}))
			require.NoError(t, callErr)
			if response.Type != ws.MessageTypeError {
				t.Fatalf("response type = %q", response.Type)
			}
			if got := parseError(t, response).Code; got != tc.code {
				t.Fatalf("error code = %q, want %q", got, tc.code)
			}
		})
	}

	var _ QueueSendNowDispatcher = (*mockQueueSendNow)(nil)
}
