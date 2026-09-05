package executor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.5
//
// A row-scoped receipt is only valid for the workspace identity captured by
// that repair. Reusing the same environment-row ID after the task's workspace
// identity changes must not let an older positive attestation admit launch.
func TestAttestedWorkspaceInventoryRowsReceiptRejectsReceiptFromDifferentWorkspace(t *testing.T) {
	row, env, receipt := positiveWorkspaceInventoryReceiptFixture(t)
	receipt.WorkspaceID = "workspace-old"
	executor, task, session, req, repositories := workspaceInventoryReceiptGateFixture(row, env, receipt)

	_, err := executor.attestedWorkspaceInventoryRowsReceipt(context.Background(), task, session, req, env, repositories)
	if !errors.Is(err, models.ErrWorkspaceInventoryRecoveryConflict) {
		t.Fatalf("cross-workspace row receipt error = %v, want recovery conflict", err)
	}
}

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.5
func TestAttestedWorkspaceInventoryRowsReceiptRejectsReceiptIdentityDrift(t *testing.T) {
	tests := map[string]func(*models.WorkspaceInventoryRecoveryReceipt){
		"environment": func(receipt *models.WorkspaceInventoryRecoveryReceipt) { receipt.TaskEnvironmentID = "environment-old" },
		"task repository": func(receipt *models.WorkspaceInventoryRecoveryReceipt) {
			receipt.TaskRepositoryID = "task-repository-old"
		},
		"repository": func(receipt *models.WorkspaceInventoryRecoveryReceipt) { receipt.RepositoryID = "repository-old" },
		"branch slot": func(receipt *models.WorkspaceInventoryRecoveryReceipt) {
			receipt.Preservation.ExpectedBranchSlug = "dev"
		},
		"worktree": func(receipt *models.WorkspaceInventoryRecoveryReceipt) {
			receipt.Preservation.WorktreeID = "worktree-old"
		},
		"observed branch": func(receipt *models.WorkspaceInventoryRecoveryReceipt) {
			receipt.Preservation.ObservedBranch = "feature/old"
		},
		"observed ref": func(receipt *models.WorkspaceInventoryRecoveryReceipt) {
			receipt.Preservation.RefName = "refs/heads/feature/old"
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			row, env, receipt := positiveWorkspaceInventoryReceiptFixture(t)
			mutate(receipt)
			postRepair := receipt.Preservation
			receipt.PostRepairEvidence = &postRepair
			executor, task, session, req, repositories := workspaceInventoryReceiptGateFixture(row, env, receipt)

			_, err := executor.attestedWorkspaceInventoryRowsReceipt(context.Background(), task, session, req, env, repositories)
			if !errors.Is(err, models.ErrWorkspaceInventoryRecoveryConflict) {
				t.Fatalf("drifted %s receipt error = %v, want recovery conflict", name, err)
			}
		})
	}
}

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.5
func TestAttestedWorkspaceInventoryRowsReceiptRejectsRepointedCheckoutPath(t *testing.T) {
	row, env, receipt := positiveWorkspaceInventoryReceiptFixture(t)
	originalPath := t.TempDir()
	row.WorktreePath = originalPath
	env.WorkspacePath = originalPath
	receipt.Preservation.PathHash = testWorkspaceInventoryPathHash(t, originalPath)
	postRepair := receipt.Preservation
	receipt.PostRepairEvidence = &postRepair

	repointedPath := t.TempDir()
	row.WorktreePath = repointedPath
	env.WorkspacePath = repointedPath
	executor, task, session, req, repositories := workspaceInventoryReceiptGateFixture(row, env, receipt)

	_, err := executor.attestedWorkspaceInventoryRowsReceipt(context.Background(), task, session, req, env, repositories)
	if !errors.Is(err, models.ErrWorkspaceInventoryRecoveryConflict) {
		t.Fatalf("repointed checkout receipt error = %v, want recovery conflict", err)
	}
}

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.10
func TestAttestedWorkspaceInventoryRowsReceiptRejectsIncompletePositiveAttestation(t *testing.T) {
	tests := map[string]func(*models.WorkspaceInventoryRecoveryReceipt){
		"missing post-repair evidence": func(receipt *models.WorkspaceInventoryRecoveryReceipt) {
			receipt.PostRepairEvidence = nil
		},
		"divergent post-repair evidence": func(receipt *models.WorkspaceInventoryRecoveryReceipt) {
			receipt.PostRepairEvidence.IndexHash = "divergent-index"
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			row, env, receipt := positiveWorkspaceInventoryReceiptFixture(t)
			mutate(receipt)
			executor, task, session, req, repositories := workspaceInventoryReceiptGateFixture(row, env, receipt)

			_, err := executor.attestedWorkspaceInventoryRowsReceipt(context.Background(), task, session, req, env, repositories)
			if !errors.Is(err, models.ErrWorkspaceInventoryRecoveryConflict) {
				t.Fatalf("incomplete positive attestation error = %v, want recovery conflict", err)
			}
		})
	}
}

