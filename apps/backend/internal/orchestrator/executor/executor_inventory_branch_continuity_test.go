package executor

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

type branchContinuityReceiptStore struct {
	*tasksqlite.Repository
	receipt *models.WorkspaceInventoryRecoveryReceipt
}

func (s *branchContinuityReceiptStore) GetWorkspaceInventoryRepairReceiptForRow(
	ctx context.Context, taskID, rowID string,
) (*models.WorkspaceInventoryRecoveryReceipt, error) {
	if rowID == s.receipt.EnvironmentRepoID {
		return s.receipt, nil
	}
	return s.Repository.GetWorkspaceInventoryRepairReceiptForRow(ctx, taskID, rowID)
}

func assertBranchEvolutionRefused(t *testing.T, mutate func(*testing.T, *branchContinuityFixture)) {
	t.Helper()
	for _, entry := range []string{"resume", "prepared launch"} {
		t.Run(entry, func(t *testing.T) {
			f := newBranchContinuityFixture(t)
			f.changeBranch(t, false)
			f.executor.repo = &branchContinuityReceiptStore{Repository: f.repo, receipt: f.receipt}
			mutate(t, f)
			before, err := json.Marshal(f.receipt)
			require.NoError(t, err)
			if entry == "resume" {
				_, err = f.executor.ResumeSessionWithOptions(t.Context(), f.session, true, ResumeOptions{})
				require.ErrorIs(t, err, models.ErrWorkspaceInventoryRecoveryConflict)
			} else {
				task, loadErr := f.repo.GetTask(t.Context(), f.session.TaskID)
				require.NoError(t, loadErr)
				_, err = f.executor.LaunchPreparedSession(t.Context(), task.ToAPI(), f.session.ID,
					LaunchOptions{AgentProfileID: "profile-123", ExecutorID: models.ExecutorIDWorktree, StartAgent: true})
				require.ErrorIs(t, err, models.ErrWorkspaceReuseUnsafe)
			}
			require.Zero(t, f.manager.launchAgentCallCount)
			after, err := json.Marshal(f.receipt)
			require.NoError(t, err)
			require.Equal(t, before, after, "refusal must preserve historical evidence")
		})
	}
}

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-008.12
func TestAttestedBranchEvolutionRequiresCompletedRepair(t *testing.T) {
	tests := map[string]func(*testing.T, *branchContinuityFixture){
		"incomplete":         func(_ *testing.T, f *branchContinuityFixture) { f.receipt.PostRepairVerifiedAt = nil },
		"negative":           func(_ *testing.T, f *branchContinuityFixture) { f.receipt.PostRepairMatched = false },
		"missing evidence":   func(_ *testing.T, f *branchContinuityFixture) { f.receipt.PostRepairEvidence = nil },
		"divergent evidence": func(_ *testing.T, f *branchContinuityFixture) { f.receipt.PostRepairEvidence.IndexHash = "divergent" },
		"inconsistent historical ref": func(_ *testing.T, f *branchContinuityFixture) {
			f.receipt.Preservation.RefName = "refs/heads/other"
			f.receipt.PostRepairEvidence.RefName = f.receipt.Preservation.RefName
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) { assertBranchEvolutionRefused(t, mutate) })
	}
}

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-008.12
func TestAttestedBranchEvolutionRejectsUnprovenIdentity(t *testing.T) {
	tests := map[string]func(*testing.T, *branchContinuityFixture){
		"current branch mismatch": func(t *testing.T, f *branchContinuityFixture) {
			runExecutorGit(t, f.row.WorktreePath, "checkout", "-b", "unrecorded")
		},
		"detached HEAD": func(t *testing.T, f *branchContinuityFixture) {
			runExecutorGit(t, f.row.WorktreePath, "checkout", "--detach")
		},
		"missing Git registration": func(t *testing.T, f *branchContinuityFixture) {
			require.NoError(t, os.Remove(filepath.Join(f.row.WorktreePath, ".git")))
		},
		"changed worktree": func(_ *testing.T, f *branchContinuityFixture) { f.receipt.Preservation.WorktreeID = "other" },
		"changed owner":    func(_ *testing.T, f *branchContinuityFixture) { f.receipt.TaskID = "other" },
		"changed slot":     func(_ *testing.T, f *branchContinuityFixture) { f.receipt.TaskRepositoryID = "other" },
		"changed path":     func(_ *testing.T, f *branchContinuityFixture) { f.receipt.Preservation.PathHash = "other" },
		"unsupported source": func(t *testing.T, f *branchContinuityFixture) {
			repository, err := f.repo.GetRepository(t.Context(), f.row.RepositoryID)
			require.NoError(t, err)
			repository.SourceType = "local"
			require.NoError(t, f.repo.UpdateRepository(t.Context(), repository))
		},
		"symlink checkout": func(t *testing.T, f *branchContinuityFixture) {
			moved := f.row.WorktreePath + "-moved"
			require.NoError(t, os.Rename(f.row.WorktreePath, moved))
			t.Cleanup(func() { _ = os.RemoveAll(moved) })
			require.NoError(t, os.Symlink(moved, f.row.WorktreePath))
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) { assertBranchEvolutionRefused(t, mutate) })
	}
}

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-008.4
func TestAttestedBranchEvolutionKeepsExplicitRetryStrict(t *testing.T) {
	f := newBranchContinuityFixture(t)
	f.changeBranch(t, false)
	_, err := f.executor.ResumeSessionWithOptions(t.Context(), f.session, true,
		ResumeOptions{RepairWorkspaceInventory: true, WorkspaceInventoryIdempotencyKey: "repair"})
	require.ErrorIs(t, err, models.ErrWorkspaceInventoryRecoveryIdempotencyConflict)
	require.Zero(t, f.manager.launchAgentCallCount)
}

