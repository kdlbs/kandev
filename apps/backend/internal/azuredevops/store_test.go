package azuredevops

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

func newTestDB(t *testing.T) *sqlx.DB {
	t.Helper()
	raw, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	raw.SetMaxOpenConns(1)
	raw.SetMaxIdleConns(1)
	db := sqlx.NewDb(raw, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestStoreConfigRoundTripAndHealth(t *testing.T) {
	db := newTestDB(t)
	store, err := NewStore(db, db)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	ctx := context.Background()
	cfg := &Config{
		WorkspaceID:        "ws-a",
		OrganizationURL:    "https://dev.azure.com/acme",
		DefaultProjectID:   "project-guid",
		DefaultProjectName: "Platform",
		AuthMethod:         AuthMethodPAT,
	}
	if err := store.UpsertConfig(ctx, cfg); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	checkedAt := time.Now().UTC().Truncate(time.Millisecond)
	if err := store.UpdateAuthHealth(ctx, "ws-a", true, "", checkedAt); err != nil {
		t.Fatalf("update health: %v", err)
	}
	got, err := store.GetConfig(ctx, "ws-a")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got == nil || got.OrganizationURL != cfg.OrganizationURL || got.DefaultProjectID != cfg.DefaultProjectID {
		t.Fatalf("round trip mismatch: got %+v want %+v", got, cfg)
	}
	if !got.LastOK || got.LastCheckedAt == nil || got.LastError != "" {
		t.Fatalf("health not persisted: %+v", got)
	}
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Fatalf("timestamps not populated: %+v", got)
	}
	ids, err := store.ListConfigWorkspaceIDs(ctx)
	if err != nil {
		t.Fatalf("list workspace ids: %v", err)
	}
	if len(ids) != 1 || ids[0] != "ws-a" {
		t.Fatalf("workspace ids = %v", ids)
	}
}

func TestStoreConfigReplayAndDelete(t *testing.T) {
	db := newTestDB(t)
	first, err := NewStore(db, db)
	if err != nil {
		t.Fatalf("first store: %v", err)
	}
	if err := first.UpsertConfig(context.Background(), &Config{
		WorkspaceID: "ws-a", OrganizationURL: "https://dev.azure.com/acme", AuthMethod: AuthMethodPAT,
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	second, err := NewStore(db, db)
	if err != nil {
		t.Fatalf("replayed store: %v", err)
	}
	if err := second.DeleteConfig(context.Background(), "ws-a"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	got, err := second.GetConfig(context.Background(), "ws-a")
	if err != nil || got != nil {
		t.Fatalf("after delete got=%+v err=%v", got, err)
	}
}

func TestStoreTaskPRSchemaAndReplay(t *testing.T) {
	db := newTestDB(t)
	if _, err := NewStore(db, db); err != nil {
		t.Fatalf("first store: %v", err)
	}
	var tableName string
	if err := db.Get(&tableName, `SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'azure_devops_task_prs'`); err != nil {
		t.Fatalf("task PR table missing: %v", err)
	}
	if tableName != "azure_devops_task_prs" {
		t.Fatalf("task PR table = %q", tableName)
	}
	if _, err := NewStore(db, db); err != nil {
		t.Fatalf("replayed store: %v", err)
	}
}

type taskPRStoreContract interface {
	UpsertTaskPR(context.Context, *TaskPR) error
	ListTaskPRsByTask(context.Context, string) ([]*TaskPR, error)
	ListTaskPRsByWorkspace(context.Context, string) (map[string][]*TaskPR, error)
	DeleteTaskPRsByTask(context.Context, string) error
}

func TestStoreDeleteTaskPRsByTaskIsScoped(t *testing.T) {
	store, err := NewStore(newTestDB(t), nil)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	ctx := context.Background()
	for _, row := range []*TaskPR{
		testTaskPR("task-a", "repo-a", 1),
		testTaskPR("task-b", "repo-b", 2),
	} {
		if err := store.UpsertTaskPR(ctx, row); err != nil {
			t.Fatalf("upsert task PR: %v", err)
		}
	}
	if err := store.DeleteTaskPRsByTask(ctx, "task-a"); err != nil {
		t.Fatalf("delete task PRs: %v", err)
	}
	deleted, err := store.ListTaskPRsByTask(ctx, "task-a")
	if err != nil || len(deleted) != 0 {
		t.Fatalf("deleted task rows = %+v, err = %v", deleted, err)
	}
	remaining, err := store.ListTaskPRsByTask(ctx, "task-b")
	if err != nil || len(remaining) != 1 {
		t.Fatalf("remaining task rows = %+v, err = %v", remaining, err)
	}
}

func TestStoreTaskPRUpsertAndRestartPersistence(t *testing.T) {
	db := newTestDB(t)
	store, err := NewStore(db, db)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	contract, ok := any(store).(taskPRStoreContract)
	if !ok {
		t.Fatal("Store does not implement the task PR persistence contract")
	}
	ctx := context.Background()
	first := testTaskPR("task-a", "repo-a", 42)
	if err := contract.UpsertTaskPR(ctx, first); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	firstID, firstCreatedAt := first.ID, first.CreatedAt
	first.Title = "Updated title"
	first.ReviewState = "approved"
	if err := contract.UpsertTaskPR(ctx, first); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	replayed, err := NewStore(db, db)
	if err != nil {
		t.Fatalf("replayed store: %v", err)
	}
	replayedContract, ok := any(replayed).(taskPRStoreContract)
	if !ok {
		t.Fatal("replayed Store does not implement the task PR persistence contract")
	}
	rows, err := replayedContract.ListTaskPRsByTask(ctx, "task-a")
	if err != nil {
		t.Fatalf("list by task: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != firstID || !rows[0].CreatedAt.Equal(firstCreatedAt) {
		t.Fatalf("upsert changed stable identity: %+v", rows)
	}
	if rows[0].Title != "Updated title" || rows[0].ReviewState != "approved" {
		t.Fatalf("mutable fields were not refreshed: %+v", rows[0])
	}
}

func TestStoreTaskPRConcurrentUpsertPreservesStableIdentity(t *testing.T) {
	store, err := NewStore(newTestDB(t), nil)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	const writers = 12
	ids := make(chan string, writers)
	errs := make(chan error, writers)
	var wait sync.WaitGroup
	for index := 0; index < writers; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			row := testTaskPR("task-a", "repo-a", 42)
			row.Title = fmt.Sprintf("Title %d", index)
			if err := store.UpsertTaskPR(context.Background(), row); err != nil {
				errs <- err
				return
			}
			ids <- row.ID
		}(index)
	}
	wait.Wait()
	close(errs)
	close(ids)
	for err := range errs {
		t.Fatalf("concurrent upsert: %v", err)
	}
	stableID := ""
	for id := range ids {
		if stableID == "" {
			stableID = id
		}
		if id != stableID {
			t.Fatalf("concurrent upsert IDs differ: %q and %q", stableID, id)
		}
	}
	rows, err := store.ListTaskPRsByTask(context.Background(), "task-a")
	if err != nil || len(rows) != 1 || rows[0].ID != stableID {
		t.Fatalf("rows after concurrent upsert = %+v, err = %v", rows, err)
	}
}

func TestStoreTaskPRListsByTaskAndWorkspace(t *testing.T) {
	db := newTestDB(t)
	if _, err := db.Exec(`CREATE TABLE tasks (id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL)`); err != nil {
		t.Fatalf("create tasks table: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO tasks (id, workspace_id) VALUES ('task-a', 'ws-a'), ('task-b', 'ws-a'), ('task-c', 'ws-b')`); err != nil {
		t.Fatalf("seed tasks: %v", err)
	}
	store, err := NewStore(db, db)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	contract, ok := any(store).(taskPRStoreContract)
	if !ok {
		t.Fatal("Store does not implement the task PR persistence contract")
	}
	ctx := context.Background()
	for _, row := range []*TaskPR{
		testTaskPR("task-a", "repo-a", 1),
		testTaskPR("task-a", "repo-b", 2),
		testTaskPR("task-b", "repo-a", 3),
		testTaskPR("task-c", "repo-c", 4),
	} {
		if err := contract.UpsertTaskPR(ctx, row); err != nil {
			t.Fatalf("upsert task PR: %v", err)
		}
	}
	byTask, err := contract.ListTaskPRsByTask(ctx, "task-a")
	if err != nil || len(byTask) != 2 {
		t.Fatalf("list by task: rows=%+v err=%v", byTask, err)
	}
	byWorkspace, err := contract.ListTaskPRsByWorkspace(ctx, "ws-a")
	if err != nil {
		t.Fatalf("list by workspace: %v", err)
	}
	if len(byWorkspace) != 2 || len(byWorkspace["task-a"]) != 2 || len(byWorkspace["task-b"]) != 1 {
		t.Fatalf("workspace grouping = %+v", byWorkspace)
	}
	if _, leaked := byWorkspace["task-c"]; leaked {
		t.Fatalf("workspace result leaked task-c: %+v", byWorkspace)
	}
}

func testTaskPR(taskID, repositoryID string, pullRequestID int) *TaskPR {
	return &TaskPR{
		TaskID: taskID, RepositoryID: repositoryID,
		OrganizationURL: "https://dev.azure.com/acme", ProjectID: "project-1",
		AzureRepositoryID: repositoryID + "-azure", PullRequestID: pullRequestID,
		PullRequestURL: "https://dev.azure.com/acme/project/_git/repo/pullrequest/42",
		Title:          "Initial title", SourceBranch: "feature", TargetBranch: "main",
		AuthorID: "author-1", AuthorName: "Ada", Status: "active",
	}
}
