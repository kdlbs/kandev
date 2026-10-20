package service_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/service"
)

// recordingTokenMinter captures the runtime-token arguments the Office
// credential re-minter passes, so a test can assert the token is scoped to
// the launch session (with the run's persisted session as the fallback).
type recordingTokenMinter struct {
	token                                                        string
	calls                                                        int
	agentID, taskID, workspaceID, runID, sessionID, capabilities string
	err                                                          error
}

func (m *recordingTokenMinter) MintRuntimeJWT(agentID, taskID, workspaceID, runID, sessionID, capabilities string) (string, error) {
	m.calls++
	m.agentID, m.taskID, m.workspaceID, m.runID = agentID, taskID, workspaceID, runID
	m.sessionID, m.capabilities = sessionID, capabilities
	return m.token, m.err
}

// TestRemintLaunchCredentials_ScopesTokenToLaunchSession pins the P45a
// contract: the re-mint carries the session the launch actually uses. A
// launch with no session yet (a ceiling-deferred replay) falls back to the
// run's persisted session id, and an env with no Office run identity
// round-trips unchanged without minting.
func TestRemintLaunchCredentials_ScopesTokenToLaunchSession(t *testing.T) {
	svc := newTestService(t, service.ServiceOptions{APIBaseURL: "http://localhost:8080/api/v1"})
	minter := &recordingTokenMinter{token: "run-token"}
	svc.SetAgentTokenMinter(minter)
	si := service.NewSchedulerIntegration(svc, time.Minute)
	ctx := context.Background()

	svc.ExecSQL(t, `INSERT INTO runs (id, agent_profile_id, reason, payload, status, session_id, capabilities, requested_at)
		VALUES ('run-remint-1', 'agent-1', 'task_assigned', '{}', 'queued', 'session-persisted', '{"can_handoff_tasks":true}', datetime('now'))`)

	officeEnv := map[string]string{
		"KANDEV_RUN_ID":       "run-remint-1",
		"KANDEV_AGENT_ID":     "agent-1",
		"KANDEV_WORKSPACE_ID": "ws-1",
	}

	got, err := si.RemintLaunchCredentials(ctx, "task-1", "session-launch", officeEnv)
	if err != nil {
		t.Fatalf("remint with launch session: %v", err)
	}
	if minter.agentID != "agent-1" || minter.taskID != "task-1" || minter.workspaceID != "ws-1" || minter.runID != "run-remint-1" {
		t.Fatalf("minted identity = %q/%q/%q/%q", minter.agentID, minter.taskID, minter.workspaceID, minter.runID)
	}
	if minter.capabilities != `{"can_handoff_tasks":true}` {
		t.Fatalf("capabilities = %q, want persisted run capabilities", minter.capabilities)
	}
	if minter.sessionID != "session-launch" {
		t.Fatalf("token session id = %q, want %q", minter.sessionID, "session-launch")
	}
	if got["KANDEV_API_KEY"] != "run-token" || got["KANDEV_RUN_TOKEN"] != "run-token" {
		t.Fatalf("refreshed env = %v, want run-token bearer keys", got)
	}
	if officeEnv["KANDEV_API_KEY"] != "" {
		t.Fatal("original env mutated; re-mint must return a copy")
	}

	if _, err := si.RemintLaunchCredentials(ctx, "task-1", "", officeEnv); err != nil {
		t.Fatalf("remint with empty session: %v", err)
	}
	if minter.sessionID != "session-persisted" {
		t.Fatalf("fallback session id = %q, want %q", minter.sessionID, "session-persisted")
	}

	before := minter.calls
	plainEnv := map[string]string{"KANDEV_SOMETHING_ELSE": "x"}
	unchanged, err := si.RemintLaunchCredentials(ctx, "task-1", "session-launch", plainEnv)
	if err != nil {
		t.Fatalf("remint non-office env: %v", err)
	}
	if minter.calls != before {
		t.Fatal("non-Office env triggered a token mint")
	}
	if unchanged["KANDEV_SOMETHING_ELSE"] != "x" {
		t.Fatalf("non-Office env changed: %v", unchanged)
	}
}

func TestRemintLaunchCredentials_FailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name      string
		runID     string
		mintErr   error
		wantErr   error
		wantCalls int
	}{
		{name: "missing run", runID: "missing", wantErr: sql.ErrNoRows},
		{name: "mint rejected", runID: "run-remint-fail", mintErr: context.Canceled, wantErr: context.Canceled, wantCalls: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestService(t)
			svc.ExecSQL(t, `INSERT INTO runs (id, agent_profile_id, reason, payload, status, requested_at)
    VALUES ('run-remint-fail', 'agent-1', 'task_assigned', '{}', 'queued', datetime('now'))`)
			minter := &recordingTokenMinter{token: "must-not-dispatch", err: tc.mintErr}
			svc.SetAgentTokenMinter(minter)
			si := service.NewSchedulerIntegration(svc, time.Minute)
			env := map[string]string{"KANDEV_RUN_ID": tc.runID, "KANDEV_AGENT_ID": "agent-1", "KANDEV_WORKSPACE_ID": "ws-1", "KANDEV_API_KEY": "captured-token"}
			got, err := si.RemintLaunchCredentials(context.Background(), "task-1", "session-launch", env)
			if !errors.Is(err, tc.wantErr) || got != nil {
				t.Fatalf("refresh = %v, %v; want nil env and preserved cause %v", got, err, tc.wantErr)
			}
			if minter.calls != tc.wantCalls {
				t.Fatalf("mint calls = %d, want %d", minter.calls, tc.wantCalls)
			}
			if env["KANDEV_API_KEY"] != "captured-token" {
				t.Fatal("failed refresh mutated input")
			}
		})
	}
}
