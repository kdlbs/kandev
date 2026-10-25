package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type completionGateTaskRepo struct {
	*mockTaskRepo
	snapshot *models.TaskCompletionGateSnapshot
	err      error
}

func (r *completionGateTaskRepo) GetTaskCompletionGate(context.Context, string) (*models.TaskCompletionGateSnapshot, error) {
	return r.snapshot, r.err
}

type completionGateLSPTracker struct {
	stopped bool
}

func (*completionGateLSPTracker) HasActiveLSPLease(string) bool             { return false }
func (*completionGateLSPTracker) HasActiveLSPLeaseForExecution(string) bool { return false }
func (*completionGateLSPTracker) StopLSPLeasesForSession(string)            {}
func (l *completionGateLSPTracker) StopLSPLeasesForTask(string)             { l.stopped = true }
func (*completionGateLSPTracker) StopLSPLeasesForExecution(string)          {}

func TestCompleteTask_CompletionGateBlocksBeforeStoppingAgents(t *testing.T) {
	repo := setupTestRepo(t)
	mockRepo := newMockTaskRepo()
	taskRepo := &completionGateTaskRepo{
		mockTaskRepo: mockRepo,
		snapshot:     &models.TaskCompletionGateSnapshot{TaskID: "task1", Blocked: true},
	}
	manager := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), mockRepo, manager)
	svc.taskRepo = taskRepo
	svc.executor = executor.NewExecutor(manager, repo, testLogger(), executor.ExecutorConfig{})
	lsp := &completionGateLSPTracker{}
	svc.SetLSPLeaseLifecycle(lsp)

	err := svc.CompleteTask(context.Background(), "task1")
	if !errors.Is(err, repoerrors.ErrTaskCompletionGateBlocked) {
		t.Fatalf("CompleteTask error = %v, want completion gate blocked", err)
	}
	if lsp.stopped {
		t.Fatal("CompleteTask stopped LSP leases before checking the completion gate")
	}
	if taskRepo.stateWrites["task1"] != 0 {
		t.Fatalf("CompleteTask wrote task state %d times while blocked", taskRepo.stateWrites["task1"])
	}
	if len(manager.stopAgentArgs) != 0 || len(manager.stopAgentWithReasonArgs) != 0 {
		t.Fatalf("CompleteTask stopped agents while blocked: stop=%v reason=%v", manager.stopAgentArgs, manager.stopAgentWithReasonArgs)
	}
	if state := taskRepo.updatedStates["task1"]; state == v1.TaskStateCompleted {
		t.Fatal("CompleteTask completed a task whose gate is blocked")
	}
}
