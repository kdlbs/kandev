package service_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// mockTaskStarter records calls to StartTask and returns a configurable error.
type mockTaskStarter struct {
	mu    sync.Mutex
	calls []startTaskCall
	err   error
}

type fakeAgentTokenMinter struct {
	token string
}

func (f fakeAgentTokenMinter) MintRuntimeJWT(_, _, _, _, _, _ string) (string, error) {
	return f.token, nil
}

type startTaskCall struct {
	TaskID         string
	AgentProfileID string
	Prompt         string
	Env            map[string]string
}

func (m *mockTaskStarter) StartTask(
	_ context.Context, taskID, agentProfileID, _, _ string,
	_ string, prompt, _ string, _ bool, _ []v1.MessageAttachment,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, startTaskCall{
		TaskID:         taskID,
		AgentProfileID: agentProfileID,
		Prompt:         prompt,
	})
	return m.err
}

func (m *mockTaskStarter) StartTaskWithEnv(
	_ context.Context, taskID, agentProfileID, _, _ string,
	_ string, prompt, _ string, _ bool, _ []v1.MessageAttachment, env map[string]string,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, startTaskCall{
		TaskID:         taskID,
		AgentProfileID: agentProfileID,
		Prompt:         prompt,
		Env:            env,
	})
	return m.err
}

func (m *mockTaskStarter) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.calls)
}

func (m *mockTaskStarter) lastCall() startTaskCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls[len(m.calls)-1]
}

