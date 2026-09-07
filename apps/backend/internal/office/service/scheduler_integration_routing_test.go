package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/service"
	"github.com/kandev/kandev/internal/workflow/engine"
)

// captureDispatcher implements service.RoutingDispatcher and records the
// LaunchContext it received. Used to assert that the prompt/env built by
// the office scheduler integration is forwarded intact to routing —
// regression coverage for the bug where routed launches passed an empty
// prompt and the task starter fell back to task.Description.
type captureDispatcher struct {
	mu     sync.Mutex
	calls  []service.LaunchContext
	runs   []*models.Run
	agents []*models.AgentInstance
	// launched, parked, and err control the (launched, parked, err) tuple
	// the fake returns. Default zero-value is (false, false, nil) — routing
	// fall-through — so callers don't need to set anything for the
	// assertion path tested here.
	launched bool
	parked   bool
	err      error
}

func (c *captureDispatcher) DispatchWithRouting(
	_ context.Context, run *models.Run,
	agent *models.AgentInstance, launch service.LaunchContext,
) (bool, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls = append(c.calls, launch)
	c.runs = append(c.runs, run)
	c.agents = append(c.agents, agent)
	return c.launched, c.parked, c.err
}

func (c *captureDispatcher) HandlePostStartFailure(
	_ context.Context, _ *models.Run, _ *models.AgentInstance, _ string,
	_ *streams.ProviderError,
) (bool, error) {
	return false, nil
}

func (c *captureDispatcher) MarkRunSuccessHealth(
	_ context.Context, _ *models.Run, _ *models.AgentInstance,
) {
}

func (c *captureDispatcher) lastCall() service.LaunchContext {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.calls) == 0 {
		return service.LaunchContext{}
	}
	return c.calls[len(c.calls)-1]
}

func (c *captureDispatcher) callCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.calls)
}

