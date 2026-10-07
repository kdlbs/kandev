//go:build unix

package executor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-MANAGED-CLONE-RELOCATION-001.1, AC-TASKS-MANAGED-CLONE-RELOCATION-001.7
func TestCompletedRelocationExecutorContinuity(t *testing.T) {
	fixture := prepareCompletedRelocationExecutorFixture(t)
	ctx := context.Background()
	var launched *LaunchAgentRequest
	agentManager := &mockAgentManager{launchAgentFunc: func(_ context.Context, request *LaunchAgentRequest) (*LaunchAgentResponse, error) {
		launched = request
		return &LaunchAgentResponse{AgentExecutionID: "execution-completed-relocation", Status: "starting"}, nil
	}}
	executor := newTestExecutor(t, agentManager, fixture.taskRepo)
	executor.SetRepoCloner(fixture.cloner, nil)
	executor.SetSelectedWorktreeRecoveryAdmission(fixture.manager.AdmitRecovery)
	session, err := fixture.taskRepo.GetTaskSession(ctx, fixture.sessionID)
	require.NoError(t, err)

	initial, err := executor.PreflightSessionWorktreeRecovery(ctx, fixture.taskID, session, false)
	require.NoError(t, err)
	require.NotNil(t, initial, "the initial source-clone transfer should publish a replacement")
	require.NoError(t, initial.Release(ctx))
	historicalHead := strings.TrimSpace(runExecutorRecoveryGit(t, fixture.sourceClone, "rev-parse", "refs/heads/"+fixture.branch))

	environment, err := fixture.taskRepo.GetTaskEnvironment(ctx, fixture.environmentID)
	require.NoError(t, err)
	require.Len(t, environment.Repos, 1)
	replacementPath, replacementID := environment.Repos[0].WorktreePath, environment.Repos[0].WorktreeID
	recordPath := registeredRelocationRecordPath(
		t, ctx, fixture.store, fixture.environmentID, replacementID, replacementPath,
	)
	recordBefore, err := os.ReadFile(recordPath)
	require.NoError(t, err)
	require.Contains(t, string(recordBefore), `"state":"complete"`)

	if err := os.WriteFile(filepath.Join(replacementPath, "after-relocation.txt"), []byte("current user work\n"), 0o644); err != nil {
		t.Fatalf("write post-relocation change: %v", err)
	}
	runExecutorRecoveryGit(t, replacementPath, "add", "after-relocation.txt")
	runExecutorRecoveryGit(t, replacementPath, "commit", "-m", "work after relocation")
	currentHead := strings.TrimSpace(runExecutorRecoveryGit(t, replacementPath, "rev-parse", "HEAD"))
	require.NotEqual(t, historicalHead, currentHead)
	statusBefore := runExecutorRecoveryGit(t, replacementPath, "status", "--porcelain=v1", "--untracked-files=all", "--ignored=traditional")
	if err := os.RemoveAll(fixture.sourceClone); err != nil {
		t.Fatalf("remove former source clone: %v", err)
	}

	manager, err := worktree.NewManager(fixture.config, fixture.store, logger.Default())
	require.NoError(t, err)
	fixture.manager = manager
	executor.SetSelectedWorktreeRecoveryAdmission(manager.AdmitRecovery)
	session, err = fixture.taskRepo.GetTaskSession(ctx, fixture.sessionID)
	require.NoError(t, err)
	preflight, err := executor.PreflightSessionWorktreeRecovery(ctx, fixture.taskID, session, false)
	require.NoError(t, err)
	require.Nil(t, preflight, "a valid completed replacement should not start another recovery operation")

	_, err = executor.ResumeSessionWithOptions(ctx, session, true, ResumeOptions{})
	require.NoError(t, err)
	require.Equal(t, 1, agentManager.launchAgentCallCount)
	require.NotNil(t, launched)
	require.Equal(t, fixture.sessionID, launched.SessionID)
	require.Equal(t, fixture.environmentID, launched.TaskEnvironmentID)
	require.Equal(t, "provider-conversation-kept", launched.ACPSessionID)

	persisted, err := fixture.store.GetWorktreeByID(ctx, replacementID)
	require.NoError(t, err)
	require.NotNil(t, persisted)
	require.Equal(t, replacementPath, persisted.Path)
	require.Equal(t, currentHead, strings.TrimSpace(runExecutorRecoveryGit(t, replacementPath, "rev-parse", "HEAD")))
	require.Equal(t, statusBefore, runExecutorRecoveryGit(t, replacementPath, "status", "--porcelain=v1", "--untracked-files=all", "--ignored=traditional"))
	recordAfter, err := os.ReadFile(recordPath)
	require.NoError(t, err)
	require.Equal(t, recordBefore, recordAfter)
	contents, err := os.ReadFile(filepath.Join(replacementPath, "after-relocation.txt"))
	require.NoError(t, err)
	require.Equal(t, "current user work\n", string(contents))
}

