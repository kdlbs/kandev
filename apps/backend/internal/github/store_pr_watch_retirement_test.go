package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func TestPRWatchRetirement_MigrationReplay(t *testing.T) {
	db := openLegacyGitHubDB(t)
	ctx := context.Background()
	retiredAt := time.Date(2026, 10, 26, 12, 30, 0, 0, time.UTC)
	if _, err := db.Exec(`
		ALTER TABLE github_pr_watches ADD COLUMN retired_at DATETIME;
		INSERT INTO workspaces (id) VALUES ('ws-retirement');
		INSERT INTO tasks (id, workspace_id) VALUES ('task-retirement', 'ws-retirement');
	`); err != nil {
		t.Fatalf("seed migration prerequisites: %v", err)
	}
	seedLegacyPRWatch(t, db, &PRWatch{
		ID: "watch-retired", TaskID: "task-retirement", WorkspaceID: "ws-retirement",
		SessionID: "session-retired", Owner: "acme", Repo: "widget", Branch: "finished",
		CreatedAt: retiredAt.Add(-time.Hour), UpdatedAt: retiredAt.Add(-time.Hour),
	})
	seedLegacyPRWatch(t, db, &PRWatch{
		ID: "watch-active", TaskID: "task-retirement", WorkspaceID: "ws-retirement",
		SessionID: "session-active", Owner: "acme", Repo: "widget", Branch: "active",
		CreatedAt: retiredAt.Add(-time.Hour), UpdatedAt: retiredAt.Add(-time.Hour),
	})
	seedLegacyPRWatch(t, db, &PRWatch{
		ID: "watch-collision-active", TaskID: "task-retirement", WorkspaceID: "ws-retirement",
		SessionID: "session-collision", Owner: "acme", Repo: "widget", Branch: "finished",
		CreatedAt: retiredAt, UpdatedAt: retiredAt,
	})
	seedLegacyPRWatch(t, db, &PRWatch{
		ID: "watch-retired-pr", TaskID: "task-retirement", WorkspaceID: "ws-retirement",
		RepositoryID: "repo-1", SessionID: "session-numbered", Owner: "acme", Repo: "widget",
		PRNumber: 42, Branch: "numbered", CreatedAt: retiredAt, UpdatedAt: retiredAt,
	})
	if _, err := db.Exec(`UPDATE github_pr_watches SET retired_at = ? WHERE id = ?`, retiredAt, "watch-retired"); err != nil {
		t.Fatalf("mark legacy watch retired: %v", err)
	}
	if _, err := db.Exec(`UPDATE github_pr_watches SET retired_at = ? WHERE id = ?`, retiredAt, "watch-retired-pr"); err != nil {
		t.Fatalf("mark legacy numbered watch retired: %v", err)
	}

	store, err := NewStore(db, db)
	require.NoError(t, err)
	assertRetirementInventory(t, store, ctx, retiredAt)

	reopened, err := NewStore(db, db)
	require.NoError(t, err)
	assertRetirementInventory(t, reopened, ctx, retiredAt)
}

func assertRetirementInventory(t *testing.T, store *Store, ctx context.Context, retiredAt time.Time) {
	t.Helper()
	all, err := store.ListPRWatchesByTaskIncludingRetired(ctx, "task-retirement")
	require.NoError(t, err)
	require.Len(t, all, 3)
	retired := findPRWatch(all, "watch-retired")
	require.NotNil(t, retired)
	require.NotNil(t, retired.RetiredAt)
	require.True(t, retired.RetiredAt.Equal(retiredAt))
	numbered, err := store.GetPRWatchByTaskRepoPRNumber(ctx, "task-retirement", "repo-1", 42)
	require.NoError(t, err)
	require.NotNil(t, numbered)
	require.NotNil(t, numbered.RetiredAt)
	require.True(t, numbered.RetiredAt.Equal(retiredAt))

	active, err := store.ListPRWatchesByTask(ctx, "task-retirement")
	require.NoError(t, err)
	require.Len(t, active, 1)
	require.Equal(t, "watch-active", active[0].ID)
	require.Nil(t, findPRWatch(all, "watch-collision-active"), "deduplication must preserve the retired canonical identity")

	byIdentity, err := store.GetPRWatchByTaskRepoBranchAny(ctx, "task-retirement", "", "finished")
	require.NoError(t, err)
	require.NotNil(t, byIdentity)
	require.Equal(t, "watch-retired", byIdentity.ID)
}

