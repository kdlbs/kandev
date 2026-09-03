package maintenance

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/sqlite"
)

// testFixture is a real on-disk SQLite database (maintenance opens
// connections by file path, not by in-memory handle, so tests need a real
// file to point Run/compact/backup at) seeded through the task repository's
// public API wherever one exists, falling back to minimal raw inserts only
// for rows with no exported constructor (turns).
type testFixture struct {
	t      *testing.T
	dbPath string
	repo   *sqlite.Repository
	sqlxDB *sqlx.DB
}

func newTestFixture(t *testing.T) *testFixture {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "maintenance-test.db")
	conn, err := db.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlxDB := sqlx.NewDb(conn, "sqlite3")
	repo, err := sqlite.NewWithDB(sqlxDB, sqlxDB, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	f := &testFixture{t: t, dbPath: dbPath, repo: repo, sqlxDB: sqlxDB}
	t.Cleanup(func() { _ = sqlxDB.Close() })
	return f
}

// closeForSwap closes the fixture's own connection so a compaction rename
// (or a second Run invocation opening its own connections) can proceed
// without a stale open handle. Safe to call at most meaningfully once;
// later calls (e.g. via t.Cleanup) are no-ops on an already-closed *sql.DB.
func (f *testFixture) closeForSwap() error {
	return f.sqlxDB.Close()
}

func (f *testFixture) seedTask(taskID string) {
	f.t.Helper()
	// WorkspaceID must be non-empty: initSchema's ensureDefaultWorkspace
	// backfill runs on every repository open (including the second
	// connection Run() opens against this same file) and would otherwise
	// try to derive it from a (non-existent, in this minimal fixture)
	// workflow row, violating tasks.workspace_id's NOT NULL constraint.
	if err := f.repo.CreateTask(context.Background(), &models.Task{ID: taskID, WorkspaceID: "workspace-test", Title: taskID}); err != nil {
		f.t.Fatalf("CreateTask(%s): %v", taskID, err)
	}
}

func (f *testFixture) seedSession(taskID, sessionID string) {
	f.t.Helper()
	if err := f.repo.CreateTaskSession(context.Background(), &models.TaskSession{ID: sessionID, TaskID: taskID}); err != nil {
		f.t.Fatalf("CreateTaskSession(%s): %v", sessionID, err)
	}
}

// seedDuplicateGitSnapshots inserts two identical-content snapshots for
// sessionID, which ListDuplicateGitSnapshotCandidates reports the older of
// as a retention candidate (CreateGitSnapshot always records every poll -
// no write-time dedup skip - so this reliably produces one candidate row).
func (f *testFixture) seedDuplicateGitSnapshots(sessionID string, base time.Time) {
	f.t.Helper()
	ctx := context.Background()
	snap := func(id string, createdAt time.Time) *models.GitSnapshot {
		return &models.GitSnapshot{
			ID: id, SessionID: sessionID, SnapshotType: models.SnapshotTypeStatusUpdate,
			Branch: "feature/x", RemoteBranch: "origin/feature/x",
			HeadCommit: "head-sha", BaseCommit: "base-sha",
			Files:     map[string]interface{}{"a.go": map[string]interface{}{"status": "modified"}},
			CreatedAt: createdAt,
		}
	}
	if err := f.repo.CreateGitSnapshot(ctx, snap(sessionID+"-snap-1", base)); err != nil {
		f.t.Fatalf("CreateGitSnapshot(1): %v", err)
	}
	if err := f.repo.CreateGitSnapshot(ctx, snap(sessionID+"-snap-2", base.Add(time.Minute))); err != nil {
		f.t.Fatalf("CreateGitSnapshot(2): %v", err)
	}
}