// TestSchedulerIntegration_RoutingReceivesBuiltPromptAndEnv is the
// regression test for the bug where StartTaskWithRoute was called with
// an empty prompt and the routed launch silently fell back to
// task.Description. Asserts that the office scheduler integration
// forwards the office-built prompt + env to the routing dispatcher
// instead of dropping them.
//
// Mechanism: run a full SchedulerTick with a fake RoutingDispatcher
// that captures LaunchContext. Assert the captured LaunchContext.Prompt
// contains the office prompt body (the task title is rendered into
// it by the prompt builder) and that env was forwarded.
func TestSchedulerIntegration_RoutingReceivesBuiltPromptAndEnv(t *testing.T) {
	mock := &mockTaskStarter{}
	svc := newTestService(t, service.ServiceOptions{
		TaskStarter: mock,
		APIBaseURL:  "http://localhost:8080/api/v1",
	})
	svc.SetAgentTokenMinter(fakeAgentTokenMinter{token: "test-token"})
	dispatcher := &captureDispatcher{}
	svc.SetRoutingDispatcher(dispatcher)
	ctx := context.Background()

	agent := &models.AgentInstance{
		ID:                 "routing-agent-1",
		WorkspaceID:        "ws-1",
		Name:               "routing-worker",
		Role:               models.AgentRoleWorker,
		Status:             models.AgentStatusIdle,
		ExecutorPreference: `{"type":"worktree"}`,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	svc.ExecSQL(t, `INSERT INTO tasks (id, workspace_id, title, description, priority, created_at, updated_at)
		VALUES ('task-routing-1', 'ws-1', 'ROUTING_PROMPT_SENTINEL_TITLE',
		        'Implement endpoint', 'medium', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)
	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned,
		`{"task_id":"task-routing-1"}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	service.RunSchedulerTick(svc, ctx)

	if dispatcher.callCount() != 1 {
		t.Fatalf("expected exactly 1 DispatchWithRouting call; got %d", dispatcher.callCount())
	}
	got := dispatcher.lastCall()
	if got.Prompt == "" {
		t.Fatal("LaunchContext.Prompt empty — the office prompt is being dropped before reaching the routing dispatcher")
	}
	if !containsIgnoreCase(got.Prompt, "ROUTING_PROMPT_SENTINEL_TITLE") {
		t.Errorf("LaunchContext.Prompt missing office-built body; got: %s", got.Prompt)
	}
	if got.Env == nil {
		t.Fatal("LaunchContext.Env nil — env built by office scheduler was not forwarded")
	}
	if got.Env["KANDEV_API_KEY"] != "test-token" {
		t.Errorf("LaunchContext.Env[KANDEV_API_KEY] = %q, want test-token", got.Env["KANDEV_API_KEY"])
	}
	if got.Env["KANDEV_RUN_ID"] == "" {
		t.Error("LaunchContext.Env missing KANDEV_RUN_ID — run identity dropped")
	}
}

func TestSchedulerIntegration_SeatActionFlowsToPromptAndLaunch(t *testing.T) {
	mock := &mockTaskStarter{}
	svc := newTestService(t, service.ServiceOptions{TaskStarter: mock})
	dispatcher := &captureDispatcher{}
	svc.SetRoutingDispatcher(dispatcher)
	svc.SetWorkflowEngineDispatcher(&seatSpyDispatcher{})
	ctx := context.Background()
	if err := svc.CreateSkill(ctx, &models.Skill{
		ID:          "decision-skill",
		WorkspaceID: "ws-1",
		Name:        "Step decision",
		Slug:        "kandev-step-decision",
		Content:     "decision skill",
		Version:     "0.42.0",
		ContentHash: "decision-skill-hash",
	}); err != nil {
		t.Fatalf("create decision skill: %v", err)
	}

	agent := &models.AgentInstance{
		ID:                 "decision-agent-1",
		WorkspaceID:        "ws-1",
		Name:               "decision-reviewer",
		Role:               models.AgentRoleWorker,
		Status:             models.AgentStatusIdle,
		ExecutorPreference: `{"type":"worktree"}`,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	svc.ExecSQL(t, `INSERT INTO workflow_steps (id, stage_type) VALUES (?, ?)`, "step-decision", "review")
	svc.ExecSQL(t, `INSERT INTO tasks (id, workspace_id, workflow_step_id, title, description, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		"task-decision", "ws-1", "step-decision", "Decision task", "Review the change")
	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned,
		`{"task_id":"task-decision"}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	service.RunSchedulerTick(svc, ctx)

	if dispatcher.callCount() != 1 {
		t.Fatalf("expected one routing dispatch, got %d", dispatcher.callCount())
	}
	prompt := dispatcher.lastCall().Prompt
	allowedIdx := strings.Index(prompt, "- Allowed actions:")
	if allowedIdx == -1 || !containsIgnoreCase(prompt[allowedIdx:], "record_step_decision") {
		t.Fatalf("prompt must advertise the seat-derived action: %s", dispatcher.lastCall().Prompt)
	}
	launch := dispatcher.lastCall()
	if len(launch.AdditionalSkillSlugs) != 1 || launch.AdditionalSkillSlugs[0] != "kandev-step-decision" {
		t.Fatalf("launch skill additions = %v, want decision skill", launch.AdditionalSkillSlugs)
	}

	runs, err := svc.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	var run *models.Run
	for _, candidate := range runs {
		if candidate.AgentProfileID == agent.ID && candidate.Reason == service.RunReasonTaskAssigned {
			run = candidate
			break
		}
	}
	if run == nil {
		t.Fatal("missing decision run")
	}
	var capabilities map[string]any
	if err := json.Unmarshal([]byte(run.Capabilities), &capabilities); err != nil {
		t.Fatalf("decode persisted capabilities: %v", err)
	}
	if _, ok := capabilities["record_step_decision"]; ok {
		t.Fatalf("seat-derived action must not be persisted as a runtime capability: %s", run.Capabilities)
	}
	var snapshot map[string]any
	if err := json.Unmarshal([]byte(run.InputSnapshot), &snapshot); err != nil {
		t.Fatalf("decode persisted input snapshot: %v", err)
	}
	if actions, ok := snapshot["available_actions"].([]any); !ok || len(actions) != 1 || actions[0] != "record_step_decision" {
		t.Fatalf("persisted snapshot missing advisory action: %#v", snapshot["available_actions"])
	}

	snapshots, err := svc.ListRunSkillSnapshotsForTest(ctx, run.ID)
	if err != nil {
		t.Fatalf("list run skill snapshots: %v", err)
	}
	if len(snapshots) != 1 || snapshots[0].SkillID != "decision-skill" {
		t.Fatalf("decision seat run snapshots = %#v, want decision skill", snapshots)
	}
}

func TestSchedulerIntegration_NonSeatRunDoesNotReceiveDecisionSkill(t *testing.T) {
	mock := &mockTaskStarter{}
	svc := newTestService(t, service.ServiceOptions{TaskStarter: mock})
	dispatcher := &captureDispatcher{}
	svc.SetRoutingDispatcher(dispatcher)
	svc.SetWorkflowEngineDispatcher(&seatSpyDispatcher{err: engine.ErrParticipantNotFound})
	ctx := context.Background()

	if err := svc.CreateSkill(ctx, &models.Skill{
		ID:          "decision-skill",
		WorkspaceID: "ws-1",
		Name:        "Step decision",
		Slug:        "kandev-step-decision",
		Content:     "decision skill",
		Version:     "0.42.0",
		ContentHash: "decision-skill-hash",
	}); err != nil {
		t.Fatalf("create decision skill: %v", err)
	}
	agent := &models.AgentInstance{
		ID:                 "non-seat-decision-agent",
		WorkspaceID:        "ws-1",
		Name:               "non-seat-reviewer",
		Role:               models.AgentRoleWorker,
		Status:             models.AgentStatusIdle,
		ExecutorPreference: `{"type":"worktree"}`,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	svc.ExecSQL(t, `INSERT INTO workflow_steps (id, stage_type) VALUES (?, ?)`, "step-non-seat", "review")
	svc.ExecSQL(t, `INSERT INTO tasks (id, workspace_id, workflow_step_id, title, description, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		"task-non-seat", "ws-1", "step-non-seat", "Non-seat task", "Review the change")
	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned,
		`{"task_id":"task-non-seat"}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	service.RunSchedulerTick(svc, ctx)

	runs, err := svc.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	var run *models.Run
	for _, candidate := range runs {
		if candidate.AgentProfileID == agent.ID && candidate.Reason == service.RunReasonTaskAssigned {
			run = candidate
			break
		}
	}
	if run == nil {
		t.Fatal("missing non-seat decision run")
	}
	snapshots, err := svc.ListRunSkillSnapshotsForTest(ctx, run.ID)
	if err != nil {
		t.Fatalf("list run skill snapshots: %v", err)
	}
	if len(snapshots) != 0 {
		t.Fatalf("non-seat run snapshots = %#v, want no decision skill", snapshots)
	}
}

// TestSchedulerIntegration_RoutingFallThrough_FallsBackToLegacy asserts
// the routing seam preserves the legacy fall-through behavior: when the
// dispatcher returns (launched=false, parked=false, err=nil), the
// scheduler still hits the legacy TaskStarter with the same prompt.
func TestSchedulerIntegration_RoutingFallThrough_FallsBackToLegacy(t *testing.T) {
	mock := &mockTaskStarter{}
	svc := newTestService(t, service.ServiceOptions{TaskStarter: mock})
	dispatcher := &captureDispatcher{launched: false}
	svc.SetRoutingDispatcher(dispatcher)
	ctx := context.Background()

	agent := &models.AgentInstance{
		ID:                 "routing-fallthrough-1",
		WorkspaceID:        "ws-1",
		Name:               "ft-worker",
		Role:               models.AgentRoleWorker,
		Status:             models.AgentStatusIdle,
		ExecutorPreference: `{"type":"worktree"}`,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	svc.ExecSQL(t, `INSERT INTO tasks (id, workspace_id, title, description, created_at, updated_at)
		VALUES ('task-ft-1', 'ws-1', 'Fall-through Task', 'desc', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)
	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned,
		`{"task_id":"task-ft-1"}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	service.RunSchedulerTick(svc, ctx)

	if dispatcher.callCount() != 1 {
		t.Fatalf("expected routing dispatcher consulted; got %d calls", dispatcher.callCount())
	}
	if mock.callCount() != 1 {
		t.Fatalf("expected legacy StartTask called after routing fall-through; got %d", mock.callCount())
	}
	if mock.lastCall().Prompt == "" {
		t.Error("legacy StartTask received empty prompt after routing fall-through")
	}
}

// TestSchedulerIntegration_RoutingParked_LeavesAgentIdle is the regression
// test for DR-14 review round 1 Finding 1: a routing dispatcher that parks
// a run (no candidate, waiting for capacity, parkRunMaxAttempts) never
// invokes an adapter, so no AgentCompleted/AgentStopped/AgentFailed event
// will ever arrive to clear the agent's "working" status. launchAgent must
// report launched=false for this outcome so prepareAndLaunch clears it.
func TestSchedulerIntegration_RoutingParked_LeavesAgentIdle(t *testing.T) {
	mock := &mockTaskStarter{}
	svc := newTestService(t, service.ServiceOptions{TaskStarter: mock})
	dispatcher := &captureDispatcher{parked: true}
	svc.SetRoutingDispatcher(dispatcher)
	ctx := context.Background()

	agent := &models.AgentInstance{
		ID:                 "routing-parked-1",
		WorkspaceID:        "ws-1",
		Name:               "parked-worker",
		Role:               models.AgentRoleWorker,
		Status:             models.AgentStatusIdle,
		ExecutorPreference: `{"type":"worktree"}`,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	svc.ExecSQL(t, `INSERT INTO tasks (id, workspace_id, title, description, created_at, updated_at)
		VALUES ('task-parked-1', 'ws-1', 'Parked Task', 'desc', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)
	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned,
		`{"task_id":"task-parked-1"}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	service.RunSchedulerTick(svc, ctx)

	if dispatcher.callCount() != 1 {
		t.Fatalf("expected routing dispatcher consulted; got %d calls", dispatcher.callCount())
	}
	if mock.callCount() != 0 {
		t.Fatalf("legacy StartTask must not run for a parked dispatch; got %d calls", mock.callCount())
	}
	assertAgentStatus(t, svc, ctx, agent.ID, models.AgentStatusIdle,
		"after a parked routing dispatch (no adapter was ever invoked)")
}

// TestSchedulerIntegration_RoutingDispatchError_LeavesAgentIdle is the
// regression test for DR-14 review round 1 Finding 2: a routing dispatch
// error is handled via HandleRunFailure (retry or eventual escalation),
// never invokes an adapter, and must not leave the agent showing "working".
func TestSchedulerIntegration_RoutingDispatchError_LeavesAgentIdle(t *testing.T) {
	mock := &mockTaskStarter{}
	svc := newTestService(t, service.ServiceOptions{TaskStarter: mock})
	dispatcher := &captureDispatcher{err: errors.New("provider unavailable")}
	svc.SetRoutingDispatcher(dispatcher)
	ctx := context.Background()

	agent := &models.AgentInstance{
		ID:                 "routing-error-1",
		WorkspaceID:        "ws-1",
		Name:               "error-worker",
		Role:               models.AgentRoleWorker,
		Status:             models.AgentStatusIdle,
		ExecutorPreference: `{"type":"worktree"}`,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	svc.ExecSQL(t, `INSERT INTO tasks (id, workspace_id, title, description, created_at, updated_at)
		VALUES ('task-routing-err-1', 'ws-1', 'Routing Error Task', 'desc', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)
	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned,
		`{"task_id":"task-routing-err-1"}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	service.RunSchedulerTick(svc, ctx)

	if dispatcher.callCount() != 1 {
		t.Fatalf("expected routing dispatcher consulted; got %d calls", dispatcher.callCount())
	}
	if mock.callCount() != 0 {
		t.Fatalf("legacy StartTask must not run after a routing dispatch error; got %d calls", mock.callCount())
	}
	assertAgentStatus(t, svc, ctx, agent.ID, models.AgentStatusIdle,
		"after a routing dispatch error (no adapter was ever invoked)")
}