func multiBranchContinuityFixture(t *testing.T) (*mockRepository, *Executor, *mockAgentManager, *models.TaskSession) {
	t.Helper()
	first, second := newBranchContinuityFixture(t), newBranchContinuityFixture(t)
	first.changeBranch(t, false)
	second.changeBranch(t, false)
	second.row.ID, second.row.RepositoryID, second.row.Position = "second-row", "second-repository", 1
	second.info.Repository.ID = second.row.RepositoryID
	second.receipt.EnvironmentRepoID, second.receipt.RepositoryID = second.row.ID, second.row.RepositoryID
	second.receipt.TaskRepositoryID, second.receipt.IdempotencyKey = "second-task-repository", "second-repair"
	first.env.WorkspacePath = filepath.Dir(filepath.Dir(first.row.WorktreePath))
	first.env.TaskDirName = "branch-continuity"
	first.env.Repos = []*models.TaskEnvironmentRepo{first.row, second.row}
	repo := newMockRepository()
	seedWorktreeExecutor(repo)
	repo.tasks["task"] = &models.Task{ID: "task", WorkspaceID: "workspace", Title: "Task"}
	repo.sessions[first.session.ID] = first.session
	repo.repositories[first.row.RepositoryID] = first.info.Repository
	repo.repositories[second.row.RepositoryID] = second.info.Repository
	for i, row := range first.env.Repos {
		id := []string{"task-repository", "second-task-repository"}[i]
		repo.taskRepositories[id] = &models.TaskRepository{
			ID: id, TaskID: "task", RepositoryID: row.RepositoryID, BaseBranch: "main", Position: i,
		}
	}
	repo.taskEnvironments[first.env.ID] = first.env
	repo.taskEnvironmentRepos[first.env.ID] = first.env.Repos
	repo.workspaceInventoryReceipts = map[string]*models.WorkspaceInventoryRecoveryReceipt{
		"task\x00repair": first.receipt, "task\x00second-repair": second.receipt,
	}
	manager := &mockAgentManager{}
	return repo, newTestExecutor(t, manager, repo), manager, first.session
}

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-008.12
func TestAttestedBranchEvolutionChecksEverySlot(t *testing.T) {
	for _, state := range []string{"positive", "incomplete", "negative", "deleted", "failed", "duplicate"} {
		t.Run(state, func(t *testing.T) {
			repo, executor, manager, session := multiBranchContinuityFixture(t)
			rows := repo.taskEnvironmentRepos[session.TaskEnvironmentID]
			switch state {
			case "incomplete":
				repo.workspaceInventoryReceipts["task\x00second-repair"].PostRepairVerifiedAt = nil
			case "negative":
				repo.workspaceInventoryReceipts["task\x00second-repair"].PostRepairMatched = false
			case "deleted", "failed":
				rows[1].Status = state
			case "duplicate":
				duplicate := *rows[1]
				duplicate.ID = "duplicate-row"
				rows = append(rows, &duplicate)
				repo.taskEnvironmentRepos[session.TaskEnvironmentID] = rows
				repo.taskEnvironments[session.TaskEnvironmentID].Repos = rows
			}
			_, err := executor.ResumeSessionWithOptions(t.Context(), session, true, ResumeOptions{})
			if state == "positive" {
				require.NoError(t, err)
				require.Equal(t, 1, manager.launchAgentCallCount)
			} else {
				require.Error(t, err)
				require.Zero(t, manager.launchAgentCallCount)
			}
		})
	}
}
