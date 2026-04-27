package service_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/orchestrate/models"
	"github.com/kandev/kandev/internal/orchestrate/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// mockTaskStarter records calls to StartTask and returns a configurable error.
type mockTaskStarter struct {
	mu    sync.Mutex
	calls []startTaskCall
	err   error
}

type startTaskCall struct {
	TaskID         string
	AgentProfileID string
	Prompt         string
}

func (m *mockTaskStarter) StartTask(
	_ context.Context, taskID, agentProfileID, _, _ string,
	_ int, prompt, _ string, _ bool, _ []v1.MessageAttachment,
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
	svc := newTestService(t)
	ctx := context.Background()

	mock := &mockTaskStarter{}
	svc.SetTaskStarter(mock)

	agent := &models.AgentInstance{
		WorkspaceID:        "ws-1",
		Name:               "launch-worker",
		Role:               models.AgentRoleWorker,
		Status:             models.AgentStatusIdle,
		AgentProfileID:     "profile-abc",
		ExecutorPreference: `{"type":"worktree"}`,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	// Insert a task so the prompt builder can find it.
	svc.ExecSQL(t, `INSERT INTO tasks (id, workspace_id, title, description, priority, created_at, updated_at)
		VALUES ('task-launch-1', 'ws-1', 'Build API', 'Implement endpoint', 3, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)

	if err := svc.QueueWakeup(ctx, agent.ID, service.WakeupReasonTaskAssigned, `{"task_id":"task-launch-1"}`, ""); err != nil {
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
	if call.AgentProfileID != "profile-abc" {
		t.Errorf("agent_profile_id = %q, want %q", call.AgentProfileID, "profile-abc")
	}
	if call.Prompt == "" {
		t.Error("expected non-empty prompt")
	}
}

func TestSchedulerTick_StartTaskError_TriggersRetry(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	mock := &mockTaskStarter{err: fmt.Errorf("container start failed")}
	svc.SetTaskStarter(mock)

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

	if err := svc.QueueWakeup(ctx, agent.ID, service.WakeupReasonTaskAssigned, `{"task_id":"task-fail-1"}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	service.RunSchedulerTick(svc, ctx)

	if mock.callCount() != 1 {
		t.Fatalf("expected 1 StartTask call, got %d", mock.callCount())
	}

	// The wakeup should have been retried (back in the queue with retry_count=1).
	// Advance the retry time so it becomes claimable.
	svc.ExecSQL(t, `UPDATE orchestrate_wakeup_queue SET scheduled_retry_at = datetime('now', '-1 second')`)

	next, err := svc.ClaimNextWakeup(ctx)
	if err != nil {
		t.Fatalf("claim after failure: %v", err)
	}
	if next == nil {
		t.Fatal("expected retried wakeup to be claimable")
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

	if err := svc.QueueWakeup(ctx, agent.ID, service.WakeupReasonTaskAssigned, `{"task_id":"task-noop-1"}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	service.RunSchedulerTick(svc, ctx)

	// Wakeup should be consumed (finished), queue empty.
	next, err := svc.ClaimNextWakeup(ctx)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if next != nil {
		t.Error("expected queue to be empty after tick with no task starter")
	}
}
