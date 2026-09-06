package handlers

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func queueCensusHandler(t *testing.T) (*Handlers, *messagequeue.Service) {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console", OutputPath: "stderr"})
	if err != nil {
		t.Fatalf("create logger: %v", err)
	}
	queue := messagequeue.NewServiceMemory(log)
	queue.SetAutoMergeEnabled(false)
	return &Handlers{queueManager: queue, logger: log}, queue
}

func queuePrincipal(workspaceID, taskID, sessionID string) context.Context {
	return mcpscope.WithPrincipal(context.Background(), mcpscope.Principal{
		WorkspaceID: workspaceID, CallerTaskID: taskID, CallerSessionID: sessionID,
		Surface: mcpprofile.SurfaceKanbanTask,
	})
}

func TestHandleGetMessageQueueCensusBindsTrustedCallerTaskAndSession(t *testing.T) {
	h, queue := queueCensusHandler(t)
	entry, err := queue.QueueMessage(context.Background(), "session-1", "task-1", "secret body", "", messagequeue.QueuedByAgent, false, nil)
	if err != nil {
		t.Fatalf("queue: %v", err)
	}
	msg := makeWSMessage(t, ws.ActionMCPGetMessageQueueCensus, map[string]interface{}{
		"task_id": "task-1", "session_id": "session-1",
	})
	resp, err := h.handleGetMessageQueueCensus(queuePrincipal("workspace-1", "task-1", "session-1"), msg)
	if err != nil {
		t.Fatalf("handle census: %v", err)
	}
	var payload struct {
		TaskID    string                          `json:"task_id"`
		SessionID string                          `json:"session_id"`
		Entries   []messagequeue.QueueCensusEntry `json:"entries"`
	}
	if err := resp.ParsePayload(&payload); err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if payload.TaskID != "task-1" || payload.SessionID != "session-1" || len(payload.Entries) != 1 || payload.Entries[0].ID != entry.ID {
		t.Fatalf("payload = %#v", payload)
	}
	if bytes, _ := json.Marshal(payload); string(bytes) == "" || containsJSONBody(bytes, "secret body") {
		t.Fatalf("census leaked message body: %s", bytes)
	}
}

func TestHandleMessageQueueCensusRejectsSpoofedOrUnscopedIdentity(t *testing.T) {
	h, _ := queueCensusHandler(t)
	tests := []struct {
		name string
		ctx  context.Context
		body map[string]interface{}
	}{
		{name: "missing principal", ctx: context.Background(), body: map[string]interface{}{"task_id": "task-1", "session_id": "session-1"}},
		{name: "missing workspace scope", ctx: queuePrincipal("", "task-1", "session-1"), body: map[string]interface{}{"task_id": "task-1", "session_id": "session-1"}},
		{name: "different task", ctx: queuePrincipal("workspace-1", "task-1", "session-1"), body: map[string]interface{}{"task_id": "task-2", "session_id": "session-1"}},
		{name: "different session", ctx: queuePrincipal("workspace-1", "task-1", "session-1"), body: map[string]interface{}{"task_id": "task-1", "session_id": "session-2"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := h.handleGetMessageQueueCensus(tt.ctx, makeWSMessage(t, ws.ActionMCPGetMessageQueueCensus, tt.body))
			if err != nil {
				t.Fatalf("handle census: %v", err)
			}
			assertWSError(t, resp, ws.ErrorCodeForbidden)
		})
	}
}

func TestHandleDisposeMessageQueueEntriesReturnsPerEntryOutcomes(t *testing.T) {
	h, queue := queueCensusHandler(t)
	ctx := queuePrincipal("workspace-1", "task-1", "session-1")
	entry, err := queue.QueueMessage(ctx, "session-1", "task-1", "remove me", "", messagequeue.QueuedByAgent, false, nil)
	if err != nil {
		t.Fatalf("queue: %v", err)
	}
	census, err := queue.Census(ctx, "session-1")
	if err != nil {
		t.Fatalf("census: %v", err)
	}
	resp, err := h.handleDisposeMessageQueueEntries(ctx, makeWSMessage(t, ws.ActionMCPDisposeMessageQueueEntries, map[string]interface{}{
		"task_id": "task-1", "session_id": "session-1",
		"entries": []map[string]interface{}{{"id": entry.ID, "claim": census.Entries[0].Claim}},
	}))
	if err != nil {
		t.Fatalf("dispose: %v", err)
	}
	var payload struct {
		BeforeCount int                                    `json:"before_count"`
		AfterCount  int                                    `json:"after_count"`
		Outcomes    []messagequeue.QueueDispositionOutcome `json:"outcomes"`
	}
	if err := resp.ParsePayload(&payload); err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if payload.BeforeCount != 1 || payload.AfterCount != 0 || len(payload.Outcomes) != 1 || payload.Outcomes[0].Status != messagequeue.QueueDispositionRemoved {
		t.Fatalf("payload = %#v", payload)
	}
}

func containsJSONBody(data []byte, body string) bool {
	return body != "" && json.Valid(data) && strings.Contains(string(data), body)
}
