package executor

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// TestResumeSession_DoesNotReorderWaitingForInput verifies that resuming a
// session that was previously in WAITING_FOR_INPUT does not bump task or
// session state and does not bump updated_at timestamps. This guards against
// the resume sequence transiently moving the task to the top of the sidebar.
func TestResumeSession_DoesNotReorderWaitingForInput(t *testing.T) {
	repo := newMockRepository()

	// Fixed timestamp in the past — must remain unchanged after resume.
	originalTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	repo.tasks["task-1"] = &models.Task{
		ID:          "task-1",
		WorkspaceID: "ws-1",
		Title:       "Resumable",
		State:       v1.TaskStateReview,
		CreatedAt:   originalTime,
		UpdatedAt:   originalTime,
	}

	session := &models.TaskSession{
		ID:             "sess-1",
		TaskID:         "task-1",
		AgentProfileID: "profile-1",
		ExecutorID:     "exec-1", // pre-set so applyExecutorConfigToResumeRequest does not bump
		State:          models.TaskSessionStateWaitingForInput,
		StartedAt:      originalTime,
		UpdatedAt:      originalTime,
	}
	repo.sessions[session.ID] = session

	// Track when the async agent-start callback finishes by hooking
	// UpdateTaskState — startAgentProcessOnResume calls updateTaskState
	// after StartAgentProcess returns. If the bug is present, that call
	// triggers a write; if the fix is in place, no write happens but we
	// still need a sync point. Use the mock's start hook instead.
	startDone := make(chan struct{})
	var startOnce sync.Once
	agentManager := &mockAgentManager{
		launchAgentFunc: func(_ context.Context, _ *LaunchAgentRequest) (*LaunchAgentResponse, error) {
			return &LaunchAgentResponse{
				AgentExecutionID: "exec-agent-1",
				ContainerID:      "container-1",
			}, nil
		},
		startAgentProcessFunc: func(_ context.Context, _ string) error {
			startOnce.Do(func() { close(startDone) })
			return nil
		},
	}

	exec := newTestExecutor(t, agentManager, repo)

	if _, err := exec.ResumeSession(context.Background(), session, true); err != nil {
		t.Fatalf("ResumeSession failed: %v", err)
	}

	// Wait for the async StartAgentProcess goroutine to run.
	select {
	case <-startDone:
	case <-time.After(2 * time.Second):
		t.Fatal("agent process start was not invoked")
	}
	// Brief grace period for the onSuccess callback (which runs synchronously
	// immediately after StartAgentProcess returns inside the resume goroutine)
	// to complete its writes.
	time.Sleep(50 * time.Millisecond)

	// Re-read state from the mock repo.
	repo.mu.Lock()
	gotSession := repo.sessions["sess-1"]
	gotTask := repo.tasks["task-1"]
	repo.mu.Unlock()

	if gotSession.State != models.TaskSessionStateWaitingForInput {
		t.Errorf("session state changed: got %q, want %q",
			gotSession.State, models.TaskSessionStateWaitingForInput)
	}
	if !gotSession.UpdatedAt.Equal(originalTime) {
		t.Errorf("session updated_at bumped: got %v, want %v",
			gotSession.UpdatedAt, originalTime)
	}
	if gotTask.State != v1.TaskStateReview {
		t.Errorf("task state changed: got %q, want %q",
			gotTask.State, v1.TaskStateReview)
	}
	if !gotTask.UpdatedAt.Equal(originalTime) {
		t.Errorf("task updated_at bumped: got %v, want %v",
			gotTask.UpdatedAt, originalTime)
	}
}
