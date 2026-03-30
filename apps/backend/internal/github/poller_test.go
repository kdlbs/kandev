package github

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
)

// setupPollerTest creates a Poller backed by an in-memory SQLite store and MockClient.
func setupPollerTest(t *testing.T) (*Poller, *Service, *MockClient, *Store) {
	t.Helper()

	rawDB, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	rawDB.SetMaxOpenConns(1)
	rawDB.SetMaxIdleConns(1)
	sqlxDB := sqlx.NewDb(rawDB, "sqlite3")
	t.Cleanup(func() { _ = sqlxDB.Close() })

	store, err := NewStore(sqlxDB, sqlxDB)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	mockClient := NewMockClient()
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	eventBus := bus.NewMemoryEventBus(log)

	svc := NewService(mockClient, "pat", nil, store, eventBus, log)
	poller := NewPoller(svc, eventBus, log)

	return poller, svc, mockClient, store
}

func TestCheckSinglePRWatch_MergedPR_SyncsThenDeletes(t *testing.T) {
	poller, _, mockClient, store := setupPollerTest(t)
	ctx := context.Background()

	// Set up a merged PR in the mock client.
	now := time.Now().UTC()
	mergedAt := now.Add(-1 * time.Hour)
	mockClient.AddPR(&PR{
		Number:     42,
		Title:      "Feature PR",
		State:      prStateMerged,
		HeadSHA:    "abc123",
		HeadBranch: "feature-branch",
		RepoOwner:  "owner",
		RepoName:   "repo",
		MergedAt:   &mergedAt,
	})

	// Create a PRWatch in the DB.
	watch := &PRWatch{
		SessionID: "sess-1",
		TaskID:    "task-1",
		Owner:     "owner",
		Repo:      "repo",
		PRNumber:  42,
		Branch:    "feature-branch",
	}
	if err := store.CreatePRWatch(ctx, watch); err != nil {
		t.Fatalf("create PR watch: %v", err)
	}

	// Create a TaskPR record so SyncTaskPR has something to update.
	taskPR := &TaskPR{
		TaskID:     "task-1",
		Owner:      "owner",
		Repo:       "repo",
		PRNumber:   42,
		PRURL:      "https://github.com/owner/repo/pull/42",
		PRTitle:    "Feature PR",
		HeadBranch: "feature-branch",
		BaseBranch: "main",
		State:      "open",
	}
	if err := store.CreateTaskPR(ctx, taskPR); err != nil {
		t.Fatalf("create task PR: %v", err)
	}

	// Act
	poller.checkSinglePRWatch(ctx, watch)

	// Assert: TaskPR record should be updated with state="merged".
	updatedTP, err := store.GetTaskPR(ctx, "task-1")
	if err != nil {
		t.Fatalf("get task PR: %v", err)
	}
	if updatedTP == nil {
		t.Fatal("expected task PR to exist after sync")
	}
	if updatedTP.State != prStateMerged {
		t.Errorf("expected task PR state=%q, got %q", prStateMerged, updatedTP.State)
	}
	if updatedTP.MergedAt == nil {
		t.Error("expected task PR MergedAt to be set")
	}

	// Assert: PRWatch should be deleted.
	remainingWatch, err := store.GetPRWatchBySession(ctx, "sess-1")
	if err != nil {
		t.Fatalf("get PR watch: %v", err)
	}
	if remainingWatch != nil {
		t.Error("expected PR watch to be deleted after merged PR")
	}
}