// @covers AC-TASKS-MANAGED-CLONE-RELOCATION-001.3, AC-TASKS-MANAGED-CLONE-RELOCATION-001.7
func TestEditedMaterializedRelocationRefusesExecutorStartup(t *testing.T) {
	fixture := prepareCompletedRelocationExecutorFixture(t)
	ctx := context.Background()
	agentManager := &mockAgentManager{}
	executor := newTestExecutor(t, agentManager, fixture.taskRepo)
	executor.SetRepoCloner(fixture.cloner, nil)
	executor.SetSelectedWorktreeRecoveryAdmission(fixture.manager.AdmitRecovery)
	session, err := fixture.taskRepo.GetTaskSession(ctx, fixture.sessionID)
	require.NoError(t, err)
	initial, err := executor.PreflightSessionWorktreeRecovery(ctx, fixture.taskID, session, false)
	require.NoError(t, err)
	require.NotNil(t, initial)
	require.NoError(t, initial.Release(ctx))
	environment, err := fixture.taskRepo.GetTaskEnvironment(ctx, fixture.environmentID)
	require.NoError(t, err)
	replacement := environment.Repos[0].WorktreePath
	recordPath := registeredRelocationRecordPath(
		t, ctx, fixture.store, fixture.environmentID, environment.Repos[0].WorktreeID, replacement,
	)
	data, err := os.ReadFile(recordPath)
	require.NoError(t, err)
	require.Contains(t, string(data), `"state":"complete"`)
	data = []byte(strings.Replace(string(data), `"state":"complete"`, `"state":"materialized"`, 1))
	require.NoError(t, os.WriteFile(recordPath, data, 0o600))
	if err := os.WriteFile(filepath.Join(replacement, "unfinished.txt"), []byte("unpublished edit\n"), 0o644); err != nil {
		t.Fatalf("write unfinished replacement change: %v", err)
	}
	runExecutorRecoveryGit(t, replacement, "add", "unfinished.txt")
	runExecutorRecoveryGit(t, replacement, "commit", "-m", "change unfinished replacement")
	journalBefore, err := os.ReadFile(recordPath)
	require.NoError(t, err)
	worktreeBefore, err := fixture.store.GetWorktreeByID(ctx, environment.Repos[0].WorktreeID)
	require.NoError(t, err)

	manager, err := worktree.NewManager(fixture.config, fixture.store, logger.Default())
	require.NoError(t, err)
	executor.SetSelectedWorktreeRecoveryAdmission(manager.AdmitRecovery)
	session, err = fixture.taskRepo.GetTaskSession(ctx, fixture.sessionID)
	require.NoError(t, err)
	if _, err := executor.ResumeSessionWithOptions(ctx, session, true, ResumeOptions{}); err == nil {
		t.Fatal("resume accepted a changed materialized replacement")
	}
	require.Zero(t, agentManager.launchAgentCallCount, "provider startup must follow successful recovery admission")
	journalAfter, err := os.ReadFile(recordPath)
	require.NoError(t, err)
	require.Equal(t, journalBefore, journalAfter)
	worktreeAfter, err := fixture.store.GetWorktreeByID(ctx, environment.Repos[0].WorktreeID)
	require.NoError(t, err)
	require.Equal(t, worktreeBefore.Path, worktreeAfter.Path)
	running, err := fixture.taskRepo.GetExecutorRunningBySessionID(ctx, fixture.sessionID)
	require.NoError(t, err)
	require.Equal(t, "provider-conversation-kept", running.ResumeToken)
	require.FileExists(t, filepath.Join(replacement, "unfinished.txt"))
}

func prepareCompletedRelocationExecutorFixture(t *testing.T) *executorPermissionRecoveryFixture {
	t.Helper()
	fixture := newExecutorPermissionRecoveryFixture(t)
	for _, path := range []string{
		filepath.Join(fixture.worktreePath, "group-writable.txt"),
		filepath.Join(fixture.worktreePath, "dirty-link"),
		fixture.worktreePath + ".kandev-clone-relocation.json",
		fixture.worktreePath + ".kandev-recovery.json",
		fixture.oldSnapshot,
		fixture.replacement,
	} {
		if path == fixture.replacement {
			runExecutorRecoveryGit(t, fixture.repositoryPath, "worktree", "remove", "--force", path)
		} else if err := os.RemoveAll(path); err != nil {
			t.Fatalf("remove legacy recovery fixture %q: %v", path, err)
		}
	}
	session, err := fixture.taskRepo.GetTaskSession(context.Background(), fixture.sessionID)
	if err != nil {
		t.Fatalf("load SQLite session: %v", err)
	}
	session.State = models.TaskSessionStateFailed
	session.Metadata = map[string]interface{}{models.SessionMetaKeyLastAgentError: models.LastAgentError{
		Message: "Published replacement commit could not be verified", OccurredAt: time.Now().UTC(),
		Scope: models.ErrorScopeSession, Code: models.LaunchErrorCategoryManagedCloneRelocationRequired,
		RecoveryActions: []string{models.RecoveryActionRelocateAndResume}, StampValue: "completed-relocation-failure",
	}}
	if err := fixture.taskRepo.UpdateTaskSession(context.Background(), session); err != nil {
		t.Fatalf("seed failed session recovery state: %v", err)
	}
	if err := fixture.taskRepo.UpsertExecutorRunning(context.Background(), &models.ExecutorRunning{
		ID: fixture.sessionID, SessionID: fixture.sessionID, TaskID: fixture.taskID,
		ResumeToken: "provider-conversation-kept", Resumable: true, Status: models.ExecutorRunningStatusStopped,
	}); err != nil {
		t.Fatalf("seed retained provider conversation: %v", err)
	}
	return fixture
}

func registeredRelocationRecordPath(
	t *testing.T,
	ctx context.Context,
	store *worktree.SQLiteStore,
	environmentID, replacementID, replacementPath string,
) string {
	t.Helper()
	artifacts, err := store.ListTaskEnvironmentRecoveryArtifacts(ctx, environmentID)
	require.NoError(t, err)
	for _, artifact := range artifacts {
		if artifact.LayoutVersion != 2 || artifact.ReplacementID != replacementID || artifact.ReplacementPath != replacementPath {
			continue
		}
		for _, path := range artifact.ArtifactPaths {
			if filepath.Base(path) == "relocation.json" && filepath.Base(filepath.Dir(path)) == "records" {
				return path
			}
		}
	}
	require.FailNow(t, "private relocation record is missing from the recovery artifact registry")
	return ""
}