func TestAttestedWorkspaceInventoryRowsReceiptAcceptsMatchingPositiveReceipt(t *testing.T) {
	row, env, receipt := positiveWorkspaceInventoryReceiptFixture(t)
	executor, task, session, req, repositories := workspaceInventoryReceiptGateFixture(row, env, receipt)

	got, err := executor.attestedWorkspaceInventoryRowsReceipt(context.Background(), task, session, req, env, repositories)
	if err != nil {
		t.Fatalf("matching positive receipt error = %v", err)
	}
	if got == nil || got.IdempotencyKey != receipt.IdempotencyKey || got.EnvironmentRepoID != receipt.EnvironmentRepoID {
		t.Fatalf("matching positive receipt = %#v, want identity from %#v", got, receipt)
	}
}

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.2
func TestRepairReuseEnvironmentInventoryFailsClosedWhenRuntimeEvidenceLookupFails(t *testing.T) {
	repositoryPath, worktreePath := createExecutorPreservationFixture(t)
	now := time.Now().UTC().Truncate(time.Second)
	row := &models.TaskEnvironmentRepo{
		ID: "environment-repo", TaskEnvironmentID: "environment", RepositoryID: "repository",
		BranchSlug: "stale", WorktreeID: "worktree-recovery", WorktreePath: worktreePath,
		WorktreeBranch: "feature/recovery", Position: 0, Status: "active", UpdatedAt: now,
	}
	env := &models.TaskEnvironment{
		ID: row.TaskEnvironmentID, TaskID: "task", WorkspacePath: worktreePath,
		Status: models.TaskEnvironmentStatusReady, UpdatedAt: now,
		Repos: []*models.TaskEnvironmentRepo{row},
	}
	repo := &mockRepository{
		taskEnvironmentRepos:       map[string][]*models.TaskEnvironmentRepo{env.ID: env.Repos},
		workspaceInventoryReceipts: map[string]*models.WorkspaceInventoryRecoveryReceipt{},
		getExecutorRunningFunc: func(context.Context, string) (*models.ExecutorRunning, error) {
			return nil, errors.New("runtime store unavailable")
		},
	}
	executor := &Executor{repo: repo}
	task := &v1.Task{ID: "task", WorkspaceID: "workspace"}
	session := &models.TaskSession{ID: "session", TaskID: task.ID, TaskEnvironmentID: env.ID, State: models.TaskSessionStateFailed}
	req := &LaunchAgentRequest{
		TaskID: task.ID, WorkspaceID: task.WorkspaceID, UseWorktree: true, WorkspaceReuseRequired: true,
		Repositories: []RepoSpec{{TaskRepositoryID: "task-repository", RepositoryID: row.RepositoryID, BranchIdentitySlug: "main"}},
	}
	repositories := []*repoInfo{{
		TaskRepositoryID: "task-repository", TaskRepositoryUpdatedAt: now,
		RepositoryID: row.RepositoryID, RepositoryPath: repositoryPath, Position: 0,
		Repository: &models.Repository{ID: row.RepositoryID, SourceType: "github"},
	}}

	_, err := executor.repairReuseEnvironmentInventory(context.Background(), task, session, req, env, repositories, "runtime-error")
	if !errors.Is(err, models.ErrWorkspaceInventoryRecoveryConflict) {
		t.Fatalf("runtime evidence lookup error = %v, want recovery conflict", err)
	}
	if len(repo.workspaceInventoryReceipts) != 0 {
		t.Fatalf("runtime evidence lookup failure committed %d receipt(s)", len(repo.workspaceInventoryReceipts))
	}
}

