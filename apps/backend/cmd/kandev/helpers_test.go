package main

import (
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func decodePayload(t *testing.T, raw json.RawMessage) map[string]interface{} {
	t.Helper()
	var payload map[string]interface{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("failed to decode payload: %v", err)
	}
	return payload
}

// TestAppendSessionStateMessage_IncludesTaskEnvironmentID asserts the snapshot
// the WS hub sends on `session.subscribe` carries `task_environment_id`.
//
// Why this matters: PR #758 routes shell terminals by environment, and the
// frontend reads `environmentIdBySessionId` from `session.state_changed`
// payloads to populate that map. If the subscribe snapshot omits it,
// late-subscribing clients (page reload, task switch, WS reconnect) leave
// `environmentId=null` for the active session and the terminal panel hangs
// on "Connecting terminal..." forever.
func TestAppendSessionStateMessage_IncludesTaskEnvironmentID(t *testing.T) {
	session := &models.TaskSession{
		ID:                "sess-1",
		TaskID:            "task-1",
		State:             models.TaskSessionStateRunning,
		TaskEnvironmentID: "env-42",
	}

	msgs := appendSessionStateMessage(session.ID, session, nil)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Action != ws.ActionSessionStateChanged {
		t.Fatalf("expected action %q, got %q", ws.ActionSessionStateChanged, msgs[0].Action)
	}

	payload := decodePayload(t, msgs[0].Payload)
	got, present := payload["task_environment_id"]
	if !present {
		t.Fatalf("payload missing task_environment_id key — frontend env map will not be seeded")
	}
	if got != "env-42" {
		t.Fatalf("expected task_environment_id=env-42, got %v", got)
	}
}

// TestAppendSessionStateMessage_OmitsEmptyTaskEnvironmentID — sessions without
// an environment (legacy rows, archived sessions) must not emit an empty
// task_environment_id field that would clobber a populated frontend map.
func TestAppendSessionStateMessage_OmitsEmptyTaskEnvironmentID(t *testing.T) {
	session := &models.TaskSession{
		ID:     "sess-2",
		TaskID: "task-1",
		State:  models.TaskSessionStateCompleted,
	}

	msgs := appendSessionStateMessage(session.ID, session, nil)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	payload := decodePayload(t, msgs[0].Payload)
	if _, present := payload["task_environment_id"]; present {
		t.Fatalf("payload should not include task_environment_id when session has none")
	}
}