func TestCheckSinglePRWatch_OpenPR_SyncsOnChange(t *testing.T) {
	poller, _, mockClient, store := setupPollerTest(t)
	ctx := context.Background()

	// Set up an open PR with a passing check.
	mockClient.AddPR(&PR{
		Number:     10,
		Title:      "Open PR",
		State:      "open",
		HeadSHA:    "def456",
		HeadBranch: "open-branch",
		RepoOwner:  "owner",
		RepoName:   "repo",
		Additions:  5,
		Deletions:  3,
	})
	mockClient.AddCheckRuns("owner", "repo", "def456", []CheckRun{
		{Name: "ci", Status: "completed", Conclusion: "success"},
	})

	// Create a PRWatch with a different last_check_status to trigger hasNew.
	watch := &PRWatch{
		SessionID:       "sess-2",
		TaskID:          "task-2",
		Owner:           "owner",
		Repo:            "repo",
		PRNumber:        10,
		Branch:          "open-branch",
		LastCheckStatus: "pending", // different from "success" -> hasNew=true
	}
	if err := store.CreatePRWatch(ctx, watch); err != nil {
		t.Fatalf("create PR watch: %v", err)
	}

	// Create a TaskPR record.
	taskPR := &TaskPR{
		TaskID:     "task-2",
		Owner:      "owner",
		Repo:       "repo",
		PRNumber:   10,
		PRURL:      "https://github.com/owner/repo/pull/10",
		PRTitle:    "Open PR",
		HeadBranch: "open-branch",
		BaseBranch: "main",
		State:      "open",
	}
	if err := store.CreateTaskPR(ctx, taskPR); err != nil {
		t.Fatalf("create task PR: %v", err)
	}

	// Act
	poller.checkSinglePRWatch(ctx, watch)

	// Assert: TaskPR should be synced with latest data.
	updatedTP, err := store.GetTaskPR(ctx, "task-2")
	if err != nil {
		t.Fatalf("get task PR: %v", err)
	}
	if updatedTP == nil {
		t.Fatal("expected task PR to exist")
	}
	if updatedTP.State != "open" {
		t.Errorf("expected state=open, got %q", updatedTP.State)
	}
	if updatedTP.ChecksState != "success" {
		t.Errorf("expected checks_state=success, got %q", updatedTP.ChecksState)
	}
	if updatedTP.Additions != 5 {
		t.Errorf("expected additions=5, got %d", updatedTP.Additions)
	}

	// Assert: PRWatch should NOT be deleted (PR is still open).
	remainingWatch, err := store.GetPRWatchBySession(ctx, "sess-2")
	if err != nil {
		t.Fatalf("get PR watch: %v", err)
	}
	if remainingWatch == nil {
		t.Error("expected PR watch to still exist for open PR")
	}
}

// mockTaskBranchProvider implements TaskBranchProvider for testing.
type mockTaskBranchProvider struct {
	tasks []TaskBranchInfo
	err   error
}

func (m *mockTaskBranchProvider) ListTasksNeedingPRWatch(_ context.Context) ([]TaskBranchInfo, error) {
	return m.tasks, m.err
}

func TestReconcileWatches_CreatesWatchesForTasks(t *testing.T) {
	poller, _, _, store := setupPollerTest(t)
	ctx := context.Background()

	prov := &mockTaskBranchProvider{
		tasks: []TaskBranchInfo{
			{TaskID: "t1", SessionID: "s1", Owner: "myorg", Repo: "myrepo", Branch: "feature-a"},
			{TaskID: "t2", SessionID: "s2", Owner: "myorg", Repo: "myrepo", Branch: "feature-b"},
		},
	}
	poller.SetTaskBranchProvider(prov)

	poller.reconcileWatches(ctx)

	// Verify watches were created for both sessions.
	w1, err := store.GetPRWatchBySession(ctx, "s1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if w1 == nil {
		t.Fatal("expected PR watch for session s1")
	}
	if w1.Branch != "feature-a" {
		t.Errorf("expected branch %q, got %q", "feature-a", w1.Branch)
	}
	if w1.TaskID != "t1" {
		t.Errorf("expected task_id %q, got %q", "t1", w1.TaskID)
	}

	w2, err := store.GetPRWatchBySession(ctx, "s2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if w2 == nil {
		t.Fatal("expected PR watch for session s2")
	}
	if w2.Branch != "feature-b" {
		t.Errorf("expected branch %q, got %q", "feature-b", w2.Branch)
	}
}

