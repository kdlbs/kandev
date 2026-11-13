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
	// The canonical Office projection reads workflows even when project_id
	// decides the result. Keep that task-owned schema local to this fixture.
	svc.ExecSQL(t, `CREATE TABLE IF NOT EXISTS workflows (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL DEFAULT '',
		workflow_template_id TEXT NOT NULL DEFAULT '',
		is_system INTEGER NOT NULL DEFAULT 0
	)`)
	officeWorkflowID := ""
	if projectID != "" {
		officeWorkflowID = "wf-parked"
	}
	svc.ExecSQL(t, `INSERT INTO workspaces (id, office_workflow_id) VALUES ('ws-1', ?)`, officeWorkflowID)
	svc.ExecSQL(t, `INSERT INTO workflows (id, workspace_id, workflow_template_id, is_system)
		VALUES ('wf-parked', 'ws-1', 'routine', 0)`)
	svc.ExecSQL(t, `INSERT INTO workflow_steps (id, agent_profile_id, stage_type)
		VALUES ('step-parked', ?, 'custom')`, agent.ID)
	insertTestTask(t, svc, "task-parked", "ws-1")
	svc.ExecSQL(t, `UPDATE tasks SET project_id = ?, workflow_id = 'wf-parked',
		workflow_step_id = 'step-parked', title = 'Routine task' WHERE id = 'task-parked'`, projectID)
	setTestTaskAssignee(t, svc, "task-parked", agent.ID)
	insertTestTaskSession(t, svc, "session-launched", "task-parked", agent.ID)
	svc.ExecSQL(t, `UPDATE task_sessions SET state = 'IDLE' WHERE id = 'session-launched'`)
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

// Office keeps its claim while a parked turn still owns teardown. Successful
// stops and explicit no-stop completion confirmations release the same claim;
// the early park alone must not admit another run onto a live agent process.
func TestParkedOfficeSessionRunWaitsForRuntimeSettlement(t *testing.T) {
	for _, subject := range []string{events.AgentStopped, events.OfficeTurnSettled} {
		t.Run(subject, func(t *testing.T) {
			f := newParkedRunFixtureForTask(t, "proj-office")
			f.publishSessionState(t, "session-launched", "RUNNING", "WAITING_FOR_INPUT", "")

			if got := f.runStatus(t); got != service.RunStatusClaimed {
				t.Fatalf("run status = %q, want claimed while Office teardown is pending", got)
			}
			free, err := f.svc.CheckoutTask(context.Background(), "task-parked", "agent-other")
			if err != nil {
				t.Fatalf("checkout before teardown: %v", err)
			}
			if free {
				t.Fatal("early Office park released its task checkout")
			}

			event := bus.NewEvent(subject, "test", map[string]string{
				"task_id":          "task-parked",
				"session_id":       "session-launched",
				"agent_profile_id": f.agent.ID,
				"run_id":           "run-parked",
				"turn_id":          "completed-turn",
				"turn_started_at":  time.Now().UTC().Add(-30 * time.Second).Format(time.RFC3339Nano),
				"updated_at":       time.Now().UTC().Format(time.RFC3339Nano),
			})
			if err := f.eb.Publish(context.Background(), subject, event); err != nil {
				t.Fatalf("publish runtime settlement: %v", err)
			}
			if got := f.runStatus(t); got != service.RunStatusFinished {
				t.Fatalf("run status = %q, want finished after runtime settlement", got)
			}
			free, err = f.svc.CheckoutTask(context.Background(), "task-parked", "agent-other")
			if err != nil {
				t.Fatalf("checkout after settlement: %v", err)
			}
			if !free {
				t.Fatal("task checkout is still held after runtime settlement")
			}
			if err := f.eb.Publish(context.Background(), subject, event); err != nil {
				t.Fatalf("duplicate runtime settlement: %v", err)
			}
			if got := f.runStatus(t); got != service.RunStatusFinished {
				t.Fatalf("duplicate settlement changed finished outcome: %q", got)
			}
		})
	}
}
