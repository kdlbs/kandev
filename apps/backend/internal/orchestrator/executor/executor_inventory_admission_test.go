package executor

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func inventoryAdmissionFixture(t *testing.T) (*mockRepository, *Executor, *models.TaskSession) {
	t.Helper()
	repositoryPath, worktreePath := createExecutorPreservationFixture(t)
	repo := newMockRepository()
	seedWorktreeExecutor(repo)
	repo.repositories["review-repo"] = &models.Repository{ID: "review-repo", WorkspaceID: "review-workspace", Name: "review", Provider: "github", LocalPath: repositoryPath}
	repo.tasks["review-task"] = &models.Task{ID: "review-task", WorkspaceID: "review-workspace", Title: "Review"}
	repo.taskRepositories["review-tr"] = &models.TaskRepository{ID: "review-tr", TaskID: "review-task", RepositoryID: "review-repo", BaseBranch: "main"}
	row := &models.TaskEnvironmentRepo{ID: "review-row", TaskEnvironmentID: "review-env", RepositoryID: "review-repo", BranchSlug: "stale", WorktreeID: "worktree-recovery", WorktreePath: worktreePath, WorktreeBranch: "feature/recovery", Status: "active"}
	repo.taskEnvironments["review-env"] = &models.TaskEnvironment{ID: "review-env", TaskID: "review-task", ExecutorType: string(models.ExecutorTypeWorktree), Status: models.TaskEnvironmentStatusReady, WorkspacePath: worktreePath, Repos: []*models.TaskEnvironmentRepo{row}}
	repo.taskEnvironmentRepos["review-env"] = []*models.TaskEnvironmentRepo{row}
	s := &models.TaskSession{ID: "review-session", TaskID: "review-task", TaskEnvironmentID: "review-env", RepositoryID: "review-repo", ExecutorID: models.ExecutorIDWorktree, AgentProfileID: "profile-123", State: models.TaskSessionStateFailed, StartedAt: time.Now(), UpdatedAt: time.Now()}
	repo.sessions[s.ID] = s
	return repo, newTestExecutor(t, &mockAgentManager{}, repo), s
}

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-008.4
func TestInventoryAdmissionExplicitRetryRejectsReusedKeyFromDifferentSession(t *testing.T) {
	repo, exec, session := inventoryAdmissionFixture(t)
	task := repo.tasks[session.TaskID].ToAPI()
	opts := ResumeOptions{RepairWorkspaceInventory: true, WorkspaceInventoryIdempotencyKey: "review-key"}
	first, _, _, _, _, err := exec.buildResumeRequestAtCredentialBoundaryWithOptions(context.Background(), task, session, false, nil, opts)
	if err != nil {
		t.Fatal(err)
	}
	if first.WorkspaceInventoryRecoveryReceipt == nil {
		t.Fatal("initial repair missing")
	}
	sibling := *session
	sibling.ID = "review-sibling"
	repo.sessions[sibling.ID] = &sibling
	retry, _, _, _, _, err := exec.buildResumeRequestAtCredentialBoundaryWithOptions(context.Background(), task, &sibling, false, nil, opts)
	if !errors.Is(err, models.ErrWorkspaceInventoryRecoveryIdempotencyConflict) {
		t.Fatalf("expected idempotency conflict for different session; got err=%v returned receipt=%+v", err, retry.WorkspaceInventoryRecoveryReceipt)
	}
}

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-008.10
func TestInventoryAdmissionInheritedResumeRejectsParentNegativeReceipt(t *testing.T) {
	repo, exec, session := inventoryAdmissionFixture(t)
	opts := ResumeOptions{RepairWorkspaceInventory: true, WorkspaceInventoryIdempotencyKey: "review-key"}
	if _, _, _, _, _, err := exec.buildResumeRequestAtCredentialBoundaryWithOptions(context.Background(), repo.tasks[session.TaskID].ToAPI(), session, false, nil, opts); err != nil {
		t.Fatal(err)
	}
	receipt := repo.workspaceInventoryReceipts[session.TaskID+"\x00review-key"]
	receipt.PostRepairMatched = false
	child := &models.Task{ID: "review-child", WorkspaceID: "review-workspace", Title: "Child", Metadata: map[string]interface{}{"workspace": map[string]interface{}{"mode": "inherit_parent"}}}
	repo.tasks[child.ID] = child
	repo.taskRepositories["review-child-tr"] = &models.TaskRepository{ID: "review-child-tr", TaskID: child.ID, RepositoryID: "review-repo", BaseBranch: "main"}
	cs := *session
	cs.ID = "review-child-session"
	cs.TaskID = child.ID
	repo.sessions[cs.ID] = &cs
	req, _, _, _, _, err := exec.buildResumeRequestAtCredentialBoundaryWithOptions(context.Background(), child.ToAPI(), &cs, false, nil, ResumeOptions{})
	if !errors.Is(err, models.ErrWorkspaceInventoryRecoveryConflict) {
		t.Fatalf("expected parent divergent receipt to block inherited resume; err=%v receipt=%+v reuse=%v", err, req.WorkspaceInventoryRecoveryReceipt, req.WorkspaceReuseRequired)
	}
}

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-008.6
func TestInventoryAdmissionRepairRejectsLiveInheritedEnvironmentWriter(t *testing.T) {
	repo, exec, session := inventoryAdmissionFixture(t)
	child := &models.Task{ID: "live-child", WorkspaceID: "review-workspace", Title: "Child"}
	repo.tasks[child.ID] = child
	cs := *session
	cs.ID = "live-child-session"
	cs.TaskID = child.ID
	cs.State = models.TaskSessionStateRunning
	repo.sessions[cs.ID] = &cs
	repo.executorsRunning[cs.ID] = &models.ExecutorRunning{TaskID: cs.TaskID, SessionID: cs.ID, Status: models.ExecutorRunningStatusRunning, WorktreeID: "worktree-recovery", WorktreePath: repo.taskEnvironments["review-env"].WorkspacePath, WorktreeBranch: "feature/recovery"}
	req, _, _, _, _, err := exec.buildResumeRequestAtCredentialBoundaryWithOptions(context.Background(), repo.tasks[session.TaskID].ToAPI(), session, false, nil, ResumeOptions{RepairWorkspaceInventory: true, WorkspaceInventoryIdempotencyKey: "live-writer-key"})
	if !errors.Is(err, models.ErrWorkspaceInventoryRecoveryConflict) {
		t.Fatalf("expected live shared-environment writer to block repair; err=%v receipt=%+v", err, req.WorkspaceInventoryRecoveryReceipt)
	}
}

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-008.4
func TestInventoryAdmissionExactRetryReturnsOriginalReceipt(t *testing.T) {
	repo, executor, session := inventoryAdmissionFixture(t)
	opts := ResumeOptions{RepairWorkspaceInventory: true, WorkspaceInventoryIdempotencyKey: "exact-retry"}
	first, _, _, _, _, err := executor.buildResumeRequestAtCredentialBoundaryWithOptions(context.Background(), repo.tasks[session.TaskID].ToAPI(), session, false, nil, opts)
	if err != nil {
		t.Fatal(err)
	}
	second, _, _, _, _, err := executor.buildResumeRequestAtCredentialBoundaryWithOptions(context.Background(), repo.tasks[session.TaskID].ToAPI(), session, false, nil, opts)
	if err != nil {
		t.Fatal(err)
	}
	if second.WorkspaceInventoryRecoveryReceipt == nil || second.WorkspaceInventoryRecoveryReceipt.ID != first.WorkspaceInventoryRecoveryReceipt.ID || second.WorkspaceInventoryRecoveryReceipt.ResultCode != models.WorkspaceInventoryRecoveryDeduplicated {
		t.Fatalf("retry did not return original deduplicated receipt: %+v", second.WorkspaceInventoryRecoveryReceipt)
	}
	if len(repo.workspaceInventoryReceipts) != 1 {
		t.Fatal("retry appended a receipt")
	}
}

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-008.10
func TestInventoryAdmissionInheritedAttestation(t *testing.T) {
	for _, mode := range []string{"inherit_parent", "shared_group"} {
		t.Run(mode, func(t *testing.T) {
			for _, outcome := range []string{"positive", "incomplete", "divergent", "incomplete_with_writer"} {
				t.Run(outcome, func(t *testing.T) {
					repo, executor, session := inventoryAdmissionFixture(t)
					// A borrower may select a subset with a different local order.
					repo.taskRepositories["review-tr"].Position = 1
					repo.taskEnvironmentRepos["review-env"][0].Position = 1
					ctx := context.Background()
					_, _, _, _, _, err := executor.buildResumeRequestAtCredentialBoundaryWithOptions(ctx, repo.tasks[session.TaskID].ToAPI(), session, false, nil,
						ResumeOptions{RepairWorkspaceInventory: true, WorkspaceInventoryIdempotencyKey: "owner-repair"})
					if err != nil {
						t.Fatal(err)
					}
					receipt := repo.workspaceInventoryReceipts[session.TaskID+"\x00owner-repair"]
					if outcome != "positive" {
						receipt.PostRepairVerifiedAt = nil
						receipt.PostRepairEvidence = nil
					}
					if outcome == "divergent" {
						receipt.Preservation.ContentHash = "changed"
					}
					child := &models.Task{ID: "borrower", WorkspaceID: "review-workspace", Title: "Borrower", Metadata: map[string]interface{}{"workspace": map[string]interface{}{"mode": mode}}}
					repo.tasks[child.ID] = child
					repo.taskRepositories["borrower-tr"] = &models.TaskRepository{ID: "borrower-tr", TaskID: child.ID, RepositoryID: "review-repo", BaseBranch: "main"}
					cs := *session
					cs.ID = "borrower-session"
					cs.TaskID = child.ID
					repo.sessions[cs.ID] = &cs
					if outcome == "incomplete_with_writer" {
						session.State = models.TaskSessionStateRunning
					}
					req, _, _, env, _, err := executor.buildResumeRequestAtCredentialBoundaryWithOptions(ctx, child.ToAPI(), &cs, false, nil, ResumeOptions{})
					if outcome == "divergent" || outcome == "incomplete_with_writer" {
						if !errors.Is(err, models.ErrWorkspaceInventoryRecoveryConflict) {
							t.Fatalf("unsafe inherited resume admitted: %v", err)
						}
						return
					}
					if err != nil {
						t.Fatal(err)
					}
					if req.WorkspaceInventoryRecoveryReceipt == nil || req.WorkspaceInventoryRecoveryReceipt.ID != receipt.ID {
						t.Fatal("owner receipt missing")
					}
					// The fresh-launch gate must also enforce the same owner's receipt.
					repo.workspaceInventoryReceipts[session.TaskID+"\x00owner-repair"].PostRepairMatched = false
					if err := executor.admitLaunchWorkspaceInventory(ctx, child.ToAPI(), &cs, req, env, []*repoInfo{{TaskRepositoryID: "borrower-tr", RepositoryID: "review-repo", RepositoryPath: repo.repositories["review-repo"].LocalPath}}); !errors.Is(err, models.ErrWorkspaceReuseUnsafe) {
						t.Fatalf("unsafe inherited launch admitted: %v", err)
					}
				})
			}
		})
	}
}