func TestReconcileWatches_NilProvider(t *testing.T) {
	poller, _, _, _ := setupPollerTest(t)
	ctx := context.Background()

	// Should not panic when provider is nil.
	poller.reconcileWatches(ctx)
}

func TestReconcileWatches_SkipsExistingWatches(t *testing.T) {
	poller, _, _, store := setupPollerTest(t)
	ctx := context.Background()

	// Pre-create a watch for s1.
	existing := &PRWatch{
		SessionID: "s1",
		TaskID:    "t1",
		Owner:     "myorg",
		Repo:      "myrepo",
		PRNumber:  0,
		Branch:    "feature-a",
	}
	if err := store.CreatePRWatch(ctx, existing); err != nil {
		t.Fatalf("create PR watch: %v", err)
	}

	prov := &mockTaskBranchProvider{
		tasks: []TaskBranchInfo{
			{TaskID: "t1", SessionID: "s1", Owner: "myorg", Repo: "myrepo", Branch: "feature-a"},
			{TaskID: "t2", SessionID: "s2", Owner: "myorg", Repo: "myrepo", Branch: "feature-b"},
		},
	}
	poller.SetTaskBranchProvider(prov)

	poller.reconcileWatches(ctx)

	// s1 should still have its original watch (EnsurePRWatch is idempotent).
	w1, _ := store.GetPRWatchBySession(ctx, "s1")
	if w1 == nil {
		t.Fatal("expected PR watch for session s1")
	}
	if w1.ID != existing.ID {
		t.Errorf("expected original watch ID %q, got %q", existing.ID, w1.ID)
	}

	// s2 should have a new watch.
	w2, _ := store.GetPRWatchBySession(ctx, "s2")
	if w2 == nil {
		t.Fatal("expected PR watch for session s2")
	}
}

func TestCheckSinglePRWatch_OpenPR_NoChange_NoSync(t *testing.T) {
	poller, _, mockClient, store := setupPollerTest(t)
	ctx := context.Background()

	// Set up an open PR with a passing check.
	mockClient.AddPR(&PR{
		Number:     20,
		Title:      "Stable PR",
		State:      "open",
		HeadSHA:    "ghi789",
		HeadBranch: "stable-branch",
		RepoOwner:  "owner",
		RepoName:   "repo",
	})
	mockClient.AddCheckRuns("owner", "repo", "ghi789", []CheckRun{
		{Name: "ci", Status: "completed", Conclusion: "success"},
	})

	// Create a PRWatch with matching last_check_status -> no change.
	watch := &PRWatch{
		SessionID:       "sess-3",
		TaskID:          "task-3",
		Owner:           "owner",
		Repo:            "repo",
		PRNumber:        20,
		Branch:          "stable-branch",
		LastCheckStatus: "success", // same -> hasNew=false
	}
	if err := store.CreatePRWatch(ctx, watch); err != nil {
		t.Fatalf("create PR watch: %v", err)
	}

	// Create a TaskPR record.
	taskPR := &TaskPR{
		TaskID:     "task-3",
		Owner:      "owner",
		Repo:       "repo",
		PRNumber:   20,
		PRURL:      "https://github.com/owner/repo/pull/20",
		PRTitle:    "Stable PR",
		HeadBranch: "stable-branch",
		BaseBranch: "main",
		State:      "open",
	}
	if err := store.CreateTaskPR(ctx, taskPR); err != nil {
		t.Fatalf("create task PR: %v", err)
	}

	// Act
	poller.checkSinglePRWatch(ctx, watch)

	// Assert: PRWatch should NOT be deleted.
	remainingWatch, err := store.GetPRWatchBySession(ctx, "sess-3")
	if err != nil {
		t.Fatalf("get PR watch: %v", err)
	}
	if remainingWatch == nil {
		t.Error("expected PR watch to still exist")
	}
}
