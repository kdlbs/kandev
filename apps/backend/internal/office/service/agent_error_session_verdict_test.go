package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/service"
	runmodels "github.com/kandev/kandev/internal/runs/models"
)

type verdictFixture struct {
	svc *service.Service
	eb  bus.EventBus
}

const (
	verdictAgentID   = "agent-verdict"
	verdictTaskID    = "task-verdict"
	verdictSessionID = "session-verdict"
	verdictRunID     = "run-verdict"
)

func newVerdictFixture(t *testing.T, guard bool, sessionState string) verdictFixture {
	t.Helper()
	svc := newTestService(t, service.ServiceOptions{TaskStarter: &mockTaskStarter{}})
	svc.SetSyncHandlers(true)
	svc.SetAgentErrorSessionVerdict(guard)
	eb := bus.NewMemoryEventBus(logger.Default())
	if err := svc.RegisterEventSubscribers(eb); err != nil {
		t.Fatalf("register subscribers: %v", err)
	}
	agent := &models.AgentInstance{
		ID: verdictAgentID, WorkspaceID: "ws-1", Name: "researcher",
		Role: models.AgentRoleWorker, Status: models.AgentStatusIdle,
	}
	if err := svc.CreateAgentInstance(context.Background(), agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	svc.ExecSQL(t, `INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES (?, 'ws-1', 'Research', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, verdictTaskID)
	svc.ExecSQL(t, `INSERT INTO task_sessions (id, task_id, agent_profile_id, state, started_at, updated_at)
		VALUES (?, ?, ?, ?, datetime('now'), datetime('now'))`,
		verdictSessionID, verdictTaskID, verdictAgentID, sessionState)
	claimedAt := time.Now().UTC().Add(-time.Minute)
	svc.ExecSQL(t, `
		INSERT INTO runs (
			id, agent_profile_id, reason, payload, status, coalesced_count,
			context_snapshot, requested_at, claimed_at, session_id
		) VALUES (?, ?, 'task_assigned', '{"task_id":"task-verdict"}', 'claimed', 1, '{}', ?, ?, ?)
	`, verdictRunID, verdictAgentID, claimedAt, claimedAt, verdictSessionID)
	return verdictFixture{svc: svc, eb: eb}
}

func (f verdictFixture) publish(t *testing.T, subject string, data map[string]interface{}) {
	t.Helper()
	if err := f.eb.Publish(context.Background(), subject, bus.NewEvent(subject, "test", data)); err != nil {
		t.Fatalf("publish %s: %v", subject, err)
	}
}

func (f verdictFixture) failExecution(t *testing.T) {
	t.Helper()
	f.publish(t, events.AgentFailed, map[string]interface{}{
		"task_id":          verdictTaskID,
		"session_id":       verdictSessionID,
		"agent_profile_id": verdictAgentID,
		"error_message":    "AI_APICallError: Go usage limit exceeded",
	})
}

func (f verdictFixture) sessionState(t *testing.T, oldState, newState, errorMessage string) {
	t.Helper()
	f.publish(t, events.TaskSessionStateChanged, map[string]interface{}{
		"task_id":          verdictTaskID,
		"session_id":       verdictSessionID,
		"old_state":        oldState,
		"new_state":        newState,
		"error_message":    errorMessage,
		"agent_profile_id": verdictAgentID,
	})
}

func (f verdictFixture) runStatus(t *testing.T) runmodels.RunStatus {
	t.Helper()
	run, err := f.svc.GetRun(context.Background(), verdictRunID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	return run.Status
}

// Shared dynamic routing can answer a provider failure by launching a
// successor in the same session. Office must not declare the run failed (and
// queue an on_agent_error replacement) before the session's verdict exists.
func TestAgentErrorSessionVerdict_SuccessorKeepsRunClaimed(t *testing.T) {
	f := newVerdictFixture(t, true, "RUNNING")

	f.failExecution(t)
	if got := f.runStatus(t); got != service.RunStatusClaimed {
		t.Fatalf("run status after AgentFailed = %q, want claimed while the session verdict is pending", got)
	}

	f.sessionState(t, "RUNNING", "STARTING", "")
	if got := f.runStatus(t); got != service.RunStatusClaimed {
		t.Fatalf("run status after successor launch = %q, want claimed", got)
	}

	f.sessionState(t, "STARTING", "RUNNING", "")
	f.sessionState(t, "RUNNING", "WAITING_FOR_INPUT", "")
	if got := f.runStatus(t); got != service.RunStatusFinished {
		t.Fatalf("run status after successor turn = %q, want finished by the successor", got)
	}
}

func TestAgentErrorSessionVerdict_FailureVerdictFailsRun(t *testing.T) {
	f := newVerdictFixture(t, true, "RUNNING")

	f.failExecution(t)
	f.sessionState(t, "RUNNING", "FAILED", "AI_APICallError: Go usage limit exceeded")

	if got := f.runStatus(t); got != service.RunStatusFailed {
		t.Fatalf("run status = %q, want failed once the session recorded the failure", got)
	}
}

func TestAgentErrorSessionVerdict_RecordedVerdictFailsRunAtOnce(t *testing.T) {
	f := newVerdictFixture(t, true, "FAILED")

	f.failExecution(t)

	if got := f.runStatus(t); got != service.RunStatusFailed {
		t.Fatalf("run status = %q, want failed when the session verdict already exists", got)
	}
}

func TestAgentErrorSessionVerdict_DisabledKeepsImmediateFailure(t *testing.T) {
	f := newVerdictFixture(t, false, "RUNNING")

	f.failExecution(t)

	if got := f.runStatus(t); got != service.RunStatusFailed {
		t.Fatalf("run status = %q, want failed immediately with the guard disabled", got)
	}
}

// A failure verdict written without a state event (or one missed while the
// failure was parked) is still applied by the bounded re-check.
func TestAgentErrorSessionVerdict_RecheckAppliesMissedFailureVerdict(t *testing.T) {
	f := newVerdictFixture(t, true, "RUNNING")
	f.svc.SetAgentErrorVerdictTimeout(20 * time.Millisecond)

	f.failExecution(t)
	f.svc.ExecSQL(t, `UPDATE task_sessions SET state = 'FAILED', error_message = 'quota' WHERE id = ?`, verdictSessionID)

	// The test database is a single in-memory connection, so the re-check
	// must finish before the test reads the run again.
	time.Sleep(500 * time.Millisecond)
	if got := f.runStatus(t); got != service.RunStatusFailed {
		t.Fatalf("run status = %q, want failed after the verdict re-check", got)
	}
}

// A re-check that cannot read the session is not a verdict. The parked failure
// must stay with its run and be re-checked, not applied to a session that may
// already belong to a successor.
func TestAgentErrorSessionVerdict_RecheckReadErrorKeepsPendingFailure(t *testing.T) {
	f := newVerdictFixture(t, true, "RUNNING")
	f.svc.SetAgentErrorVerdictTimeout(300 * time.Millisecond)

	f.failExecution(t)
	f.svc.ExecSQL(t, `ALTER TABLE task_sessions RENAME TO task_sessions_unreadable`)

	time.Sleep(800 * time.Millisecond)
	if got := f.runStatus(t); got != service.RunStatusClaimed {
		t.Fatalf("run status after re-check read errors = %q, want claimed", got)
	}

	f.svc.ExecSQL(t, `UPDATE task_sessions_unreadable SET state = 'FAILED', error_message = 'quota' WHERE id = ?`, verdictSessionID)
	f.svc.ExecSQL(t, `ALTER TABLE task_sessions_unreadable RENAME TO task_sessions`)

	time.Sleep(800 * time.Millisecond)
	if got := f.runStatus(t); got != service.RunStatusFailed {
		t.Fatalf("run status after the recorded verdict = %q, want failed", got)
	}
}
