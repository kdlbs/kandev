package lifecycle

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/worktree"
)

// errorChainPreparer is an EnvironmentPreparer that returns a failed
// EnvPrepareResult carrying a typed error chain. The orchestrator and any
// other consumer downstream of launchApplyPrepareResult must still be able
// to reach the underlying sentinel via errors.Is.
type errorChainPreparer struct {
	err error
}

func (p *errorChainPreparer) Name() string { return "error-chain" }
func (p *errorChainPreparer) Prepare(_ context.Context, _ *EnvPrepareRequest, _ PrepareProgressCallback) (*EnvPrepareResult, error) {
	return &EnvPrepareResult{
		Success:      false,
		ErrorMessage: p.err.Error(),
		Error:        p.err,
		Steps:        []PrepareStep{{Name: "Create worktree", Status: PrepareStepFailed, Error: p.err.Error()}},
	}, nil
}

type directErrorPreparer struct {
	err error
}

func (p *directErrorPreparer) Name() string { return "direct-error" }
func (p *directErrorPreparer) Prepare(_ context.Context, _ *EnvPrepareRequest, _ PrepareProgressCallback) (*EnvPrepareResult, error) {
	return nil, p.err
}

// newErrorChainTestManager builds a Manager just rich enough to call
// launchApplyPrepareResult. Only the eventPublisher field is exercised.
func newErrorChainTestManager(t *testing.T) *Manager {
	t.Helper()
	mgr, _ := createTestManagerWithTracking()
	return mgr
}

func TestLaunchApplyPrepareResult_PreservesTypedChainViaWorktreeSentinel(t *testing.T) {
	mgr := newErrorChainTestManager(t)
	cleanupManagerStopCh(t, mgr)

	// ClassifyGitError wraps with %w so the typed sentinel is at the head of
	// the chain — exactly the path that previously lost identity when
	// env_preparer_worktree flattened the error into ErrorMessage.
	underlying := worktree.ClassifyGitError(
		"fatal: 'feature/foo' is already checked out at '/tmp/repo/.git/worktrees/other'",
		nil,
	)

	var workspacePath, mainRepoGitDir, worktreeID, worktreeBranch string
	err := mgr.launchApplyPrepareResult(
		&LaunchRequest{TaskID: "task-1", SessionID: "session-1"},
		&EnvPrepareResult{
			Success:      false,
			ErrorMessage: underlying.Error(),
			Error:        underlying,
		},
		&workspacePath, &mainRepoGitDir, &worktreeID, &worktreeBranch,
	)
	require.Error(t, err)

	if !errors.Is(err, worktree.ErrBranchCheckedOut) {
		t.Fatalf("errors.Is(err, worktree.ErrBranchCheckedOut) = false; err = %v", err)
	}

	// The formatted message must remain byte-compatible with the prior
	// %s-based wrap: "environment preparation failed: <underlying.Error()>".
	// This protects downstream consumers that grep ErrorMessage / log output
	// for the well-known prefix.
	require.Contains(t, err.Error(), "environment preparation failed: branch is already checked out in another worktree")
}

func TestLaunchApplyPrepareResult_FallsBackToErrorMessageWhenErrorNil(t *testing.T) {
	mgr := newErrorChainTestManager(t)
	cleanupManagerStopCh(t, mgr)

	var workspacePath, mainRepoGitDir, worktreeID, worktreeBranch string
	err := mgr.launchApplyPrepareResult(
		&LaunchRequest{TaskID: "task-2", SessionID: "session-2"},
		&EnvPrepareResult{
			Success:      false,
			ErrorMessage: "no repository path provided",
			// Error intentionally nil — mirrors the validate-step failure
			// path where no typed sentinel exists.
		},
		&workspacePath, &mainRepoGitDir, &worktreeID, &worktreeBranch,
	)
	require.Error(t, err)
	require.Equal(t, "environment preparation failed: no repository path provided", err.Error())
}

func TestLaunchApplyPrepareResult_UsesDisplayMessageAndPreservesCause(t *testing.T) {
	mgr := newErrorChainTestManager(t)
	cleanupManagerStopCh(t, mgr)

	cause := errors.New("raw cause contains internal detail")
	var workspacePath, mainRepoGitDir, worktreeID, worktreeBranch string
	err := mgr.launchApplyPrepareResult(
		&LaunchRequest{TaskID: "task-2b", SessionID: "session-2b"},
		&EnvPrepareResult{
			Success:      false,
			ErrorMessage: "safe display message",
			Error:        cause,
		},
		&workspacePath, &mainRepoGitDir, &worktreeID, &worktreeBranch,
	)

	require.Error(t, err)
	require.Equal(t, "environment preparation failed: safe display message", err.Error())
	require.ErrorIs(t, err, cause)
}