func TestPRWatchRetirement_RestartAndArchive(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	_, err := store.db.ExecContext(ctx, `INSERT INTO tasks (id, workspace_id) VALUES ('task-1', 'ws-1')`)
	require.NoError(t, err)
	watch := &PRWatch{
		WorkspaceID: "ws-1", SessionID: "session-1", TaskID: "task-1",
		Owner: "acme", Repo: "widget", Branch: "finished",
	}
	require.NoError(t, store.CreatePRWatch(ctx, watch))
	sibling := &PRWatch{
		WorkspaceID: "ws-1", SessionID: "session-sibling", TaskID: "task-1",
		Owner: "acme", Repo: "widget", Branch: "active",
	}
	require.NoError(t, store.CreatePRWatch(ctx, sibling))
	checkedAt := time.Date(2026, 10, 26, 11, 0, 0, 0, time.UTC)
	require.NoError(t, store.UpdatePRWatchTimestamps(ctx, watch.ID, checkedAt, &checkedAt, "success", "approved"))
	retiredAt := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)

	retired, err := store.RetirePRWatch(ctx, watch.ID, retiredAt)
	require.NoError(t, err)
	require.NotNil(t, retired.RetiredAt)
	require.True(t, retired.RetiredAt.Equal(retiredAt))
	retiredAgain, err := store.RetirePRWatch(ctx, watch.ID, retiredAt.Add(time.Hour))
	require.NoError(t, err)
	require.True(t, retiredAgain.RetiredAt.Equal(retiredAt), "repeated retirement must preserve its original timestamp")
	callbackCalled := false
	accepted, err := store.ApplyPRWatchResult(ctx, watch.ID, func(*sqlx.Tx, *PRWatch) error {
		callbackCalled = true
		return nil
	})
	require.NoError(t, err)
	require.False(t, accepted)
	require.False(t, callbackCalled, "retired rows must not admit stale provider results")

	active, err := store.ListActivePRWatches(ctx)
	require.NoError(t, err)
	require.Len(t, active, 1)
	require.Equal(t, sibling.ID, active[0].ID)

	if _, err := store.db.ExecContext(ctx, `UPDATE tasks SET archived_at = ? WHERE id = ?`, retiredAt, "task-1"); err != nil {
		t.Fatalf("archive task: %v", err)
	}
	_, err = store.DeleteActivePRWatchesByTaskID(ctx, watch.TaskID)
	require.NoError(t, err)
	archivedWatch, err := store.GetPRWatch(ctx, watch.ID)
	require.NoError(t, err)
	require.NotNil(t, archivedWatch, "archive cleanup must retain explicit retirement")
	deletedSibling, err := store.GetPRWatch(ctx, sibling.ID)
	require.NoError(t, err)
	require.Nil(t, deletedSibling, "archive cleanup must remove active watches")
	if _, err := store.db.ExecContext(ctx, `UPDATE tasks SET archived_at = NULL WHERE id = ?`, "task-1"); err != nil {
		t.Fatalf("unarchive task: %v", err)
	}
	activeAfterUnarchive, err := store.ListActivePRWatches(ctx)
	require.NoError(t, err)
	require.Empty(t, activeAfterUnarchive, "unarchive must not reactivate a retired watch")

	resumed, err := store.ResumePRWatch(ctx, watch.ID, retiredAt.Add(2*time.Hour))
	require.NoError(t, err)
	require.NotNil(t, resumed)
	require.NotEqual(t, watch.ID, resumed.ID)
	require.Nil(t, resumed.RetiredAt)
	require.Nil(t, resumed.LastCheckedAt)
	require.Nil(t, resumed.LastCommentAt)
	require.Equal(t, watch.TaskID, resumed.TaskID)
	require.Equal(t, watch.RepositoryID, resumed.RepositoryID)
	require.Equal(t, watch.Branch, resumed.Branch)

	oldWatch, err := store.GetPRWatch(ctx, watch.ID)
	require.NoError(t, err)
	require.Nil(t, oldWatch)
	activeAfterResume, err := store.ListActivePRWatches(ctx)
	require.NoError(t, err)
	require.Len(t, activeAfterResume, 1)
	require.Equal(t, resumed.ID, activeAfterResume[0].ID)

	applyErr := errors.New("reject result for rollback")
	accepted, err = store.ApplyPRWatchResult(ctx, resumed.ID, func(tx *sqlx.Tx, _ *PRWatch) error {
		_, updateErr := tx.ExecContext(ctx, `UPDATE github_pr_watches SET last_check_status = 'uncommitted' WHERE id = ?`, resumed.ID)
		if updateErr != nil {
			return updateErr
		}
		return applyErr
	})
	require.False(t, accepted)
	require.ErrorIs(t, err, applyErr)
	rolledBack, err := store.GetPRWatch(ctx, resumed.ID)
	require.NoError(t, err)
	require.NotEqual(t, "uncommitted", rolledBack.LastCheckStatus)

	_, err = store.DeleteTaskOwnedStateByTaskID(ctx, watch.TaskID)
	require.NoError(t, err)
	remaining, err := store.ListPRWatchesByTaskIncludingRetired(ctx, watch.TaskID)
	require.NoError(t, err)
	require.Empty(t, remaining, "permanent task cleanup must remove every watch lifetime")
}

