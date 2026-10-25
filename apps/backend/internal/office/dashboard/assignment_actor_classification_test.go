package dashboard_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/scheduler"
	officeservice "github.com/kandev/kandev/internal/office/service"
	runsservice "github.com/kandev/kandev/internal/runs/service"
)

// wireRealReactivity replaces the dashboard-side reactivity stub with the
// production adapter around a real *scheduler.SchedulerService backed by
// the same repository, so a dashboard-level mutation call reaches an
// actually-persisted runs row rather than a captured TaskReactivityChange.
func wireRealReactivity(t *testing.T, deps *testDeps) {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	svc := officeservice.NewService(officeservice.ServiceOptions{Repo: deps.repo, Logger: log})
	ss := scheduler.NewSchedulerService(deps.repo, log, svc)
	ss.SetRunsService(runsservice.New(deps.repo.RunsRepository(), nil, log, nil))
	deps.svc.SetReactivityApplier(scheduler.NewDashboardReactivityAdapter(ss))
}

// createIdleAgent registers an idle agent instance so the reactivity
// pipeline's guardAgentStatus check admits the wake instead of skipping it.
func createIdleAgent(t *testing.T, deps *testDeps, id, wsID string) {
	t.Helper()
	if err := deps.repo.CreateAgentInstance(context.Background(), &models.AgentInstance{
		ID: id, WorkspaceID: wsID, Name: id, Status: models.AgentStatusIdle,
	}); err != nil {
		t.Fatalf("create agent %s: %v", id, err)
	}
}

// latestTaskAssignedPayload reads back the most recently persisted
// task_assigned run row's JSON payload for the given assignee, decoded.
func latestTaskAssignedPayload(t *testing.T, deps *testDeps, assigneeID string) map[string]any {
	t.Helper()
	var payload string
	err := deps.db.Get(&payload, `
		SELECT payload FROM runs
		WHERE reason = 'task_assigned' AND agent_profile_id = ?
		ORDER BY requested_at DESC LIMIT 1
	`, assigneeID)
	if err != nil {
		t.Fatalf("query persisted run payload for %s: %v", assigneeID, err)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(payload), &decoded); err != nil {
		t.Fatalf("decode payload %s: %v", payload, err)
	}
	return decoded
}

// TestSetTaskAssigneeAsAgent_PersistedRunPayloadCarriesActorType exercises
// runReactivityForAssigneeChange's actor-classification line
// (service_tasks.go's callerAgentID != "" branch) through a real
// SetTaskAssigneeAsAgent call, all the way to the persisted runs row.
// Existing coverage either hand-built the scheduler-side payload directly
// (assignment_rate_limit_inscope_producer_test.go) or stubbed the
// reactivity applier before any run was queued (session_termination_test.go)
// — neither proves this line's output ever reaches a stored payload.
func TestSetTaskAssigneeAsAgent_PersistedRunPayloadCarriesActorType(t *testing.T) {
	deps := newTestDeps(t)
	wireRealReactivity(t, deps)
	deps.agents.names = map[string]string{"agent-actor": "agent-actor"}
	deps.agents.roles = map[string]string{"agent-actor": string(models.AgentRoleWorker)}

	insertTestTask(t, deps.db, "task-actor-classify", "ws-actor", "Actor classify", "todo", 2)
	createIdleAgent(t, deps, "agent-assignee-by-agent", "ws-actor")

	if err := deps.svc.SetTaskAssigneeAsAgent(
		context.Background(), "agent-actor", "task-actor-classify", "agent-assignee-by-agent",
	); err != nil {
		t.Fatalf("set assignee (agent caller): %v", err)
	}

	got := latestTaskAssignedPayload(t, deps, "agent-assignee-by-agent")
	if got["actor_type"] != "agent" {
		t.Fatalf("persisted run payload actor_type = %v, want %q: %#v", got["actor_type"], "agent", got)
	}
}

// TestSetTaskAssigneeAsAgent_EmptyCallerNeverPersistsAgentActor is the
// negative counterpart: an internal/admin caller (empty callerAgentID) must
// never persist actor_type "agent" in the run row the new assignee wakes on.
func TestSetTaskAssigneeAsAgent_EmptyCallerNeverPersistsAgentActor(t *testing.T) {
	deps := newTestDeps(t)
	wireRealReactivity(t, deps)

	insertTestTask(t, deps.db, "task-actor-classify-2", "ws-actor-2", "Actor classify 2", "todo", 2)
	createIdleAgent(t, deps, "agent-assignee-by-user", "ws-actor-2")

	if err := deps.svc.SetTaskAssigneeAsAgent(
		context.Background(), "", "task-actor-classify-2", "agent-assignee-by-user",
	); err != nil {
		t.Fatalf("set assignee (empty caller): %v", err)
	}

	got := latestTaskAssignedPayload(t, deps, "agent-assignee-by-user")
	if got["actor_type"] == "agent" {
		t.Fatalf("persisted run payload actor_type = %v, want anything but %q for an empty callerAgentID: %#v",
			got["actor_type"], "agent", got)
	}
}
