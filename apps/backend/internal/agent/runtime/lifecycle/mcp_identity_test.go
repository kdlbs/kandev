package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// recordingMCPHandler captures the context its dispatch ran under so tests can
// assert which identity (if any) reached the tool handlers.
type recordingMCPHandler struct {
	gotCtx context.Context
	calls  int
}

func (h *recordingMCPHandler) Dispatch(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	h.gotCtx = ctx
	h.calls++
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{"ok": true})
}

func newMCPStreamManager(t *testing.T, inner *recordingMCPHandler, scoper MCPIdentityScoper) *StreamManager {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	sm := NewStreamManager(log, StreamCallbacks{}, inner, nil)
	sm.mcpIdentityScoper = scoper
	return sm
}

func mcpRequest(t *testing.T, payload map[string]interface{}) *ws.Message {
	t.Helper()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return &ws.Message{ID: "req-1", Type: ws.MessageTypeRequest, Action: "mcp.list_tasks", Payload: data}
}

// TestMCPHandlerForScopesToExecutionTask is the core wiring assertion: the
// identity handed to the tool handlers comes from the execution that owns the
// stream.
func TestMCPHandlerForScopesToExecutionTask(t *testing.T) {
	inner := &recordingMCPHandler{}
	var scopedTaskIDs []string
	sm := newMCPStreamManager(t, inner, func(ctx context.Context, taskID string) (context.Context, error) {
		scopedTaskIDs = append(scopedTaskIDs, taskID)
		return authn.WithIdentity(ctx, authn.Identity{UserID: "owner-of-" + taskID, Role: authn.RoleMember}), nil
	})

	handler := sm.mcpHandlerFor(&AgentExecution{ID: "exec-1", TaskID: "task-a"})
	resp, err := handler.Dispatch(context.Background(), mcpRequest(t, map[string]interface{}{}))
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if resp.Type != ws.MessageTypeResponse {
		t.Fatalf("response type = %q, want response", resp.Type)
	}

	if len(scopedTaskIDs) != 1 || scopedTaskIDs[0] != "task-a" {
		t.Fatalf("scoped task IDs = %v, want [task-a]", scopedTaskIDs)
	}
	identity, ok := authn.IdentityFromContext(inner.gotCtx)
	if !ok {
		t.Fatal("tool handlers received no identity")
	}
	if identity.UserID != "owner-of-task-a" {
		t.Errorf("UserID = %q, want owner-of-task-a", identity.UserID)
	}
}

// TestMCPHandlerForIgnoresPayloadSessionID is the privilege-escalation pin. The
// payload is agent-controlled, so scoping must key off the execution's task
// even when the request names a different session or task.
func TestMCPHandlerForIgnoresPayloadSessionID(t *testing.T) {
	inner := &recordingMCPHandler{}
	var scopedTaskIDs []string
	sm := newMCPStreamManager(t, inner, func(ctx context.Context, taskID string) (context.Context, error) {
		scopedTaskIDs = append(scopedTaskIDs, taskID)
		return ctx, nil
	})

	handler := sm.mcpHandlerFor(&AgentExecution{ID: "exec-1", TaskID: "task-a"})
	_, err := handler.Dispatch(context.Background(), mcpRequest(t, map[string]interface{}{
		"session_id": "session-of-victim",
		"task_id":    "task-victim",
	}))
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	if len(scopedTaskIDs) != 1 || scopedTaskIDs[0] != "task-a" {
		t.Errorf("scoped task IDs = %v, want [task-a] — payload IDs must not steer scoping", scopedTaskIDs)
	}
}

// TestMCPHandlerForDeniesWhenScopingFails pins fail-closed behavior: an
// unresolvable owner must not fall through to the unscoped handlers.
func TestMCPHandlerForDeniesWhenScopingFails(t *testing.T) {
	inner := &recordingMCPHandler{}
	sm := newMCPStreamManager(t, inner, func(context.Context, string) (context.Context, error) {
		return nil, errors.New("db unavailable")
	})

	handler := sm.mcpHandlerFor(&AgentExecution{ID: "exec-1", TaskID: "task-a"})
	resp, err := handler.Dispatch(context.Background(), mcpRequest(t, map[string]interface{}{}))
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	if resp.Type != ws.MessageTypeError {
		t.Errorf("response type = %q, want error", resp.Type)
	}
	if inner.calls != 0 {
		t.Errorf("inner handler ran %d times, want 0 — the request must be denied", inner.calls)
	}
}

// TestMCPHandlerForPassesThroughWithoutScoper keeps single-user instances and
// isolated tests on the original unwrapped handler.
func TestMCPHandlerForPassesThroughWithoutScoper(t *testing.T) {
	inner := &recordingMCPHandler{}
	sm := newMCPStreamManager(t, inner, nil)

	if got := sm.mcpHandlerFor(&AgentExecution{ID: "exec-1", TaskID: "task-a"}); got != inner {
		t.Errorf("handler = %T, want the unwrapped inner handler", got)
	}
}

// TestMCPHandlerForPassesThroughWithoutTaskID covers executions with no task
// (there is no owner to resolve, so wrapping would deny every call).
func TestMCPHandlerForPassesThroughWithoutTaskID(t *testing.T) {
	inner := &recordingMCPHandler{}
	sm := newMCPStreamManager(t, inner, func(ctx context.Context, _ string) (context.Context, error) {
		return ctx, nil
	})

	if got := sm.mcpHandlerFor(&AgentExecution{ID: "exec-1"}); got != inner {
		t.Errorf("handler = %T, want the unwrapped inner handler", got)
	}
}

func TestSetMCPIdentityScoperReachesStreamManager(t *testing.T) {
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	m := &Manager{streamManager: NewStreamManager(log, StreamCallbacks{}, &recordingMCPHandler{}, nil)}

	m.SetMCPIdentityScoper(func(ctx context.Context, _ string) (context.Context, error) { return ctx, nil })

	if m.streamManager.mcpIdentityScoper == nil {
		t.Error("SetMCPIdentityScoper did not reach the stream manager")
	}
}
