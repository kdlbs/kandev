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

type parkedRunFixture struct {
	svc   *service.Service
	eb    bus.EventBus
	agent *models.AgentInstance
}

func newParkedRunFixture(t *testing.T) parkedRunFixture {
	t.Helper()
	return newParkedRunFixtureForTask(t, "")
}

// newParkedRunFixtureForTask seeds the parked-run task; a non-empty projectID
// makes it an Office task that receives the Office IDLE teardown.
func newParkedRunFixtureForTask(t *testing.T, projectID string) parkedRunFixture {
	t.Helper()
	svc := newTestService(t, service.ServiceOptions{TaskStarter: &mockTaskStarter{}})
	svc.SetSyncHandlers(true)
	eb := bus.NewMemoryEventBus(logger.Default())
	if err := svc.RegisterEventSubscribers(eb); err != nil {
		t.Fatalf("register subscribers: %v", err)
	}
	agent := &models.AgentInstance{
		ID: "agent-parked", WorkspaceID: "ws-1", Name: "parked",
		Role: models.AgentRoleWorker, Status: models.AgentStatusIdle,
	}
	if err := svc.CreateAgentInstance(context.Background(), agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	svc.ExecSQL(t, `INSERT INTO tasks (id, workspace_id, project_id, title, created_at, updated_at)
		VALUES ('task-parked', 'ws-1', ?, 'Routine task', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, projectID)
	claimedAt := time.Now().UTC().Add(-time.Minute)
	svc.ExecSQL(t, `
		INSERT INTO runs (
			id, agent_profile_id, reason, payload, status, coalesced_count,
			context_snapshot, requested_at, claimed_at, session_id
		) VALUES (
			'run-parked', ?, 'task_children_completed', '{"task_id":"task-parked"}',
			'claimed', 1, '{}', ?, ?, 'session-launched'
		)
	`, agent.ID, claimedAt, claimedAt)
	ok, err := svc.CheckoutTask(context.Background(), "task-parked", agent.ID)
	if err != nil || !ok {
		t.Fatalf("seed checkout: ok=%v err=%v", ok, err)
	}
	return parkedRunFixture{svc: svc, eb: eb, agent: agent}
}

func (f parkedRunFixture) publishSessionState(t *testing.T, sessionID, oldState, newState, errorMessage string) {
	t.Helper()
	event := bus.NewEvent(events.TaskSessionStateChanged, "test", map[string]interface{}{
		"task_id":          "task-parked",
		"session_id":       sessionID,
		"old_state":        oldState,
		"new_state":        newState,
		"error_message":    errorMessage,
		"agent_profile_id": f.agent.ID,
	})
	if err := f.eb.Publish(context.Background(), events.TaskSessionStateChanged, event); err != nil {
		t.Fatalf("publish session state: %v", err)
	}
}

func (f parkedRunFixture) runStatus(t *testing.T) runmodels.RunStatus {
	t.Helper()
	runs, err := f.svc.ListRuns(context.Background(), "ws-1")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	for _, run := range runs {
		if run.ID == "run-parked" {
			return run.Status
		}
	}
	t.Fatal("run-parked missing")
	return ""
}

// A run whose launched session ends its turn on the per-launch session model
// (no IDLE teardown, no process exit) parks the session at WAITING_FOR_INPUT.
// That park is the turn's only completion signal, so it must settle the run;
// otherwise the run stays claimed, blocks the agent ceiling, and is requeued
// by stale-claim recovery into a fresh duplicate launch.
func TestParkedLaunchedSessionSettlesClaimedRun(t *testing.T) {
	f := newParkedRunFixture(t)

	f.publishSessionState(t, "session-launched", "RUNNING", "WAITING_FOR_INPUT", "")

	if got := f.runStatus(t); got != service.RunStatusFinished {
		t.Fatalf("run status = %q, want finished after the launched session parked", got)
	}
	free, err := f.svc.CheckoutTask(context.Background(), "task-parked", "agent-other")
	if err != nil {
		t.Fatalf("checkout after settle: %v", err)
	}
	if !free {
		t.Fatal("task checkout is still held after the run settled")
	}
}

func TestParkedSessionSettlementIgnoresFailuresAndForeignSessions(t *testing.T) {
	cases := []struct {
		name, sessionID, oldState, newState, errorMessage string
	}{
		{"failed turn parks with error", "session-launched", "RUNNING", "WAITING_FOR_INPUT", "agent crashed"},
		{"another session of the task", "session-other", "RUNNING", "WAITING_FOR_INPUT", ""},
		{"not a turn end", "session-launched", "STARTING", "WAITING_FOR_INPUT", ""},
		{"office idle park is settled by teardown", "session-launched", "RUNNING", "IDLE", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newParkedRunFixture(t)
			f.publishSessionState(t, tc.sessionID, tc.oldState, tc.newState, tc.errorMessage)
			if got := f.runStatus(t); got != service.RunStatusClaimed {
				t.Fatalf("run status = %q, want claimed (this transition does not complete the run)", got)
			}
		})
	}
}

// An Office turn that never reaches a stopping teardown — a cancelled turn,
// a session without an agent execution, or a session relaunched for the next
// run — would otherwise leave its claimed run stuck and hold the agent
// ceiling. The park is the turn's only completion signal in every case, so it
// settles the run; the Office IDLE teardown's later AgentStopped is then a
// no-op.
func TestParkedOfficeSessionRunSettlesOnPark(t *testing.T) {
	f := newParkedRunFixtureForTask(t, "proj-office")

	f.publishSessionState(t, "session-launched", "RUNNING", "WAITING_FOR_INPUT", "")

	if got := f.runStatus(t); got != service.RunStatusFinished {
		t.Fatalf("run status = %q, want finished after the Office session parked", got)
	}
	free, err := f.svc.CheckoutTask(context.Background(), "task-parked", "agent-other")
	if err != nil {
		t.Fatalf("checkout after settle: %v", err)
	}
	if !free {
		t.Fatal("task checkout is still held after the Office run settled")
	}

	// The Office IDLE teardown still fires AgentStopped afterwards; it must not
	// error or re-settle the already finished run.
	event := bus.NewEvent(events.AgentStopped, "test", map[string]string{
		"task_id":          "task-parked",
		"session_id":       "session-launched",
		"agent_profile_id": f.agent.ID,
		"run_id":           "run-parked",
	})
	if err := f.eb.Publish(context.Background(), events.AgentStopped, event); err != nil {
		t.Fatalf("publish agent stopped: %v", err)
	}
	if got := f.runStatus(t); got != service.RunStatusFinished {
		t.Fatalf("run status = %q, want finished after the late AgentStopped", got)
	}
}
