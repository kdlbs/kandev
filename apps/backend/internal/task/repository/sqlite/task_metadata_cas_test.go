package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

const casTaskID = "task-metadata-cas"

func seedMetadataCASTask(t *testing.T, repo *Repository, metadata map[string]interface{}) {
	t.Helper()
	if err := repo.CreateTask(context.Background(), &models.Task{
		ID: casTaskID, WorkspaceID: "ws-cas", WorkflowID: "wf-cas",
		Title: "CAS", Metadata: metadata,
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}
}

func metadataValue(t *testing.T, repo *Repository, key string) (interface{}, bool) {
	t.Helper()
	task, err := repo.GetTask(context.Background(), casTaskID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	value, ok := task.Metadata[key]
	return value, ok
}

// runMetadataCASContract is the shared behaviour both dialects must satisfy.
// SetTaskMetadataKeyIfPresent is the guard that stops a deferred-launch prompt
// edit from re-creating an intent a concurrent start already consumed, so its
// two halves — rewrite when present, refuse when absent — are what matter.
func runMetadataCASContract(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()

	written, err := repo.SetTaskMetadataKeyIfPresent(ctx, casTaskID, "deferred_launch",
		map[string]interface{}{"prompt": "refreshed"})
	if err != nil {
		t.Fatalf("SetTaskMetadataKeyIfPresent on a present key: %v", err)
	}
	if !written {
		t.Fatal("a present key must be rewritten")
	}
	value, ok := metadataValue(t, repo, "deferred_launch")
	if !ok {
		t.Fatal("the rewritten key vanished")
	}
	launch, _ := value.(map[string]interface{})
	if launch["prompt"] != "refreshed" {
		t.Fatalf("prompt = %v, want the rewritten value", launch["prompt"])
	}
	if _, untouched := metadataValue(t, repo, "other_key"); !untouched {
		t.Fatal("the patch must not disturb neighbouring metadata keys")
	}

	// The case the guard exists for: a concurrent claim removed the key.
	if _, err := repo.RemoveTaskMetadataKey(ctx, casTaskID, "deferred_launch"); err != nil {
		t.Fatalf("RemoveTaskMetadataKey: %v", err)
	}
	written, err = repo.SetTaskMetadataKeyIfPresent(ctx, casTaskID, "deferred_launch",
		map[string]interface{}{"prompt": "too late"})
	if err != nil {
		t.Fatalf("SetTaskMetadataKeyIfPresent on an absent key: %v", err)
	}
	if written {
		t.Fatal("an absent key must not be re-created")
	}
	if _, resurrected := metadataValue(t, repo, "deferred_launch"); resurrected {
		t.Fatal("the patch resurrected a key a concurrent claim had consumed")
	}
	if _, untouched := metadataValue(t, repo, "other_key"); !untouched {
		t.Fatal("a refused patch must leave the rest of the metadata alone")
	}
}

func newRepoForMetadataCASTests(t *testing.T) *Repository {
	t.Helper()
	dbConn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "metadata-cas-test.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	repo, err := NewWithDB(sqlxDB, sqlxDB, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	t.Cleanup(func() { _ = sqlxDB.Close() })
	return repo
}

func TestSetTaskMetadataKeyIfPresentSQLite(t *testing.T) {
	repo := newRepoForMetadataCASTests(t)
	seedMetadataCASTask(t, repo, map[string]interface{}{
		"deferred_launch": map[string]interface{}{"prompt": "stale"},
		"other_key":       "keep me",
	})
	runMetadataCASContract(t, repo)
}

// The JSON patch and its presence predicate are written per dialect, so SQLite
// coverage says nothing about the PostgreSQL statement. Skips unless
// KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresSetTaskMetadataKeyIfPresent(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo := newPostgresMetadataCASRepo(t, db)
	seedMetadataCASTask(t, repo, map[string]interface{}{
		"deferred_launch": map[string]interface{}{"prompt": "stale"},
		"other_key":       "keep me",
	})
	runMetadataCASContract(t, repo)
}

func newPostgresMetadataCASRepo(t *testing.T, db *sqlx.DB) *Repository {
	t.Helper()
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	return repo
}