// seedObsoletePlanRevisions inserts count revisions for taskID; every
// revision except the last (HEAD) is a retention candidate.
func (f *testFixture) seedObsoletePlanRevisions(taskID string, count int, base time.Time) {
	f.t.Helper()
	ctx := context.Background()
	for i := 1; i <= count; i++ {
		rev := &models.TaskPlanRevision{
			ID: fmt.Sprintf("%s-rev-%d", taskID, i), TaskID: taskID,
			RevisionNumber: i, Title: "v", Content: "revision content",
			CreatedAt: base.Add(time.Duration(i) * time.Hour),
		}
		if err := f.repo.InsertTaskPlanRevision(ctx, rev); err != nil {
			f.t.Fatalf("InsertTaskPlanRevision(%d): %v", i, err)
		}
	}
}

// seedOrphanedMessagePayload creates one large externalized payload and
// then deletes its only referencing message, leaving the payload row
// orphaned. Returns the digest.
func (f *testFixture) seedOrphanedMessagePayload(taskID, sessionID, turnID string) string {
	f.t.Helper()
	ctx := context.Background()
	f.seedTurn(sessionID, taskID, turnID)

	msg := &models.Message{
		ID: taskID + "-orphan-msg", TaskSessionID: sessionID, AuthorType: models.MessageAuthorAgent,
		Type: models.MessageTypeToolCall, TurnID: turnID,
		Metadata: shellOutputMetadataFixture(strings.Repeat("o", 5000)),
	}
	if err := f.repo.CreateMessage(ctx, msg); err != nil {
		f.t.Fatalf("CreateMessage: %v", err)
	}
	if msg.PayloadDigest == "" {
		f.t.Fatal("expected message to externalize a payload digest")
	}
	if _, err := f.repo.DB().ExecContext(ctx, `DELETE FROM task_session_messages WHERE id = ?`, msg.ID); err != nil {
		f.t.Fatalf("delete message: %v", err)
	}
	return msg.PayloadDigest
}

func (f *testFixture) seedTurn(sessionID, taskID, turnID string) {
	f.t.Helper()
	now := time.Now().UTC()
	if _, err := f.repo.DB().Exec(`
		INSERT OR IGNORE INTO task_session_turns
			(id, task_session_id, task_id, started_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, turnID, sessionID, taskID, now, now, now); err != nil {
		f.t.Fatalf("seed turn %s: %v", turnID, err)
	}
}

func shellOutputMetadataFixture(stdout string) map[string]interface{} {
	return map[string]interface{}{
		"tool_call_id": "call-1",
		"normalized": map[string]interface{}{
			"shell_exec": map[string]interface{}{
				"command": "echo",
				"output": map[string]interface{}{
					"stdout":    stdout,
					"stderr":    "",
					"truncated": false,
				},
			},
		},
	}
}

func (f *testFixture) countRows(query string, args ...interface{}) int {
	f.t.Helper()
	var n int
	if err := f.repo.DB().QueryRow(query, args...).Scan(&n); err != nil {
		f.t.Fatalf("count query %q: %v", query, err)
	}
	return n
}

// openRepoForVerification opens a fresh connection to an existing database
// file (e.g. one just replaced by a compaction swap) so a test can confirm
// the post-swap file is a fully valid, readable database with the expected
// rows - independent of any connection the code under test held open.
func openRepoForVerification(t *testing.T, dbPath string) *sqlite.Repository {
	t.Helper()
	conn, err := db.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open sqlite for verification: %v", err)
	}
	sqlxDB := sqlx.NewDb(conn, "sqlite3")
	t.Cleanup(func() { _ = sqlxDB.Close() })
	repo, err := sqlite.NewWithDB(sqlxDB, sqlxDB, nil)
	if err != nil {
		t.Fatalf("new repo for verification: %v", err)
	}
	return repo
}

func (f *testFixture) fileSize() int64 {
	f.t.Helper()
	info, err := os.Stat(f.dbPath)
	if err != nil {
		f.t.Fatalf("stat %s: %v", f.dbPath, err)
	}
	return info.Size()
}