func TestSchedulerTick_LaunchesAgent(t *testing.T) {
	mock := &mockTaskStarter{}
	svc := newTestService(t, service.ServiceOptions{TaskStarter: mock})
	ctx := context.Background()

	agent := &models.AgentInstance{
		ID:                 "launch-agent-1",
		WorkspaceID:        "ws-1",
		Name:               "launch-worker",
		Role:               models.AgentRoleWorker,
		Status:             models.AgentStatusIdle,
		ExecutorPreference: `{"type":"worktree"}`,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	// Insert a task so the prompt builder can find it.
	svc.ExecSQL(t, `INSERT INTO tasks (id, workspace_id, title, description, priority, created_at, updated_at)
		VALUES ('task-launch-1', 'ws-1', 'Build API', 'Implement endpoint', 'medium', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)

	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned, `{"task_id":"task-launch-1"}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	service.RunSchedulerTick(svc, ctx)

	if mock.callCount() != 1 {
		t.Fatalf("expected 1 StartTask call, got %d", mock.callCount())
	}

	call := mock.lastCall()
	if call.TaskID != "task-launch-1" {
		t.Errorf("task_id = %q, want %q", call.TaskID, "task-launch-1")
	}
	// After ADR 0005 the agent's row id is the agent_profile_id — they are
	// the same column under the merged schema.
	if call.AgentProfileID != agent.ID {
		t.Errorf("agent_profile_id = %q, want %q", call.AgentProfileID, agent.ID)
	}
	if call.Prompt == "" {
		t.Error("expected non-empty prompt")
	}
}

func TestSchedulerTick_LaunchIncludesRuntimeTokenEnv(t *testing.T) {
	mock := &mockTaskStarter{}
	svc := newTestService(t, service.ServiceOptions{
		TaskStarter: mock,
		APIBaseURL:  "http://localhost:8080/api/v1",
	})
	svc.SetAgentTokenMinter(fakeAgentTokenMinter{token: "run-token"})
	ctx := context.Background()

	agent := &models.AgentInstance{
		ID:                 "token-agent-1",
		WorkspaceID:        "ws-1",
		Name:               "token-worker",
		Role:               models.AgentRoleWorker,
		Status:             models.AgentStatusIdle,
		ExecutorPreference: `{"type":"worktree"}`,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	svc.ExecSQL(t, `INSERT INTO tasks (id, workspace_id, title, description, created_at, updated_at)
		VALUES ('task-token-1', 'ws-1', 'Build API', 'Implement endpoint', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)
	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned, `{"task_id":"task-token-1","session_id":"sess-token"}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	service.RunSchedulerTick(svc, ctx)

	call := mock.lastCall()
	if call.Env["KANDEV_API_KEY"] != "run-token" {
		t.Fatalf("KANDEV_API_KEY = %q, want run-token", call.Env["KANDEV_API_KEY"])
	}
	if call.Env["KANDEV_RUN_TOKEN"] != "run-token" {
		t.Fatalf("KANDEV_RUN_TOKEN = %q, want run-token", call.Env["KANDEV_RUN_TOKEN"])
	}
	if call.Env["KANDEV_RUN_ID"] == "" {
		t.Fatal("expected KANDEV_RUN_ID")
	}
}

func TestSchedulerTick_SnapshotsRunSkills(t *testing.T) {
	mock := &mockTaskStarter{}
	svc := newTestService(t, service.ServiceOptions{TaskStarter: mock})
	ctx := context.Background()

	skill := &models.Skill{
		WorkspaceID: "ws-1",
		Name:        "Review",
		Slug:        "review",
		SourceType:  service.SkillSourceTypeInline,
		Content:     "# Review\n",
	}
	if err := svc.CreateSkill(ctx, skill); err != nil {
		t.Fatalf("create skill: %v", err)
	}
	agent := &models.AgentInstance{
		ID:                 "skill-agent-1",
		WorkspaceID:        "ws-1",
		Name:               "skill-worker",
		Role:               models.AgentRoleWorker,
		Status:             models.AgentStatusIdle,
		DesiredSkills:      `["review"]`,
		ExecutorPreference: `{"type":"worktree"}`,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	svc.ExecSQL(t, `INSERT INTO tasks (id, workspace_id, title, description, created_at, updated_at)
		VALUES ('task-skill-1', 'ws-1', 'Review API', 'Review endpoint', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)
	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned, `{"task_id":"task-skill-1"}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	service.RunSchedulerTick(svc, ctx)

	runs, err := svc.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	snaps, err := svc.ListRunSkillSnapshotsForTest(ctx, runs[0].ID)
	if err != nil {
		t.Fatalf("list run skill snapshots: %v", err)
	}
	if len(snaps) != 1 {
		t.Fatalf("snapshots = %d, want 1", len(snaps))
	}
	if snaps[0].SkillID != skill.ID || snaps[0].ContentHash != skill.ContentHash || snaps[0].Version != skill.Version {
		t.Fatalf("snapshot = %#v, skill = %#v", snaps[0], skill)
	}
}

func TestSchedulerTick_KeepsRunClaimedUntilAgentCompletes(t *testing.T) {
	mock := &mockTaskStarter{}
	svc := newTestService(t, service.ServiceOptions{TaskStarter: mock})
	svc.SetSyncHandlers(true) // handlers must complete before assertions
	ctx := context.Background()
	eb := bus.NewMemoryEventBus(logger.Default())
	if err := svc.RegisterEventSubscribers(eb); err != nil {
		t.Fatalf("register subscribers: %v", err)
	}

	// Wave G: AgentInstance.ID is the agent_profiles row id; the previous
	// AgentProfileID alias is gone, so seed the id directly.
	agent := &models.AgentInstance{
		ID:                 "profile-abc",
		WorkspaceID:        "ws-1",
		Name:               "lifecycle-worker",
		Role:               models.AgentRoleWorker,
		Status:             models.AgentStatusIdle,
		ExecutorPreference: `{"type":"worktree"}`,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	svc.ExecSQL(t, `INSERT INTO tasks (id, workspace_id, title, description, created_at, updated_at)
		VALUES ('task-life-1', 'ws-1', 'Build API', 'Implement endpoint', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)
	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned, `{"task_id":"task-life-1"}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	service.RunSchedulerTick(svc, ctx)

	runs, err := svc.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 || runs[0].Status != service.RunStatusClaimed {
		t.Fatalf("run after launch = %#v, want exactly one claimed run", runs)
	}

	event := bus.NewEvent(events.AgentCompleted, "test", map[string]string{
		"task_id":    "task-life-1",
		"session_id": "session-1",
	})
	if err := eb.Publish(ctx, events.AgentCompleted, event); err != nil {
		t.Fatalf("publish agent completed: %v", err)
	}

	runs, err = svc.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list runs after complete: %v", err)
	}
	if runs[0].Status != service.RunStatusFinished {
		t.Fatalf("run status after completion = %q, want finished", runs[0].Status)
	}
}

// TestSchedulerTick_AgentStoppedFinishesRun pins the regression where
// the office fire-and-forget turn-complete handler calls StopAgent (which
// publishes events.AgentStopped, NOT events.AgentCompleted). Without
// subscribing to AgentStopped, the run stays in 'claimed' state forever
// and the same-agent eligibility filter in ClaimNextEligibleRun silently
// drops every subsequent run (comments, status changes, mentions) for
// that agent on that task.
func TestSchedulerTick_AgentStoppedFinishesRun(t *testing.T) {
	mock := &mockTaskStarter{}
	svc := newTestService(t, service.ServiceOptions{TaskStarter: mock})
	svc.SetSyncHandlers(true)
	ctx := context.Background()
	eb := bus.NewMemoryEventBus(logger.Default())
	if err := svc.RegisterEventSubscribers(eb); err != nil {
		t.Fatalf("register subscribers: %v", err)
	}

	agent := &models.AgentInstance{
		ID:                 "profile-stop",
		WorkspaceID:        "ws-1",
		Name:               "stop-handler",
		Role:               models.AgentRoleWorker,
		Status:             models.AgentStatusIdle,
		ExecutorPreference: `{"type":"worktree"}`,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	svc.ExecSQL(t, `INSERT INTO tasks (id, workspace_id, title, description, created_at, updated_at)
		VALUES ('task-stop-1', 'ws-1', 'Stop handler test', 'desc', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)
	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned, `{"task_id":"task-stop-1"}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	service.RunSchedulerTick(svc, ctx)

	// AgentStopped (not AgentCompleted) — this is what the office
	// fire-and-forget B4 path publishes via lifecycle.StopAgent.
	event := bus.NewEvent(events.AgentStopped, "test", map[string]string{
		"task_id":    "task-stop-1",
		"session_id": "session-stop-1",
	})
	if err := eb.Publish(ctx, events.AgentStopped, event); err != nil {
		t.Fatalf("publish agent stopped: %v", err)
	}

	runs, err := svc.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if runs[0].Status != service.RunStatusFinished {
		t.Fatalf("run status after AgentStopped = %q, want finished — comments will silently fail until this is fixed",
			runs[0].Status)
	}
}

func TestSchedulerTick_RecoversStaleClaimedRun(t *testing.T) {
	svc := newTestService(t, service.ServiceOptions{})
	ctx := context.Background()

	agent := &models.AgentInstance{
		WorkspaceID:        "ws-1",
		Name:               "stale-worker",
		Role:               models.AgentRoleWorker,
		Status:             models.AgentStatusIdle,
		ExecutorPreference: `{"type":"worktree"}`,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	svc.ExecSQL(t, `
		INSERT INTO runs (
			id, agent_profile_id, reason, payload, status, coalesced_count,
			context_snapshot, retry_count, requested_at, claimed_at
		) VALUES (
			'run-stale-1', ?, 'task_assigned', '{"task_id":"task-stale-1"}',
			'claimed', 1, '{}', 0, datetime('now', '-35 minutes'), datetime('now', '-35 minutes')
		)
	`, agent.ID)

	service.RunSchedulerTick(svc, ctx)

	runs, err := svc.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("run count = %d, want 1", len(runs))
	}
	if runs[0].Status != service.RunStatusQueued {
		t.Fatalf("run status = %q, want queued", runs[0].Status)
	}
}

func TestSchedulerTick_StartTaskError_TriggersRetry(t *testing.T) {
	mock := &mockTaskStarter{err: fmt.Errorf("container start failed")}
	svc := newTestService(t, service.ServiceOptions{TaskStarter: mock})
	ctx := context.Background()

	agent := &models.AgentInstance{
		WorkspaceID:        "ws-1",
		Name:               "fail-worker",
		Role:               models.AgentRoleWorker,
		Status:             models.AgentStatusIdle,
		ExecutorPreference: `{"type":"worktree"}`,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	svc.ExecSQL(t, `INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES ('task-fail-1', 'ws-1', 'Failing Task', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)

	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned, `{"task_id":"task-fail-1"}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	service.RunSchedulerTick(svc, ctx)

	if mock.callCount() != 1 {
		t.Fatalf("expected 1 StartTask call, got %d", mock.callCount())
	}

	// The run should have been retried (back in the queue with retry_count=1).
	// Advance the retry time so it becomes claimable.
	svc.ExecSQL(t, `UPDATE runs SET scheduled_retry_at = datetime('now', '-1 second')`)

	next, err := svc.ClaimNextRun(ctx)
	if err != nil {
		t.Fatalf("claim after failure: %v", err)
	}
	if next == nil {
		t.Fatal("expected retried run to be claimable")
	}
	if next.RetryCount != 1 {
		t.Errorf("retry_count = %d, want 1", next.RetryCount)
	}
}

func TestSchedulerTick_NoTaskStarter_FinishesWithoutError(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	// Intentionally do NOT call SetTaskStarter.

	agent := &models.AgentInstance{
		WorkspaceID:        "ws-1",
		Name:               "noop-worker",
		Role:               models.AgentRoleWorker,
		Status:             models.AgentStatusIdle,
		ExecutorPreference: `{"type":"worktree"}`,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	svc.ExecSQL(t, `INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES ('task-noop-1', 'ws-1', 'NoOp Task', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)

	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned, `{"task_id":"task-noop-1"}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	service.RunSchedulerTick(svc, ctx)

	// Run should be consumed (finished), queue empty.
	next, err := svc.ClaimNextRun(ctx)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if next != nil {
		t.Error("expected queue to be empty after tick with no task starter")
	}
}
