package orchestrator

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
)

type exactProfileChangingAgentManager struct {
	*mockAgentManager
	onResolve func()
	once      sync.Once
}

func (m *exactProfileChangingAgentManager) ResolveAgentProfile(ctx context.Context, profileID string) (*executor.AgentProfileInfo, error) {
	m.once.Do(m.onResolve)
	return m.mockAgentManager.ResolveAgentProfile(ctx, profileID)
}

func TestStartCreatedSessionDenialReceiptUsesObservedAssignmentDuringReplacement(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateCreated)
	task, err := repo.GetTask(ctx, "task1")
	if err != nil {
		t.Fatal(err)
	}
	task.WorkspaceID = "ws1"
	if err := repo.UpdateTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	const revision int64 = 42
	seedExactProfileBindingSnapshot(t, repo, revision)
	first := &models.ExactProfileAssignment{TaskID: "task1", WorkspaceID: "ws1", AgentProfileID: "profile-exact", ProfileRevision: revision, Generation: 1}
	if _, err := repo.AssignExactProfileAssignment(ctx, first); err != nil {
		t.Fatal(err)
	}
	agentMgr := &exactProfileChangingAgentManager{mockAgentManager: &mockAgentManager{resolveProfileInfo: &executor.AgentProfileInfo{
		ProfileID: "profile-exact", WorkspaceID: "ws1", Enabled: false, Revision: revision, Model: "exact-model",
	}}, onResolve: func() {
		next := *first
		next.Generation = 2
		if _, err := repo.AssignExactProfileAssignment(ctx, &next); err != nil {
			t.Fatal(err)
		}
	}}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	if _, err := svc.StartCreatedSession(ctx, "task1", "session1", "profile-exact", "start", false, false, true, nil, nil); !errors.Is(err, ErrExactProfileAssignmentInvalid) {
		t.Fatalf("launch error = %v, want invalid exact profile", err)
	}
	current, err := repo.GetExactProfileAssignment(ctx, "task1")
	if err != nil {
		t.Fatal(err)
	}
	if current.Generation != 2 {
		t.Fatalf("current generation = %d, want replacement 2", current.Generation)
	}
	receipt, err := repo.GetExactProfileLaunchReceipt(ctx, "task1", "session1")
	if err != nil {
		t.Fatal(err)
	}
	if receipt == nil || receipt.Generation != 1 || receipt.Outcome != models.ExactProfileLaunchOutcomeFailedClosed {
		t.Fatalf("denial receipt = %+v, want observed generation 1", receipt)
	}
}
