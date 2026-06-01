package github

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
)

// setupPollerTest creates a Poller backed by an in-memory SQLite store and MockClient.
// The tasks table is created so the JOIN in ListActivePRWatches works; tests that
// want a watch to appear in that listing must seed a matching task row via seedTask.
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

	// Minimal tasks schema: ListActivePRWatches only joins on id and filters by archived_at.
	if _, err := sqlxDB.Exec(`
		CREATE TABLE tasks (
			id TEXT PRIMARY KEY,
			archived_at DATETIME
		)`); err != nil {
		t.Fatalf("create tasks table: %v", err)
	}

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

// seedTask inserts a minimal task row for JOIN-based queries. Pass archived=true
// to seed an archived task (archived_at set to now).
func seedTask(t *testing.T, store *Store, taskID string, archived bool) {
	t.Helper()
	var archivedAt interface{}
	if archived {
		archivedAt = time.Now().UTC()
	}
	if _, err := store.db.Exec(`INSERT INTO tasks (id, archived_at) VALUES (?, ?)`, taskID, archivedAt); err != nil {
		t.Fatalf("seed task %s: %v", taskID, err)
	}
}

func TestCheckSinglePRWatch_MergedPR_SyncsThenResets(t *testing.T) {
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

	// Assert: PRWatch should be reset to pr_number=0 (still present so the
	// poller can discover a follow-up PR on the same branch).
	remainingWatch, err := store.GetPRWatchBySession(ctx, "sess-1")
	if err != nil {
		t.Fatalf("get PR watch: %v", err)
	}
	if remainingWatch == nil {
		t.Fatal("expected PR watch to remain after merged PR (reset, not deleted)")
	}
	if remainingWatch.PRNumber != 0 {
		t.Errorf("expected PR watch pr_number=0 after merge, got %d", remainingWatch.PRNumber)
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
	tasks    []TaskBranchInfo
	err      error
	branches map[string]string // sessionID -> branch
}

func (m *mockTaskBranchProvider) ListTasksNeedingPRWatch(_ context.Context) ([]TaskBranchInfo, error) {
	return m.tasks, m.err
}

func (m *mockTaskBranchProvider) ResolveBranchForSession(_ context.Context, _, sessionID string) string {
	if m.branches != nil {
		return m.branches[sessionID]
	}
	return ""
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

func TestRefreshStaleBranches_UpdatesBranchWhenChanged(t *testing.T) {
	poller, _, _, store := setupPollerTest(t)
	ctx := context.Background()

	// Create a watch with pr_number=0 on old branch.
	seedTask(t, store, "t1", false)
	watch := &PRWatch{
		SessionID: "s1",
		TaskID:    "t1",
		Owner:     "myorg",
		Repo:      "myrepo",
		PRNumber:  0,
		Branch:    "old-branch",
	}
	if err := store.CreatePRWatch(ctx, watch); err != nil {
		t.Fatalf("create PR watch: %v", err)
	}

	// Provider resolves a different branch for this session.
	prov := &mockTaskBranchProvider{
		branches: map[string]string{
			"s1": "new-branch",
		},
	}
	poller.SetTaskBranchProvider(prov)

	poller.refreshStaleBranches(ctx)

	// Verify the watch branch was updated.
	updated, err := store.GetPRWatchBySession(ctx, "s1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated == nil {
		t.Fatal("expected PR watch to exist")
	}
	if updated.Branch != "new-branch" {
		t.Errorf("expected branch %q, got %q", "new-branch", updated.Branch)
	}
}

func TestRefreshStaleBranches_SkipsWhenBranchUnchanged(t *testing.T) {
	poller, _, _, store := setupPollerTest(t)
	ctx := context.Background()

	seedTask(t, store, "t1", false)
	watch := &PRWatch{
		SessionID: "s1",
		TaskID:    "t1",
		Owner:     "myorg",
		Repo:      "myrepo",
		PRNumber:  0,
		Branch:    "same-branch",
	}
	if err := store.CreatePRWatch(ctx, watch); err != nil {
		t.Fatalf("create PR watch: %v", err)
	}

	prov := &mockTaskBranchProvider{
		branches: map[string]string{
			"s1": "same-branch",
		},
	}
	poller.SetTaskBranchProvider(prov)

	poller.refreshStaleBranches(ctx)

	updated, _ := store.GetPRWatchBySession(ctx, "s1")
	if updated.Branch != "same-branch" {
		t.Errorf("expected branch unchanged, got %q", updated.Branch)
	}
}

func TestRefreshStaleBranches_SkipsWatchesWithPR(t *testing.T) {
	poller, _, _, store := setupPollerTest(t)
	ctx := context.Background()

	// Watch that already found a PR (pr_number > 0).
	seedTask(t, store, "t1", false)
	watch := &PRWatch{
		SessionID: "s1",
		TaskID:    "t1",
		Owner:     "myorg",
		Repo:      "myrepo",
		PRNumber:  42,
		Branch:    "old-branch",
	}
	if err := store.CreatePRWatch(ctx, watch); err != nil {
		t.Fatalf("create PR watch: %v", err)
	}

	prov := &mockTaskBranchProvider{
		branches: map[string]string{
			"s1": "new-branch",
		},
	}
	poller.SetTaskBranchProvider(prov)

	poller.refreshStaleBranches(ctx)

	// Branch should NOT be updated because PR was already found.
	updated, _ := store.GetPRWatchBySession(ctx, "s1")
	if updated.Branch != "old-branch" {
		t.Errorf("expected branch unchanged for PR watch, got %q", updated.Branch)
	}
}

func TestRefreshStaleBranches_SkipsWhenResolverReturnsEmpty(t *testing.T) {
	poller, _, _, store := setupPollerTest(t)
	ctx := context.Background()

	seedTask(t, store, "t1", false)
	watch := &PRWatch{
		SessionID: "s1",
		TaskID:    "t1",
		Owner:     "myorg",
		Repo:      "myrepo",
		PRNumber:  0,
		Branch:    "old-branch",
	}
	if err := store.CreatePRWatch(ctx, watch); err != nil {
		t.Fatalf("create PR watch: %v", err)
	}

	// Provider returns empty string (can't resolve).
	prov := &mockTaskBranchProvider{
		branches: map[string]string{},
	}
	poller.SetTaskBranchProvider(prov)

	poller.refreshStaleBranches(ctx)

	updated, _ := store.GetPRWatchBySession(ctx, "s1")
	if updated.Branch != "old-branch" {
		t.Errorf("expected branch unchanged when resolver returns empty, got %q", updated.Branch)
	}
}

// graphQLMockClient wraps MockClient with a canned GraphQL executor so tests
// exercise the batched poll path without hitting the network.
type graphQLMockClient struct {
	*MockClient
	prResponses     []string // FIFO; one entry consumed per ExecuteGraphQL call carrying "Batch"
	branchResponses []string // FIFO; consumed for "Branches" queries
	prErr           error    // returned for the next "Batch" call
	branchErr       error    // returned for the next "Branches" call
	prQueries       []string
	branchQueries   []string
	// onExecute, if set, is called at the top of every ExecuteGraphQL before
	// canned responses are consumed. Used by the singleflight coalescing
	// test to block the first in-flight call while a second one races.
	onExecute func()
}

func (m *graphQLMockClient) ExecuteGraphQL(_ context.Context, query string, _ map[string]any, out any) error {
	if m.onExecute != nil {
		m.onExecute()
	}
	if strings.Contains(query, "query Branches") {
		m.branchQueries = append(m.branchQueries, query)
		if m.branchErr != nil {
			return m.branchErr
		}
		if len(m.branchResponses) == 0 {
			return errors.New("no canned branch response")
		}
		resp := m.branchResponses[0]
		m.branchResponses = m.branchResponses[1:]
		return json.Unmarshal([]byte(resp), out)
	}
	m.prQueries = append(m.prQueries, query)
	if m.prErr != nil {
		return m.prErr
	}
	if len(m.prResponses) == 0 {
		return errors.New("no canned PR response")
	}
	resp := m.prResponses[0]
	m.prResponses = m.prResponses[1:]
	return json.Unmarshal([]byte(resp), out)
}

func setupBatchedPollerTest(t *testing.T) (*Poller, *Service, *graphQLMockClient, *Store) {
	t.Helper()
	poller, svc, mockClient, store := setupPollerTest(t)
	wrapped := &graphQLMockClient{MockClient: mockClient}
	svc.client = wrapped
	return poller, svc, wrapped, store
}

func TestTryBatchedPRWatchCheck_NumberedWatch_AppliesStatus(t *testing.T) {
	poller, _, gh, store := setupBatchedPollerTest(t)
	ctx := context.Background()

	gh.prResponses = []string{`{
		"data": {
			"repo0": {
				"pr0": {
					"state": "OPEN", "title": "Test PR", "url": "https://x/1",
					"isDraft": false, "mergeable": "MERGEABLE", "mergeStateStatus": "CLEAN",
					"headRefName": "feat", "baseRefName": "main", "headRefOid": "abc",
					"author": {"login": "alice"},
					"createdAt": "2026-01-01T00:00:00Z", "updatedAt": "2026-01-02T00:00:00Z",
					"reviews": {"nodes": [{"state": "APPROVED", "author": {"login": "bob"}, "submittedAt": "2026-01-02T00:00:00Z"}]},
					"reviewRequests": {"totalCount": 0},
					"commits": {"nodes": [{"commit": {"statusCheckRollup": {"state": "SUCCESS"}}}]}
				}
			}
		}
	}`}

	watch := &PRWatch{
		SessionID: "s1", TaskID: "t1", Owner: "o", Repo: "r", PRNumber: 42, Branch: "feat",
		LastCheckStatus: "pending", LastReviewState: "",
	}
	if err := store.CreatePRWatch(ctx, watch); err != nil {
		t.Fatalf("create PR watch: %v", err)
	}
	taskPR := &TaskPR{
		TaskID: "t1", Owner: "o", Repo: "r", PRNumber: 42,
		PRURL: "https://x/1", PRTitle: "Test PR", HeadBranch: "feat", BaseBranch: "main", State: "open",
	}
	if err := store.CreateTaskPR(ctx, taskPR); err != nil {
		t.Fatalf("create task PR: %v", err)
	}

	if !poller.tryBatchedPRWatchCheck(ctx, []*PRWatch{watch}) {
		t.Fatalf("expected batched path to succeed")
	}

	updated, err := store.GetTaskPR(ctx, "t1")
	if err != nil || updated == nil {
		t.Fatalf("get task PR: err=%v, pr=%v", err, updated)
	}
	if updated.ChecksState != "success" {
		t.Errorf("ChecksState = %q, want success", updated.ChecksState)
	}
	if updated.ReviewState != "approved" {
		t.Errorf("ReviewState = %q, want approved", updated.ReviewState)
	}
}

func TestTryBatchedPRWatchCheck_SearchingWatch_DetectsPR(t *testing.T) {
	poller, _, gh, store := setupBatchedPollerTest(t)
	ctx := context.Background()

	gh.branchResponses = []string{`{
		"data": {
			"b0": {
				"pullRequests": {
					"nodes": [{
						"number": 7,
						"state": "OPEN", "title": "branch PR", "url": "https://x/7",
						"isDraft": false, "mergeable": "MERGEABLE", "mergeStateStatus": "CLEAN",
						"headRefName": "feat", "baseRefName": "main", "headRefOid": "deadbeef",
						"author": {"login": "alice"},
						"createdAt": "2026-01-01T00:00:00Z", "updatedAt": "2026-01-01T00:00:00Z",
						"reviews": {"nodes": []}, "reviewRequests": {"totalCount": 0},
						"commits": {"nodes": []}
					}]
				}
			}
		}
	}`}

	watch := &PRWatch{
		SessionID: "s2", TaskID: "t2", Owner: "o", Repo: "r", PRNumber: 0, Branch: "feat",
	}
	if err := store.CreatePRWatch(ctx, watch); err != nil {
		t.Fatalf("create PR watch: %v", err)
	}

	if !poller.tryBatchedPRWatchCheck(ctx, []*PRWatch{watch}) {
		t.Fatalf("expected batched path to succeed")
	}

	updated, err := store.GetPRWatchBySession(ctx, "s2")
	if err != nil || updated == nil {
		t.Fatalf("get PR watch: err=%v, w=%v", err, updated)
	}
	if updated.PRNumber != 7 {
		t.Errorf("PRNumber = %d, want 7 (PR should be detected and recorded)", updated.PRNumber)
	}
}

func TestTryBatchedPRWatchCheck_FallsBackOnUnsupportedClient(t *testing.T) {
	// MockClient does not implement GraphQLExecutor, so the batched path must
	// return false to trigger per-watch fallback.
	poller, _, _, store := setupPollerTest(t)
	ctx := context.Background()

	watch := &PRWatch{SessionID: "s1", TaskID: "t1", Owner: "o", Repo: "r", PRNumber: 1, Branch: "feat"}
	if err := store.CreatePRWatch(ctx, watch); err != nil {
		t.Fatalf("create PR watch: %v", err)
	}
	if poller.tryBatchedPRWatchCheck(ctx, []*PRWatch{watch}) {
		t.Errorf("expected false when client does not implement GraphQLExecutor")
	}
}

// TestSyncWatchesBatched_ManyWatches_TwoGraphQLCalls is the regression test
// for the OS-freeze caused by per-watch gh subprocess fan-out. A workspace
// with N numbered + M searching watches must fire exactly TWO GraphQL
// queries (one numbered, one searching), regardless of N or M — that's
// what keeps the gh subprocess burst bounded.
func TestSyncWatchesBatched_ManyWatches_TwoGraphQLCalls(t *testing.T) {
	_, svc, gh, store := setupBatchedPollerTest(t)
	ctx := context.Background()

	// Canned responses: one PR alias per numbered watch, one branch alias per
	// searching watch. Hand-rolled minimal payloads so the assertion stays on
	// "did we batch?" rather than on the field decoder.
	gh.prResponses = []string{`{"data":{
		"repo0":{"pr0":{"state":"OPEN","title":"a","url":"x","headRefName":"a","baseRefName":"main","headRefOid":"1","author":{"login":"x"},"createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-01T00:00:00Z","reviews":{"nodes":[]},"reviewRequests":{"totalCount":0},"commits":{"nodes":[]}}},
		"repo1":{"pr0":{"state":"OPEN","title":"b","url":"x","headRefName":"b","baseRefName":"main","headRefOid":"2","author":{"login":"x"},"createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-01T00:00:00Z","reviews":{"nodes":[]},"reviewRequests":{"totalCount":0},"commits":{"nodes":[]}}},
		"repo2":{"pr0":{"state":"OPEN","title":"c","url":"x","headRefName":"c","baseRefName":"main","headRefOid":"3","author":{"login":"x"},"createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-01T00:00:00Z","reviews":{"nodes":[]},"reviewRequests":{"totalCount":0},"commits":{"nodes":[]}}}
	}}`}
	gh.branchResponses = []string{`{"data":{
		"b0":{"pullRequests":{"nodes":[]}},
		"b1":{"pullRequests":{"nodes":[]}},
		"b2":{"pullRequests":{"nodes":[]}},
		"b3":{"pullRequests":{"nodes":[]}}
	}}`}

	var watches []*PRWatch
	for i := 0; i < 3; i++ {
		w := &PRWatch{SessionID: "n" + string(rune('a'+i)), TaskID: "tn" + string(rune('a'+i)),
			Owner: "o", Repo: "r", PRNumber: i + 1, Branch: "n"}
		if err := store.CreatePRWatch(ctx, w); err != nil {
			t.Fatalf("create numbered: %v", err)
		}
		watches = append(watches, w)
	}
	for i := 0; i < 4; i++ {
		w := &PRWatch{SessionID: "s" + string(rune('a'+i)), TaskID: "ts" + string(rune('a'+i)),
			Owner: "o", Repo: "r", PRNumber: 0, Branch: "s" + string(rune('a'+i))}
		if err := store.CreatePRWatch(ctx, w); err != nil {
			t.Fatalf("create searching: %v", err)
		}
		watches = append(watches, w)
	}

	if _, err := svc.SyncWatchesBatched(ctx, watches); err != nil {
		t.Fatalf("SyncWatchesBatched: %v", err)
	}

	// THE assertion: 7 watches → exactly 1 numbered query + 1 branch query.
	// Pre-fix this would have been 7 separate gh subprocess invocations.
	if got := len(gh.prQueries); got != 1 {
		t.Errorf("numbered GraphQL calls = %d, want 1 (all numbered watches must batch)", got)
	}
	if got := len(gh.branchQueries); got != 1 {
		t.Errorf("branch GraphQL calls = %d, want 1 (all searching watches must batch)", got)
	}
}

func TestSyncWatchesBatched_NumberedQueryError_ReturnsError(t *testing.T) {
	_, svc, gh, _ := setupBatchedPollerTest(t)
	ctx := context.Background()
	gh.prErr = errors.New("graphql 500")

	numbered := []*PRWatch{{Owner: "o", Repo: "r", PRNumber: 1}}
	got, err := svc.SyncWatchesBatched(ctx, numbered)
	if err == nil || got != nil {
		t.Errorf("expected (nil, err) on numbered query error, got (%v, %v)", got, err)
	}
}

func TestSyncWatchesBatched_BranchQueryError_ReturnsError(t *testing.T) {
	// Greptile P2 (preserved from previous fetchBatchedStatuses test): when only
	// the branch query fails (no numbered watches), an empty non-nil map
	// silently absorbed the failure. The fix returns a non-nil error so the
	// caller falls back per-watch.
	_, svc, gh, _ := setupBatchedPollerTest(t)
	ctx := context.Background()
	gh.branchErr = errors.New("graphql 500")

	searching := []*PRWatch{{Owner: "o", Repo: "r", PRNumber: 0, Branch: "feat"}}
	got, err := svc.SyncWatchesBatched(ctx, searching)
	if err == nil || got != nil {
		t.Errorf("expected (nil, err) on branch query error, got (%v, %v)", got, err)
	}
}

func TestSyncWatchesBatched_MergedPR_ResetsWatch(t *testing.T) {
	_, svc, gh, store := setupBatchedPollerTest(t)
	ctx := context.Background()

	watch := &PRWatch{
		SessionID: "s1", TaskID: "t1", Owner: "o", Repo: "r", PRNumber: 99, Branch: "feat",
	}
	if err := store.CreatePRWatch(ctx, watch); err != nil {
		t.Fatalf("create PR watch: %v", err)
	}
	taskPR := &TaskPR{
		TaskID: "t1", Owner: "o", Repo: "r", PRNumber: 99,
		PRURL: "https://x/99", PRTitle: "Merged PR", HeadBranch: "feat", BaseBranch: "main", State: "open",
	}
	if err := store.CreateTaskPR(ctx, taskPR); err != nil {
		t.Fatalf("create task PR: %v", err)
	}

	// Canned merged-PR response — convertBatchedPRResult promotes state to
	// "merged" whenever mergedAt is non-empty, which is what triggers the
	// watch reset.
	gh.prResponses = []string{`{
		"data": {
			"repo0": {
				"pr0": {
					"state": "MERGED", "title": "Merged PR", "url": "https://x/99",
					"isDraft": false, "mergeable": "MERGEABLE", "mergeStateStatus": "CLEAN",
					"headRefName": "feat", "baseRefName": "main", "headRefOid": "abc",
					"author": {"login": "alice"},
					"createdAt": "2026-01-01T00:00:00Z", "updatedAt": "2026-01-02T00:00:00Z",
					"mergedAt": "2026-01-02T00:00:00Z",
					"reviews": {"nodes": []},
					"reviewRequests": {"totalCount": 0},
					"commits": {"nodes": [{"commit": {"statusCheckRollup": {"state": "SUCCESS"}}}]}
				}
			}
		}
	}`}

	if _, err := svc.SyncWatchesBatched(ctx, []*PRWatch{watch}); err != nil {
		t.Fatalf("SyncWatchesBatched: %v", err)
	}

	updated, err := store.GetPRWatchBySession(ctx, "s1")
	if err != nil || updated == nil {
		t.Fatalf("get PR watch: err=%v, w=%v", err, updated)
	}
	if updated.PRNumber != 0 {
		t.Errorf("expected watch reset to PRNumber=0 after merge, got %d", updated.PRNumber)
	}
}

// TestRefreshStaleWorkspaceWatches_CoalescesConcurrentCalls regression-tests
// the singleflight guard on per-workspace background refresh. Without it,
// the frontend's repeated WS polls (every ~5s) stack background goroutines
// that each fire a batched GraphQL request, defeating the per-process
// throttle once the cap is small enough to queue.
func TestRefreshStaleWorkspaceWatches_CoalescesConcurrentCalls(t *testing.T) {
	_, svc, gh, store := setupBatchedPollerTest(t)
	ctx := context.Background()

	// Seed two tasks with one searching watch each. Searching (PRNumber=0)
	// means refreshStaleWorkspaceWatches drives them through the branch
	// query path, which is what onExecute will block on.
	for _, id := range []string{"t1", "t2"} {
		w := &PRWatch{SessionID: "sess-" + id, TaskID: id, Owner: "o", Repo: "r", PRNumber: 0, Branch: "br-" + id}
		if err := store.CreatePRWatch(ctx, w); err != nil {
			t.Fatalf("create watch: %v", err)
		}
	}

	// Two canned branch responses so neither the first NOR a hypothetical
	// duplicate refresh starves on missing data — we want the assertion to
	// be "duplicate never ran", not "duplicate failed".
	gh.branchResponses = []string{
		`{"data":{"b0":{"pullRequests":{"nodes":[]}},"b1":{"pullRequests":{"nodes":[]}}}}`,
		`{"data":{"b0":{"pullRequests":{"nodes":[]}},"b1":{"pullRequests":{"nodes":[]}}}}`,
	}

	started := make(chan struct{}, 4)
	release := make(chan struct{})
	gh.onExecute = func() {
		started <- struct{}{}
		<-release
	}

	staleTasks := map[string]struct{}{"t1": {}, "t2": {}}

	// Kick off the first refresh and wait until it's actually inside ExecuteGraphQL.
	svc.refreshStaleWorkspaceWatches("ws1", staleTasks)
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatalf("first refresh never entered ExecuteGraphQL")
	}

	// Fire a second refresh for the SAME workspace while the first is blocked.
	// The singleflight guard must drop it on the floor — no new goroutine, no
	// new GraphQL call. A small grace period lets a buggy unguarded goroutine
	// reach ExecuteGraphQL if it were going to.
	svc.refreshStaleWorkspaceWatches("ws1", staleTasks)
	select {
	case <-started:
		t.Fatalf("second refresh entered ExecuteGraphQL while first was in flight — coalescing broken")
	case <-time.After(100 * time.Millisecond):
	}

	// Release the first and confirm post-completion refreshes work again.
	close(release)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, busy := svc.inflightWorkspaceRefreshes.Load("ws1"); !busy {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if _, busy := svc.inflightWorkspaceRefreshes.Load("ws1"); busy {
		t.Fatalf("inflight key for ws1 never cleared after refresh completion")
	}

	// A fresh refresh must start (key was cleared in the defer).
	release2 := make(chan struct{})
	gh.onExecute = func() {
		started <- struct{}{}
		<-release2
	}
	svc.refreshStaleWorkspaceWatches("ws1", staleTasks)
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatalf("post-completion refresh never started — inflight key leaked")
	}
	close(release2)
}

func TestWaitForRateLimit_SkipsWhenHealthy(t *testing.T) {
	poller, _, _, _ := setupPollerTest(t)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if !poller.waitForRateLimit(ctx, ResourceGraphQL, "test") {
		t.Errorf("expected true when no exhaustion recorded")
	}
}

func TestSearchBucketExhausted(t *testing.T) {
	poller, svc, _, _ := setupPollerTest(t)

	if poller.searchBucketExhausted("test") {
		t.Errorf("expected false when no exhaustion recorded")
	}

	// Mark the search bucket exhausted with a future reset.
	svc.RateTracker().Record(RateSnapshot{
		Resource:  ResourceSearch,
		Limit:     30,
		Remaining: 0,
		ResetAt:   time.Now().Add(5 * time.Minute),
		UpdatedAt: time.Now(),
	})

	if !poller.searchBucketExhausted("test") {
		t.Errorf("expected true when search bucket exhausted")
	}

	// Other buckets exhausting must not trip the search guard.
	poller2, svc2, _, _ := setupPollerTest(t)
	svc2.RateTracker().Record(RateSnapshot{
		Resource:  ResourceCore,
		Remaining: 0,
		ResetAt:   time.Now().Add(5 * time.Minute),
		UpdatedAt: time.Now(),
	})
	if poller2.searchBucketExhausted("test") {
		t.Errorf("expected false when only core (not search) is exhausted")
	}
}

// Regression: when the search bucket trips mid-cycle, checkReviewWatches must
// stop iterating so the remaining watches don't issue doomed search requests
// that deepen the secondary-limit penalty.
func TestCheckReviewWatches_BailsOutWhenSearchExhaustedMidCycle(t *testing.T) {
	poller, svc, _, store := setupPollerTest(t)
	ctx := context.Background()

	// Pre-exhaust the search bucket so the very first iteration short-circuits.
	svc.RateTracker().Record(RateSnapshot{
		Resource:  ResourceSearch,
		Remaining: 0,
		ResetAt:   time.Now().Add(5 * time.Minute),
		UpdatedAt: time.Now(),
	})

	for _, id := range []string{"rw1", "rw2", "rw3"} {
		if err := store.CreateReviewWatch(ctx, &ReviewWatch{
			ID:          id,
			WorkspaceID: "ws1",
			Enabled:     true,
		}); err != nil {
			t.Fatalf("seed review watch %s: %v", id, err)
		}
	}

	// Should return cleanly without panic / without errors despite N watches
	// being enabled.
	poller.checkReviewWatches(ctx)
}

func TestCheckIssueWatches_BailsOutWhenSearchExhaustedMidCycle(t *testing.T) {
	poller, svc, _, store := setupPollerTest(t)
	ctx := context.Background()

	svc.RateTracker().Record(RateSnapshot{
		Resource:  ResourceSearch,
		Remaining: 0,
		ResetAt:   time.Now().Add(5 * time.Minute),
		UpdatedAt: time.Now(),
	})

	for _, id := range []string{"iw1", "iw2"} {
		if err := store.CreateIssueWatch(ctx, &IssueWatch{
			ID:          id,
			WorkspaceID: "ws1",
			Enabled:     true,
		}); err != nil {
			t.Fatalf("seed issue watch %s: %v", id, err)
		}
	}

	poller.checkIssueWatches(ctx)
}

func TestWaitForRateLimit_ReturnsFalseOnContextCancel(t *testing.T) {
	poller, svc, _, _ := setupPollerTest(t)
	// Force exhaustion with a far-future reset.
	svc.RateTracker().Record(RateSnapshot{
		Resource:  ResourceGraphQL,
		Limit:     5000,
		Remaining: 0,
		ResetAt:   time.Now().Add(time.Hour),
		UpdatedAt: time.Now(),
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately so the timer select returns ctx.Done first
	if poller.waitForRateLimit(ctx, ResourceGraphQL, "test") {
		t.Errorf("expected false when context cancelled during sleep")
	}
}
