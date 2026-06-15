package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/common/config"
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

func TestBootInitialStateIncludesFeatureFlags(t *testing.T) {
	state := bootInitialState(routeParams{
		features: config.FeaturesConfig{Office: true},
	})

	raw, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("Marshal state: %v", err)
	}
	var decoded struct {
		Features config.FeaturesConfig `json:"features"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal state: %v", err)
	}
	if !decoded.Features.Office {
		t.Fatal("features.office should hydrate true from the backend boot payload")
	}
}

func TestAppendSessionStateMessage_IncludesUpdatedAt(t *testing.T) {
	updatedAt := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
	session := &models.TaskSession{
		ID:        "sess-3",
		TaskID:    "task-1",
		State:     models.TaskSessionStateWaitingForInput,
		UpdatedAt: updatedAt,
	}

	msgs := appendSessionStateMessage(session.ID, session, nil)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	payload := decodePayload(t, msgs[0].Payload)
	got, present := payload["updated_at"]
	if !present {
		t.Fatal("payload missing updated_at — stale subscribe snapshots cannot be ignored")
	}
	if got != updatedAt.Format(time.RFC3339Nano) {
		t.Fatalf("expected updated_at=%q, got %v", updatedAt.Format(time.RFC3339Nano), got)
	}
}

func TestAppendContextWindowMessage_DoesNotEmitStateSnapshot(t *testing.T) {
	session := &models.TaskSession{
		ID:        "sess-4",
		TaskID:    "task-1",
		State:     models.TaskSessionStateRunning,
		UpdatedAt: time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC),
		Metadata: map[string]interface{}{
			"context_window": map[string]interface{}{"size": 100},
		},
	}

	msgs := appendContextWindowMessage(session.ID, session, nil)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	payload := decodePayload(t, msgs[0].Payload)
	if _, present := payload["new_state"]; present {
		t.Fatal("context-window snapshot must not carry new_state and overwrite fresher session state")
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

func TestExternalMCPOpenMiddleware_AllowsLoopbackAndRemote(t *testing.T) {
	r := setupExternalMCPAccessRouter()

	for _, tc := range []struct{ name, remoteAddr string }{
		{"loopback", "127.0.0.1:4321"},
		{"remote", "203.0.113.10:4321"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/mcp", nil)
			req.RemoteAddr = tc.remoteAddr
			r.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("request = %d, want %d", w.Code, http.StatusOK)
			}
		})
	}
}

func setupExternalMCPAccessRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(externalMCPOpenMiddleware())
	r.GET("/mcp", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	return r
}