func TestRunEnvironmentPreparerWithProgress_PropagatesTypedErrorChain(t *testing.T) {
	mgr := newErrorChainTestManager(t)
	cleanupManagerStopCh(t, mgr)

	registry := NewPreparerRegistry(mgr.logger)
	registry.Register("worktree", &errorChainPreparer{
		err: worktree.ClassifyGitError(
			"fatal: 'feature/bar' is already used by worktree at '/tmp/repo/.git/worktrees/other'",
			nil,
		),
	})
	mgr.preparerRegistry = registry

	result := mgr.runEnvironmentPreparerWithProgress(
		context.Background(),
		&LaunchRequest{
			TaskID:         "task-3",
			SessionID:      "session-3",
			ExecutorType:   "worktree",
			RepositoryPath: "/tmp/repo",
		},
		"",
		func(PrepareStep, int, int) {},
	)
	require.NotNil(t, result)
	require.False(t, result.Success)
	require.NotNil(t, result.Error)
	require.True(t, errors.Is(result.Error, worktree.ErrBranchCheckedOut),
		"preparer Error must carry the worktree.ErrBranchCheckedOut sentinel")

	// Drive the wrapper too — together these prove the typed sentinel
	// survives end-to-end.
	var workspacePath, mainRepoGitDir, worktreeID, worktreeBranch string
	wrapped := mgr.launchApplyPrepareResult(
		&LaunchRequest{TaskID: "task-3", SessionID: "session-3"},
		result,
		&workspacePath, &mainRepoGitDir, &worktreeID, &worktreeBranch,
	)
	require.Error(t, wrapped)
	require.True(t, errors.Is(wrapped, worktree.ErrBranchCheckedOut),
		"wrapped launch error must still expose worktree.ErrBranchCheckedOut")
}

func TestRunEnvironmentPreparerWithProgress_PreservesDirectErrorChain(t *testing.T) {
	mgr := newErrorChainTestManager(t)
	cleanupManagerStopCh(t, mgr)

	registry := NewPreparerRegistry(mgr.logger)
	registry.Register("worktree", &directErrorPreparer{
		err: worktree.ClassifyGitError(
			"fatal: 'feature/direct' is already used by worktree at '/tmp/repo/.git/worktrees/other'",
			nil,
		),
	})
	mgr.preparerRegistry = registry

	result := mgr.runEnvironmentPreparerWithProgress(
		context.Background(),
		&LaunchRequest{
			TaskID:         "task-4",
			SessionID:      "session-4",
			ExecutorType:   "worktree",
			RepositoryPath: "/tmp/repo",
		},
		"",
		func(PrepareStep, int, int) {},
	)

	require.NotNil(t, result)
	require.False(t, result.Success)
	require.ErrorIs(t, result.Error, worktree.ErrBranchCheckedOut)
}

func TestWorktreePreparerMultiRepoFailureCarriesError(t *testing.T) {
	preparer := NewWorktreePreparer(nil, newTestLocalLogger())
	result, err := preparer.Prepare(context.Background(), &EnvPrepareRequest{
		Repositories: []RepoPrepareSpec{
			{RepositoryID: "repo-a", RepositoryPath: "/tmp/repo-a"},
			{RepositoryID: "repo-b", RepositoryPath: "/tmp/repo-b"},
		},
	}, nil)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.Success)
	require.Error(t, result.Error)
}

func TestLaunchRequiredPreparationFailureDoesNotCreateRuntime(t *testing.T) {
	profileResolver := &countingProfileResolver{info: &AgentProfileInfo{
		ProfileID: "profile-refresh-failure",
		AgentName: "auggie",
	}}
	mgr, backend := newEnvironmentExecutionTestManagerWithProfileResolver(t, nil, profileResolver)
	mgr.preparerRegistry = NewPreparerRegistry(mgr.logger)
	mgr.preparerRegistry.Register("worktree", &directErrorPreparer{err: &RepositoryPreparationError{
		RepositoryID:     "repo-back",
		TaskRepositoryID: "tr-back",
		RepositoryName:   "backend",
		Cause:            worktree.ErrGitCommandFailed,
	}})

	_, err := mgr.Launch(context.Background(), &LaunchRequest{
		TaskID:         "task-refresh-failure",
		SessionID:      "session-refresh-failure",
		AgentProfileID: "profile-refresh-failure",
		ExecutorType:   "worktree",
		RepositoryPath: "/tmp/repo",
		UseWorktree:    true,
		BaseBranch:     "main",
	})
	require.Error(t, err)
	require.ErrorIs(t, err, worktree.ErrGitCommandFailed)
	require.Equal(t, int32(0), backend.createCount.Load(), "runtime must not be created after preparation failure")
}
