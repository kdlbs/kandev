package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	commonlogger "github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// agent.stdin and agent.resize resolve the execution with a bare in-memory
// lookup, which skips the GetOrEnsure* chokepoint where the lifecycle manager
// runs the per-user check. Unguarded, a caller holding any live session ID
// could write arbitrary bytes into another user's agent process — effectively
// running commands in their worktree — or resize its PTY.

func passthroughTestLogger(t *testing.T) *commonlogger.Logger {
	t.Helper()
	log, err := commonlogger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	return log
}

// TestPassthroughActionsDenyForeignSession asserts both actions refuse before
// touching the execution. The manager carries only the access checker, so the
// guard must fire before any other lifecycle state is consulted.
func TestPassthroughActionsDenyForeignSession(t *testing.T) {
	mgr := &lifecycle.Manager{}
	mgr.SetSessionAccessChecker(func(_ context.Context, sessionID string) error {
		if sessionID == "sess-a" {
			return nil
		}
		return errors.New("task not found")
	})
	h := &PassthroughHandlers{lifecycleMgr: mgr, logger: passthroughTestLogger(t)}

	cases := map[string]struct {
		action  string
		handler func(context.Context, *ws.Message) (*ws.Message, error)
		payload string
	}{
		"stdin": {ws.ActionAgentStdin, h.wsAgentStdin, `{"session_id":"sess-b","data":"rm -rf /\n"}`},
		"resize": {ws.ActionAgentResize, h.wsAgentResize,
			`{"session_id":"sess-b","cols":80,"rows":24}`},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			msg := &ws.Message{ID: "1", Type: ws.MessageTypeRequest, Action: tc.action,
				Payload: json.RawMessage(tc.payload)}

			resp, err := tc.handler(context.Background(), msg)

			if err == nil {
				t.Fatalf("foreign session was accepted (resp=%+v)", resp)
			}
			// The refusal must not distinguish "not yours" from "no such session".
			if err.Error() != "session not found" {
				t.Errorf("error = %q, want \"session not found\"", err.Error())
			}
		})
	}
}

// TestPassthroughGuardIsNoOpWhenUnwired keeps single-user instances working.
// This asserts the guard's own predicate rather than driving the handler: with
// no checker installed the handler falls through to GetExecutionBySessionID,
// which a bare Manager cannot serve, so the property is only observable here.
func TestPassthroughGuardIsNoOpWhenUnwired(t *testing.T) {
	if err := (&lifecycle.Manager{}).CheckSessionAccess(context.Background(), "sess-b"); err != nil {
		t.Errorf("unwired checker denied the call (%v); pre-auth behavior broken", err)
	}
}

// TestUserShellStopDeniesForeignEnvironment covers the legacy fallback in
// wsUserShellStop: task_id is optional there and the fallback tears down the
// PTY through the interactive runner directly, never reaching
// GetOrEnsureExecutionForEnvironment where the environment check normally runs.
// With task_id empty and a foreign task_environment_id, it stopped another
// user's terminal.
func TestUserShellStopDeniesForeignEnvironment(t *testing.T) {
	mgr := &lifecycle.Manager{}
	mgr.SetEnvironmentAccessChecker(func(_ context.Context, envID string) error {
		if envID == "env-a" {
			return nil
		}
		return errors.New("task not found")
	})
	h := &ShellHandlers{lifecycleMgr: mgr, logger: passthroughTestLogger(t)}
	msg := &ws.Message{ID: "1", Type: ws.MessageTypeRequest, Action: ws.ActionUserShellStop,
		Payload: json.RawMessage(`{"task_id":"","task_environment_id":"env-b","terminal_id":"shell-1"}`)}

	_, err := h.wsUserShellStop(context.Background(), msg)

	if err == nil {
		t.Fatal("foreign task environment was accepted")
	}
	if err.Error() != "task environment not found" {
		t.Errorf("error = %q, want \"task environment not found\"", err.Error())
	}
}

// TestUserShellStopGuardIsNoOpWhenUnwired keeps single-user instances working.
func TestUserShellStopGuardIsNoOpWhenUnwired(t *testing.T) {
	if err := (&lifecycle.Manager{}).CheckEnvironmentAccess(context.Background(), "env-b"); err != nil {
		t.Errorf("unwired checker denied the call (%v); pre-auth behavior broken", err)
	}
}
