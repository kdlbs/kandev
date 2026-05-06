package service_test

import (
	"context"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/service"
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
	// launched controls the (launched, parked, err) tuple the fake
	// returns. Default zero-value is (false, false, nil) — routing
	// fall-through — so callers don't need to set it for the assertion
	// path tested here.
	launched bool
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
	return c.launched, false, nil
}

func (c *captureDispatcher) HandlePostStartFailure(
	_ context.Context, _ *models.Run, _ *models.AgentInstance, _ string,
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