// @covers AC-INTEGRATIONS-GITHUB-PR-WATCH-RETIREMENT-001.3
// @covers AC-INTEGRATIONS-GITHUB-PR-WATCH-RETIREMENT-001.4
func TestPRWatchRetirement_PreservesHistoryAndMixedTargets(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	for _, taskID := range []string{"task-retired", "task-shared"} {
		if _, err := store.db.ExecContext(ctx, `INSERT INTO tasks (id, workspace_id) VALUES (?, ?)`, taskID, "ws-1"); err != nil {
			t.Fatal(err)
		}
	}
	retired := &PRWatch{
		WorkspaceID: "ws-1", TaskID: "task-retired", RepositoryID: "repo-1", Owner: "acme", Repo: "widget",
		PRNumber: 42, Branch: "feature/finished",
	}
	shared := &PRWatch{
		WorkspaceID: "ws-1", TaskID: "task-shared", RepositoryID: "repo-1", Owner: "acme", Repo: "widget",
		PRNumber: 42, Branch: "feature/shared",
	}
	otherBranch := &PRWatch{
		WorkspaceID: "ws-1", TaskID: "task-retired", RepositoryID: "repo-1", Owner: "acme", Repo: "widget",
		PRNumber: 43, Branch: "feature/other",
	}
	otherRepository := &PRWatch{
		WorkspaceID: "ws-1", TaskID: "task-retired", RepositoryID: "repo-2", Owner: "acme", Repo: "other",
		PRNumber: 42, Branch: "feature/other-repo",
	}
	for _, watch := range []*PRWatch{retired, shared, otherBranch, otherRepository} {
		if err := store.CreatePRWatch(ctx, watch); err != nil {
			t.Fatal(err)
		}
	}
	history := &TaskPR{
		WorkspaceID: "ws-1", TaskID: retired.TaskID, RepositoryID: retired.RepositoryID,
		Owner: "acme", Repo: "widget", PRNumber: 42, PRURL: "https://github.com/acme/widget/pull/42",
		PRTitle: "Preserved history", State: "open", ReviewState: "approved", ChecksState: "success",
	}
	if err := store.CreateTaskPR(ctx, history); err != nil {
		t.Fatal(err)
	}
	autoMerge := true
	if _, err := store.UpdateTaskPRAutomationOptions(ctx, history.TaskID, history.RepositoryID, history.PRNumber,
		TaskPRAutomationOptionsPatch{AutoMergeEnabled: &autoMerge}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RetirePRWatch(ctx, retired.ID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}

	active, err := store.ListActivePRWatches(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 3 || findPRWatch(active, retired.ID) != nil ||
		findPRWatch(active, shared.ID) == nil || findPRWatch(active, otherBranch.ID) == nil ||
		findPRWatch(active, otherRepository.ID) == nil {
		t.Fatalf("active watches after retirement = %+v, want only active shared and unrelated targets", active)
	}
	rows, err := store.ListTaskPRsByTask(ctx, history.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].PRNumber != 42 || rows[0].ReviewState != "approved" || rows[0].ChecksState != "success" {
		t.Fatalf("task PR history after retirement = %+v", rows)
	}
	options, err := store.GetTaskPRAutomationOptions(ctx, history.TaskID, history.RepositoryID, history.PRNumber)
	if err != nil {
		t.Fatal(err)
	}
	if options == nil || !options.AutoMergeEnabled {
		t.Fatalf("automation options after retirement = %+v, want auto-merge preserved", options)
	}
}

func findPRWatch(watches []*PRWatch, id string) *PRWatch {
	for _, watch := range watches {
		if watch.ID == id {
			return watch
		}
	}
	return nil
}

// @covers AC-INTEGRATIONS-GITHUB-PR-WATCH-RETIREMENT-001.1
// @covers AC-INTEGRATIONS-GITHUB-PR-WATCH-RETIREMENT-001.2
func TestPRWatchRetirement_DeleteThenReconcile(t *testing.T) {
	poller, service, client, store := setupPollerTest(t)
	ctx := context.Background()
	seedTask(t, store, "task-retired", false)
	watch, err := service.CreatePRWatchForWorkspace(
		ctx, testWorkspaceID, "session-retired", "task-retired", "repo-1", "acme", "widget", 0, "finished",
	)
	require.NoError(t, err)
	require.NoError(t, service.DeletePRWatch(ctx, watch.ID))
	require.NoError(t, service.DeletePRWatch(ctx, watch.ID), "repeated deletion must be idempotent")

	provider := &mockTaskBranchProvider{tasks: []TaskBranchInfo{{
		WorkspaceID: testWorkspaceID, TaskID: watch.TaskID, SessionID: watch.SessionID,
		RepositoryID: watch.RepositoryID, Owner: watch.Owner, Repo: watch.Repo, Branch: watch.Branch,
	}}}
	poller.SetTaskBranchProvider(provider)
	for range 3 {
		poller.reconcileWatches(ctx)
	}

	active, err := store.ListActivePRWatches(ctx)
	require.NoError(t, err)
	require.Empty(t, active, "reconciliation must not recreate an explicitly retired target")
	all, err := store.ListPRWatchesByTaskIncludingRetired(ctx, watch.TaskID)
	require.NoError(t, err)
	require.Len(t, all, 1)
	require.Equal(t, watch.ID, all[0].ID)
	require.NotNil(t, all[0].RetiredAt)
	results, err := service.SyncWorkspaceWatchesBatched(ctx, testWorkspaceID, []*PRWatch{watch})
	require.NoError(t, err)
	require.Empty(t, results, "explicit sync must discard retired watch lifetimes before provider admission")
	_, _, err = service.CheckPRWatchForWorkspace(ctx, watch)
	require.ErrorIs(t, err, ErrPRWatchRetired)
	_, err = service.triggerPRDetectionWithOptions(ctx, watch, watch.TaskID, true)
	require.ErrorIs(t, err, ErrPRWatchRetired, "explicit PR refresh must stop before resolving credentials or calling GitHub")
	require.Zero(t, client.FindPRByBranchCallCount())
	_, err = service.EnsurePRWatchForWorkspace(
		ctx, testWorkspaceID, "session-new", watch.TaskID, watch.RepositoryID, watch.Owner, watch.Repo, watch.Branch,
	)
	require.ErrorIs(t, err, ErrPRWatchRetired)
}

// @covers AC-INTEGRATIONS-GITHUB-PR-WATCH-RETIREMENT-001.1
// @covers AC-INTEGRATIONS-GITHUB-PR-WATCH-RETIREMENT-001.2
func TestPRWatchRetirement_NumberedTerminalTargetsStayRetired(t *testing.T) {
	for _, state := range []string{prStateMerged, prStateClosed} {
		t.Run(state, func(t *testing.T) {
			poller, service, client, store := setupPollerTest(t)
			t.Cleanup(service.Stop)
			ctx := context.Background()
			seedTask(t, store, "task-terminal", false)
			client.AddPR(&PR{
				Number: 42, State: state, RepoOwner: "acme", RepoName: "widget",
				HeadBranch: "finished", HTMLURL: "https://github.com/acme/widget/pull/42",
			})
			watch, err := service.CreatePRWatchForWorkspace(
				ctx, testWorkspaceID, "session-terminal", "task-terminal", "repo-1", "acme", "widget", 42, "finished",
			)
			require.NoError(t, err)
			require.NoError(t, service.DeletePRWatch(ctx, watch.ID))
			provider := &mockTaskBranchProvider{tasks: []TaskBranchInfo{{
				WorkspaceID: testWorkspaceID, TaskID: watch.TaskID, SessionID: watch.SessionID,
				RepositoryID: watch.RepositoryID, Owner: watch.Owner, Repo: watch.Repo, Branch: watch.Branch,
			}}}
			poller.SetTaskBranchProvider(provider)
			poller.SetClock(func() time.Time { return time.Now().Add(2 * time.Minute) })
			for range 3 {
				poller.reconcileWatches(ctx)
			}
			poller.checkPRWatches(ctx)

			active, err := store.ListActivePRWatches(ctx)
			require.NoError(t, err)
			require.Empty(t, active, "a numbered terminal watch must reserve its branch")
			require.Zero(t, client.FindPRByBranchCallCount(), "retired branch discovery must not call GitHub")

			reopenedStore, err := NewStore(store.db, store.ro)
			require.NoError(t, err)
			restartedClient := NewMockClient()
			restartedClient.AddPR(&PR{
				Number: 42, State: state, RepoOwner: "acme", RepoName: "widget",
				HeadBranch: "finished", HTMLURL: "https://github.com/acme/widget/pull/42",
			})
			restartedService := newWorkspaceAuthenticatedTestService(t, restartedClient, reopenedStore, testWorkspaceID)
			t.Cleanup(restartedService.Stop)
			restartedPoller := NewPoller(restartedService, nil, testLogger(t))
			restartedPoller.SetTaskBranchProvider(provider)
			restartedPoller.SetClock(func() time.Time { return time.Now().Add(2 * time.Minute) })
			for range 3 {
				restartedPoller.reconcileWatches(ctx)
			}
			restartedPoller.checkPRWatches(ctx)

			active, err = reopenedStore.ListActivePRWatches(ctx)
			require.NoError(t, err)
			require.Empty(t, active, "restart must not recreate a numbered retired target")
			require.Zero(t, restartedClient.FindPRByBranchCallCount(), "restart must not probe the retired branch")
		})
	}
}

// @covers AC-INTEGRATIONS-GITHUB-PR-WATCH-RETIREMENT-001.2
func TestPRWatchRetirement_NumberedBranchBlocksExistingSearchingSibling(t *testing.T) {
	poller, service, client, store := setupPollerTest(t)
	t.Cleanup(service.Stop)
	ctx := context.Background()
	seedTask(t, store, "task-shadowed-branch", false)
	numbered, err := service.CreatePRWatchForWorkspace(
		ctx, testWorkspaceID, "session-numbered", "task-shadowed-branch", "repo-1", "acme", "widget", 42, "finished",
	)
	require.NoError(t, err)
	searching := &PRWatch{
		WorkspaceID: testWorkspaceID, SessionID: "session-searching", TaskID: numbered.TaskID,
		RepositoryID: numbered.RepositoryID, Owner: numbered.Owner, Repo: numbered.Repo, Branch: numbered.Branch,
	}
	require.NoError(t, store.CreatePRWatch(ctx, searching))
	require.NoError(t, service.DeletePRWatch(ctx, numbered.ID))

	poller.SetTaskBranchProvider(&mockTaskBranchProvider{tasks: []TaskBranchInfo{{
		WorkspaceID: testWorkspaceID, TaskID: searching.TaskID, SessionID: searching.SessionID,
		RepositoryID: searching.RepositoryID, Owner: searching.Owner, Repo: searching.Repo, Branch: searching.Branch,
	}}})
	poller.SetClock(func() time.Time { return time.Now().Add(2 * time.Minute) })
	poller.checkPRWatches(ctx)
	require.Zero(t, client.FindPRByBranchCallCount(), "an existing sibling cannot probe a retired branch")

	accepted, err := service.ApplyPRWatchDiscoveryResult(ctx, searching, &PR{
		Number: 43, Title: "late branch result", State: "open", RepoOwner: "acme", RepoName: "widget",
	})
	require.NoError(t, err)
	require.False(t, accepted, "a result for a sibling lifetime cannot enter a retired branch")
	rows, err := store.ListTaskPRsByTask(ctx, searching.TaskID)
	require.NoError(t, err)
	require.Empty(t, rows)
}

// @covers AC-INTEGRATIONS-GITHUB-PR-WATCH-RETIREMENT-001.7
func TestPRWatchRetirement_InFlightResults(t *testing.T) {
	_, service, _, store := setupPollerTest(t)
	ctx := context.Background()
	seedTask(t, store, "task-retired", false)
	watch, err := service.CreatePRWatchForWorkspace(
		ctx, testWorkspaceID, "session-retired", "task-retired", "", "acme", "widget", 42, "feature",
	)
	require.NoError(t, err)
	var feedback atomic.Int32
	subscription, err := service.eventBus.Subscribe(events.GitHubPRFeedback, func(context.Context, *bus.Event) error {
		feedback.Add(1)
		return nil
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = subscription.Unsubscribe() })
	require.NoError(t, service.DeletePRWatch(ctx, watch.ID))

	requiredReviews := 1
	status := &PRStatus{
		PR: &PR{
			Number: 42, Title: "late result", State: "open", RepoOwner: "acme", RepoName: "widget",
			HTMLURL: "https://github.com/acme/widget/pull/42", HeadBranch: "feature", BaseBranch: "main",
			UpdatedAt: time.Now().UTC(),
		},
		ChecksState: "success", ReviewState: "approved", RequiredReviews: &requiredReviews,
	}
	accepted, changed, err := service.applyPRWatchStatusResult(ctx, watch, status, time.Now().UTC(), true)
	require.NoError(t, err)
	require.False(t, accepted)
	require.False(t, changed)
	rows, err := store.ListTaskPRsByTask(ctx, watch.TaskID)
	require.NoError(t, err)
	require.Empty(t, rows, "a stale status result must not create a task PR")
	retired, err := store.GetPRWatch(ctx, watch.ID)
	require.NoError(t, err)
	require.NotNil(t, retired)
	require.Nil(t, retired.LastCheckedAt, "stale result must not update watch timestamps")
	require.Zero(t, feedback.Load(), "stale result must not publish feedback")

	pushWatch, err := service.CreatePRWatchForWorkspace(
		ctx, testWorkspaceID, "session-push", "task-push", "", "acme", "widget", 44, "feature/push",
	)
	require.NoError(t, err)
	pushPR := &PR{
		Number: 44, Title: "push result", State: "open", RepoOwner: "acme", RepoName: "widget",
		HTMLURL: "https://github.com/acme/widget/pull/44", HeadBranch: "feature/push", BaseBranch: "main",
	}
	applied, err := service.ApplyPRWatchDiscoveryResult(ctx, pushWatch, pushPR)
	require.NoError(t, err)
	require.True(t, applied, "an active push result must associate through the lifetime fence")
	pushRows, err := store.ListTaskPRsByTask(ctx, pushWatch.TaskID)
	require.NoError(t, err)
	require.Len(t, pushRows, 1)

	searching, err := service.CreatePRWatchForWorkspace(
		ctx, testWorkspaceID, "session-search", "task-retired", "", "acme", "widget", 0, "another-branch",
	)
	require.NoError(t, err)
	require.NoError(t, service.DeletePRWatch(ctx, searching.ID))
	found := &PR{Number: 43, Title: "late discovery", State: "open", RepoOwner: "acme", RepoName: "widget"}
	accepted, promoted, err := service.applyPRWatchDiscoveryResult(ctx, searching, found, time.Now().UTC())
	require.NoError(t, err)
	require.False(t, accepted)
	require.False(t, promoted)
	rows, err = store.ListTaskPRsByTask(ctx, searching.TaskID)
	require.NoError(t, err)
	require.Empty(t, rows, "a stale discovery result must not create a task PR")

	resumed, err := store.ResumePRWatch(ctx, watch.ID, time.Now().UTC())
	require.NoError(t, err)
	require.NotNil(t, resumed)
	require.NotEqual(t, watch.ID, resumed.ID)
	accepted, changed, err = service.applyPRWatchStatusResult(ctx, watch, status, time.Now().UTC(), true)
	require.NoError(t, err)
	require.False(t, accepted, "resumption must not admit results tied to the retired watch ID")
	require.False(t, changed)
	require.Zero(t, feedback.Load())
	rows, err = store.ListTaskPRsByTask(ctx, watch.TaskID)
	require.NoError(t, err)
	require.Empty(t, rows)
}

// @covers AC-INTEGRATIONS-GITHUB-PR-WATCH-RETIREMENT-001.5
func TestPRWatchRetirement_HTTPResumeAndScope(t *testing.T) {
	service, store := setupWatchServiceTest(t)
	storeTask := &taskmodels.Task{ID: "task-resume", WorkspaceID: "ws-1"}
	service.SetTaskIssueStore(&fakeTaskIssueStore{
		task:  storeTask,
		repos: []*taskmodels.TaskRepository{{RepositoryID: "repo-1"}},
	})
	if _, err := store.db.Exec(`INSERT INTO tasks (id, workspace_id) VALUES (?, ?)`, storeTask.ID, storeTask.WorkspaceID); err != nil {
		t.Fatal(err)
	}
	watch := &PRWatch{
		WorkspaceID: "ws-1", TaskID: storeTask.ID, RepositoryID: "repo-1", Owner: "acme", Repo: "widget",
		Branch: "feature", PRNumber: 42,
	}
	require.NoError(t, store.CreatePRWatch(context.Background(), watch))
	router := gin.New()
	useControllerTestWorkspace(router)
	NewController(service, newControllerTestLogger()).RegisterHTTPRoutes(router)
	client := controllerTestClient{router: router}

	before := client.do(http.MethodGet, "/api/v1/github/watches/pr?workspace_id=ws-1")
	require.Equal(t, http.StatusOK, before.Code)
	require.Contains(t, before.Body.String(), watch.ID)

	deleted := client.do(http.MethodDelete, "/api/v1/github/watches/pr/"+watch.ID+"?workspace_id=ws-1")
	require.Equal(t, http.StatusOK, deleted.Code, deleted.Body.String())
	deletedAgain := client.do(http.MethodDelete, "/api/v1/github/watches/pr/"+watch.ID+"?workspace_id=ws-1")
	require.Equal(t, http.StatusOK, deletedAgain.Code, deletedAgain.Body.String())

	active := client.do(http.MethodGet, "/api/v1/github/watches/pr?workspace_id=ws-1")
	require.Equal(t, http.StatusOK, active.Code)
	require.NotContains(t, active.Body.String(), watch.ID)
	inclusive := client.do(http.MethodGet, "/api/v1/github/watches/pr?workspace_id=ws-1&include_retired=true")
	require.Equal(t, http.StatusOK, inclusive.Code)
	require.Contains(t, inclusive.Body.String(), watch.ID)
	require.Contains(t, inclusive.Body.String(), "retired_at")

	foreignResume := client.do(http.MethodPost, "/api/v1/github/watches/pr/"+watch.ID+"/resume?workspace_id=other", nil)
	require.Equal(t, http.StatusNotFound, foreignResume.Code)
	archivedAt := time.Now().UTC()
	for name, taskStore := range map[string]*fakeTaskIssueStore{
		"archived": {task: &taskmodels.Task{ID: storeTask.ID, WorkspaceID: "ws-1", ArchivedAt: &archivedAt}},
		"missing":  {taskErr: fmt.Errorf("task lookup failed: %w", ErrTaskNotFound)},
		"detached": {task: storeTask, repos: []*taskmodels.TaskRepository{{RepositoryID: "another-repo"}}},
	} {
		t.Run(name, func(t *testing.T) {
			service.SetTaskIssueStore(taskStore)
			response := client.do(http.MethodPost, "/api/v1/github/watches/pr/"+watch.ID+"/resume?workspace_id=ws-1", nil)
			require.Equal(t, http.StatusConflict, response.Code, response.Body.String())
			current, err := store.GetPRWatch(context.Background(), watch.ID)
			require.NoError(t, err)
			require.NotNil(t, current, "ineligible resume must not change stored retirement")
			require.NotNil(t, current.RetiredAt)
		})
	}
	service.SetTaskIssueStore(&fakeTaskIssueStore{
		task:  storeTask,
		repos: []*taskmodels.TaskRepository{{RepositoryID: "repo-1"}},
	})
	resumedResponse := client.do(http.MethodPost, "/api/v1/github/watches/pr/"+watch.ID+"/resume?workspace_id=ws-1", nil)
	require.Equal(t, http.StatusOK, resumedResponse.Code, resumedResponse.Body.String())
	var response struct {
		Watch PRWatch `json:"watch"`
	}
	require.NoError(t, json.Unmarshal(resumedResponse.Body.Bytes(), &response))
	require.NotEmpty(t, response.Watch.ID)
	require.NotEqual(t, watch.ID, response.Watch.ID)
	require.Nil(t, response.Watch.RetiredAt)
	active = client.do(http.MethodGet, "/api/v1/github/watches/pr?workspace_id=ws-1")
	require.Contains(t, active.Body.String(), response.Watch.ID)
	require.NotContains(t, active.Body.String(), watch.ID)
}

func TestPRWatchRetirement_HTTPResumeLegacyEmptyRepositoryID(t *testing.T) {
	service, store := setupWatchServiceTest(t)
	storeTask := &taskmodels.Task{ID: "task-resume-legacy", WorkspaceID: "ws-1"}
	service.SetTaskIssueStore(&fakeTaskIssueStore{task: storeTask})
	if _, err := store.db.Exec("INSERT INTO tasks (id, workspace_id) VALUES (?, ?)", storeTask.ID, storeTask.WorkspaceID); err != nil {
		t.Fatal(err)
	}
	watch := &PRWatch{
		WorkspaceID: "ws-1", TaskID: storeTask.ID, Owner: "acme", Repo: "widget",
		Branch: "legacy-feature", PRNumber: 42,
	}
	require.NoError(t, store.CreatePRWatch(context.Background(), watch))

	router := gin.New()
	useControllerTestWorkspace(router)
	NewController(service, newControllerTestLogger()).RegisterHTTPRoutes(router)
	client := controllerTestClient{router: router}
	deleted := client.do(http.MethodDelete, "/api/v1/github/watches/pr/"+watch.ID+"?workspace_id=ws-1")
	require.Equal(t, http.StatusOK, deleted.Code, deleted.Body.String())

	resumed := client.do(http.MethodPost, "/api/v1/github/watches/pr/"+watch.ID+"/resume?workspace_id=ws-1", nil)
	require.Equal(t, http.StatusOK, resumed.Code, resumed.Body.String())
	var response struct {
		Watch PRWatch `json:"watch"`
	}
	require.NoError(t, json.Unmarshal(resumed.Body.Bytes(), &response))
	require.NotEmpty(t, response.Watch.ID)
	require.NotEqual(t, watch.ID, response.Watch.ID)
	require.Empty(t, response.Watch.RepositoryID)
	require.Nil(t, response.Watch.RetiredAt)
}

// @covers AC-INTEGRATIONS-GITHUB-PR-WATCH-RETIREMENT-001.1
func TestPRWatchRetirement_WSDelete(t *testing.T) {
	service, store := setupWatchServiceTest(t)
	watch := &PRWatch{WorkspaceID: "ws-1", TaskID: "task-ws", Owner: "acme", Repo: "widget", Branch: "feature"}
	require.NoError(t, store.CreatePRWatch(context.Background(), watch))
	handler := wsDeletePRWatch(service, newControllerTestLogger())
	ctx := context.Background()
	for range 2 {
		request, err := ws.NewRequest("request", "github.watch.pr.delete", map[string]string{"id": watch.ID})
		require.NoError(t, err)
		response, err := handler(ctx, request)
		require.NoError(t, err)
		require.Equal(t, ws.MessageTypeResponse, response.Type)
	}
	retired, err := store.GetPRWatch(context.Background(), watch.ID)
	require.NoError(t, err)
	require.NotNil(t, retired)
	require.NotNil(t, retired.RetiredAt)
}

type controllerTestClient struct{ router http.Handler }

func (c controllerTestClient) do(method, target string, _ ...any) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, target, nil)
	response := httptest.NewRecorder()
	c.router.ServeHTTP(response, request)
	return response
}