func positiveWorkspaceInventoryReceiptFixture(t *testing.T) (*models.TaskEnvironmentRepo, *models.TaskEnvironment, *models.WorkspaceInventoryRecoveryReceipt) {
	t.Helper()
	verifiedAt := time.Now().UTC()
	worktreePath := t.TempDir()
	row := &models.TaskEnvironmentRepo{
		ID: "environment-repo", TaskEnvironmentID: "environment", RepositoryID: "repository",
		BranchSlug: "main", WorktreeID: "worktree-recovery", WorktreePath: worktreePath,
		WorktreeBranch: "feature/recovery", Position: 0, Status: "active",
	}
	env := &models.TaskEnvironment{
		ID: row.TaskEnvironmentID, TaskID: "task", WorkspacePath: row.WorktreePath,
		Status: models.TaskEnvironmentStatusReady, Repos: []*models.TaskEnvironmentRepo{row},
	}
	receipt := &models.WorkspaceInventoryRecoveryReceipt{
		TaskID: "task", WorkspaceID: "workspace", SessionID: "session-old",
		TaskEnvironmentID: row.TaskEnvironmentID, TaskRepositoryID: "task-repository",
		EnvironmentRepoID: row.ID, RepositoryID: row.RepositoryID,
		IdempotencyKey: "old-repair", PostRepairMatched: true, PostRepairVerifiedAt: &verifiedAt,
		Preservation: models.WorkspaceInventoryPreservation{
			ExpectedBranchSlug: row.BranchSlug, ObservedBranch: row.WorktreeBranch,
			RefName: "refs/heads/" + row.WorktreeBranch, WorktreeID: row.WorktreeID,
			PathHash: testWorkspaceInventoryPathHash(t, worktreePath),
		},
	}
	postRepair := receipt.Preservation
	receipt.PostRepairEvidence = &postRepair
	return row, env, receipt
}

func workspaceInventoryReceiptGateFixture(
	row *models.TaskEnvironmentRepo,
	env *models.TaskEnvironment,
	receipt *models.WorkspaceInventoryRecoveryReceipt,
) (*Executor, *v1.Task, *models.TaskSession, *LaunchAgentRequest, []*repoInfo) {
	repo := &mockRepository{workspaceInventoryReceipts: map[string]*models.WorkspaceInventoryRecoveryReceipt{
		"task\x00old-repair": receipt,
	}}
	task := &v1.Task{ID: "task", WorkspaceID: "workspace"}
	session := &models.TaskSession{ID: "session-current", TaskID: task.ID, TaskEnvironmentID: env.ID}
	req := &LaunchAgentRequest{
		TaskID: task.ID, WorkspaceID: task.WorkspaceID, UseWorktree: true, WorkspaceReuseRequired: true,
		Repositories: []RepoSpec{{TaskRepositoryID: "task-repository", RepositoryID: row.RepositoryID, BranchIdentitySlug: row.BranchSlug}},
	}
	repositories := []*repoInfo{{TaskRepositoryID: "task-repository", RepositoryID: row.RepositoryID, Position: 0}}
	return &Executor{repo: repo}, task, session, req, repositories
}

func testWorkspaceInventoryPathHash(t *testing.T, path string) string {
	t.Helper()
	canonical, err := worktree.CanonicalDirectory(path)
	if err != nil {
		t.Fatalf("canonical test checkout path: %v", err)
	}
	sum := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(sum[:])
}
