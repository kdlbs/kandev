package github

import (
	"context"
	"testing"
	"time"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type discoveryComparisonTargetObserver struct {
	store          *Store
	calls          int
	syncCalls      int
	taskID         string
	candidate      taskmodels.ComparisonTargetCandidate
	persistedTitle string
}

func (o *discoveryComparisonTargetObserver) ReconcileComparisonTarget(
	ctx context.Context, taskID string, candidate taskmodels.ComparisonTargetCandidate,
) (*taskmodels.ComparisonTargetReconciliation, error) {
	persisted, err := o.store.GetTaskPRByRepoAndNumber(ctx, taskID, "repo-1", candidate.Number)
	if err != nil {
		return nil, err
	}
	o.calls++
	o.taskID = taskID
	o.candidate = candidate
	if persisted != nil {
		o.persistedTitle = persisted.PRTitle
	}
	return &taskmodels.ComparisonTargetReconciliation{Status: taskmodels.ComparisonTargetNoMatch}, nil
}

func (o *discoveryComparisonTargetObserver) ReconcileComparisonTargetFromSync(
	context.Context, string, taskmodels.ComparisonTargetCandidate,
) (*taskmodels.ComparisonTargetReconciliation, error) {
	o.syncCalls++
	return &taskmodels.ComparisonTargetReconciliation{Status: taskmodels.ComparisonTargetNoMatch}, nil
}

func (*discoveryComparisonTargetObserver) RemoveComparisonTargetForChange(
	context.Context, string, string, string, string, int,
) error {
	return nil
}

func TestCreatePRWatch_DoesNotReuseDifferentPROnSameBranch(t *testing.T) {
	_, service, _, store := setupPollerTest(t)
	ctx := context.Background()
	seedTask(t, store, "task-multiple-prs", false)

	first, err := service.CreatePRWatchForWorkspace(
		ctx, testWorkspaceID, "session-multiple-prs", "task-multiple-prs", "repo-1", "acme", "widget", 42, "feature/fork",
	)
	require.NoError(t, err)

	retry, err := service.CreatePRWatchForWorkspace(
		ctx, testWorkspaceID, "session-multiple-prs", "task-multiple-prs", "repo-1", "acme", "widget", 42, "feature/fork",
	)
	require.NoError(t, err)
	require.Equal(t, first.ID, retry.ID, "the same numbered PR should reuse its watch")

	second, err := service.CreatePRWatchForWorkspace(
		ctx, testWorkspaceID, "session-multiple-prs", "task-multiple-prs", "repo-1", "acme", "widget", 43, "feature/fork",
	)
	require.NoError(t, err)
	require.NotEqual(t, first.ID, second.ID, "a different PR on the same head branch needs its own watch")
	require.Equal(t, 43, second.PRNumber)

	pr := &PR{
		Number: 43, Title: "Different base", State: "open", RepoOwner: "acme", RepoName: "widget",
		HTMLURL: "https://github.com/acme/widget/pull/43", HeadBranch: "feature/fork", BaseBranch: "release",
		HeadSHA: "pr-43-head", AuthorLogin: "author",
	}
	applied, err := service.ApplyPRWatchDiscoveryResult(ctx, second, pr)
	require.NoError(t, err)
	require.True(t, applied, "push discovery must apply PR 43 to its own watch")

	firstByPR, err := store.GetPRWatchByTaskRepoPRNumber(ctx, "task-multiple-prs", "repo-1", 42)
	require.NoError(t, err)
	secondByPR, err := store.GetPRWatchByTaskRepoPRNumber(ctx, "task-multiple-prs", "repo-1", 43)
	require.NoError(t, err)
	require.Equal(t, first.ID, firstByPR.ID)
	require.Equal(t, second.ID, secondByPR.ID)
	pr43, err := store.GetTaskPRByRepoAndNumber(ctx, "task-multiple-prs", "repo-1", 43)
	require.NoError(t, err)
	require.NotNil(t, pr43)
	require.Equal(t, "Different base", pr43.PRTitle)
}

func TestTerminalPRWatchResultPreservesSearchingSibling(t *testing.T) {
	_, service, _, store := setupPollerTest(t)
	ctx := context.Background()
	seedTask(t, store, "task-terminal-sibling", false)

	numbered, err := service.CreatePRWatchForWorkspace(
		ctx, testWorkspaceID, "session-terminal-sibling", "task-terminal-sibling", "repo-1", "acme", "widget", 42, "feature/terminal",
	)
	require.NoError(t, err)
	searching, err := service.CreatePRWatchForWorkspace(
		ctx, testWorkspaceID, "session-terminal-sibling", "task-terminal-sibling", "repo-1", "acme", "widget", 0, "feature/terminal",
	)
	require.NoError(t, err)
	require.NotEqual(t, numbered.ID, searching.ID)

	status := resultEventStatus(numbered, "Merged PR", time.Now().UTC())
	status.PR.State = prStateMerged
	accepted, _, err := service.applyPRWatchStatusResult(ctx, numbered, status, time.Now().UTC(), false)
	require.NoError(t, err)
	require.True(t, accepted)

	stored, err := store.GetTaskPRByRepoAndNumber(ctx, "task-terminal-sibling", "repo-1", 42)
	require.NoError(t, err)
	require.NotNil(t, stored)
	require.Equal(t, prStateMerged, stored.State)

	watches, err := store.ListPRWatchesByTask(ctx, "task-terminal-sibling")
	require.NoError(t, err)
	require.Len(t, watches, 1)
	require.Equal(t, searching.ID, watches[0].ID)
	require.Zero(t, watches[0].PRNumber)
}

func TestDiscoveredPRReconcilesComparisonTargetAfterCommit(t *testing.T) {
	_, service, _, store := setupPollerTest(t)
	ctx := context.Background()
	seedTask(t, store, "task-discovery-comparison", false)
	watch, err := service.CreatePRWatchForWorkspace(
		ctx, testWorkspaceID, "session-discovery-comparison", "task-discovery-comparison", "repo-1", "upstream", "repo", 0, "feature/replacement",
	)
	require.NoError(t, err)

	observer := &discoveryComparisonTargetObserver{store: store}
	service.SetComparisonTargetObserver(observer)
	pr := &PR{
		Number: 43, Title: "Replacement PR", State: "open", RepoOwner: "upstream", RepoName: "repo",
		HTMLURL: "https://github.com/upstream/repo/pull/43", HeadRepoOwner: "contributor", HeadRepoName: "fork",
		BaseRepoOwner: "upstream", BaseRepoName: "repo", HeadBranch: "feature/replacement", BaseBranch: "release",
		HeadSHA: "replacement-head", AuthorLogin: "author", CreatedAt: time.Now().Add(-time.Hour), UpdatedAt: time.Now(),
	}
	accepted, promoted, err := service.applyPRWatchDiscoveryResult(ctx, watch, pr, time.Now().UTC())
	require.NoError(t, err)
	require.True(t, accepted)
	require.True(t, promoted)
	require.Equal(t, 1, observer.calls, "discovery must reconcile the comparison target")
	require.Zero(t, observer.syncCalls, "discovery must permit replacing the previous target")
	require.Equal(t, "task-discovery-comparison", observer.taskID)
	require.Equal(t, 43, observer.candidate.Number)
	require.Equal(t, "contributor/fork", observer.candidate.HeadRepository.Path)
	require.Equal(t, "upstream/repo", observer.candidate.TargetRepository.Path)
	require.Equal(t, "release", observer.candidate.TargetBranch)
	require.Equal(t, "Replacement PR", observer.persistedTitle, "the TaskPR must be committed before reconciliation")
}
