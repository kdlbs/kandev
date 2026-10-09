package sqlite

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	sqlite3 "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
)

func newSidebarReaderPool(t testing.TB) *Repository {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sidebar-scratch.db")
	writerDB, err := db.OpenSQLite(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = writerDB.Close() })
	readerDB, err := db.OpenSQLiteReader(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = readerDB.Close() })
	repo, err := NewWithDB(sqlx.NewDb(writerDB, "sqlite3"), sqlx.NewDb(readerDB, "sqlite3"), nil)
	require.NoError(t, err)
	return repo
}

func TestSidebarQueryScratchLifecycle(t *testing.T) {
	for _, stage := range []string{"created", "indexed", "preferences", "page", "queue", "empty_count", "headers", "hydrated", "commit", "success"} {
		t.Run(stage, func(t *testing.T) {
			repo := newSidebarReaderPool(t)
			repo.ro.SetMaxOpenConns(1)
			seedWorkspace(t, repo, "scratch")
			if stage != "empty_count" {
				require.NoError(t, repo.CreateTask(t.Context(), &models.Task{ID: "kept", WorkspaceID: "scratch", Title: "Kept"}))
			}
			injected := errors.New("injected sidebar stage failure")
			repo.sidebarQueryStage = func(current string, _ *sqlx.Tx) error {
				if current == stage {
					return injected
				}
				return nil
			}
			query := sidebarTaskQuery(1)
			query.Sort = models.SidebarTaskViewSort{Key: "color", Color: "red", Direction: "desc"}
			query.CollapsedGroupKeys = []string{"nonmatching-group"}
			prefs := sidebarColorPreferencesForTest()
			prefs.PinnedTaskIDs = []string{"kept"}
			_, err := repo.QuerySidebarTaskPage(t.Context(), "scratch", query, prefs)
			if stage == "success" {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, injected)
			}
			assertSidebarScratchAbsent(t, repo)
			repo.sidebarQueryStage = nil
			_, err = repo.QuerySidebarTaskPage(t.Context(), "scratch", query, prefs)
			require.NoError(t, err, "next borrower must start with a clean temporary schema")
		})
	}
}

func TestSidebarQueryScratchCancellation(t *testing.T) {
	for _, stage := range []string{"before_start", "created", "indexed", "preferences", "page", "queue", "headers", "hydrated"} {
		t.Run(stage, func(t *testing.T) {
			repo := newSidebarReaderPool(t)
			repo.ro.SetMaxOpenConns(1)
			seedWorkspace(t, repo, "cancelled")
			ctx, cancel := context.WithCancel(t.Context())
			t.Cleanup(cancel)
			if stage == "before_start" {
				cancel()
			}
			repo.sidebarQueryStage = func(current string, _ *sqlx.Tx) error {
				if current == stage {
					cancel()
					return ctx.Err()
				}
				return nil
			}
			query := sidebarTaskQuery(1)
			query.Sort = models.SidebarTaskViewSort{Key: "color", Color: "red", Direction: "desc"}
			prefs := sidebarColorPreferencesForTest()
			prefs.PinnedTaskIDs = []string{"kept"}
			_, err := repo.QuerySidebarTaskPage(ctx, "cancelled", query, prefs)
			require.ErrorIs(t, err, context.Canceled)
			assertSidebarScratchAbsent(t, repo)
		})
	}
}

func TestSidebarQueryScratchDiscardsUncertainConnections(t *testing.T) {
	for _, failure := range []string{"cleanup_deadline", "rollback", "commit"} {
		t.Run(failure, func(t *testing.T) {
			repo := newSidebarReaderPool(t)
			repo.ro.SetMaxOpenConns(1)
			snapshot, err := beginSidebarQuerySnapshot(t.Context(), repo.ro)
			require.NoError(t, err)
			t.Cleanup(snapshot.close)
			_, _, err = snapshot.prepare(t.Context(), "sqlite3", "empty", sidebarTaskQuery(1))
			require.NoError(t, err)
			var original *sqlite3.SQLiteConn
			require.NoError(t, snapshot.conn.Raw(func(raw any) error {
				original = raw.(*sqlite3.SQLiteConn)
				original.RegisterAuthorizer(func(operation int, action, _, _ string) int {
					if operation == sqlite3.SQLITE_TRANSACTION && ((failure == "rollback" && action == "ROLLBACK") || (failure == "commit" && action == "COMMIT")) {
						return sqlite3.SQLITE_DENY
					}
					return sqlite3.SQLITE_OK
				})
				return nil
			}))
			if failure == "commit" {
				require.Error(t, snapshot.commit(t.Context()))
			}
			cleanupCtx, cancel := context.WithCancel(t.Context())
			if failure == "cleanup_deadline" {
				cancel()
			}
			snapshot.release(cleanupCtx)
			cancel()
			conn, err := repo.ro.Connx(t.Context())
			require.NoError(t, err)
			defer func() { _ = conn.Close() }()
			require.NoError(t, conn.Raw(func(raw any) error {
				require.NotSame(t, original, raw.(*sqlite3.SQLiteConn), "uncertain connection must be discarded")
				return nil
			}))
			var count int
			require.NoError(t, conn.QueryRowxContext(t.Context(), "SELECT COUNT(*) FROM sqlite_temp_master WHERE name IN ('kandev_sidebar_filtered', 'kandev_sidebar_preferences')").Scan(&count))
			require.Zero(t, count)
		})
	}
}

func TestSidebarQueryScratchWorkspaceIsolation(t *testing.T) {
	repo := newSidebarReaderPool(t)
	for _, id := range []string{"one", "two"} {
		seedWorkspace(t, repo, id)
		require.NoError(t, repo.CreateTask(t.Context(), &models.Task{ID: id, WorkspaceID: id, Title: id}))
	}
	var wg sync.WaitGroup
	errors := make(chan error, 12)
	for index := range 12 {
		wg.Go(func() {
			id := "one"
			if index%2 == 1 {
				id = "two"
			}
			page, err := repo.QuerySidebarTaskPage(t.Context(), id, sidebarTaskQuery(1), models.SidebarTaskViewPreferences{})
			if err == nil && (len(page.Tasks) != 1 || page.Tasks[0].ID != id) {
				err = fmt.Errorf("unexpected workspace page for %s", id)
			}
			errors <- err
		})
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
}

func TestSidebarQueryScratchKeepsReadSnapshotAndReadOnlyMain(t *testing.T) {
	repo := newSidebarReaderPool(t)
	seedWorkspace(t, repo, "snapshot")
	require.NoError(t, repo.CreateTask(t.Context(), &models.Task{ID: "original", WorkspaceID: "snapshot", Title: "Before"}))
	repo.sidebarQueryStage = func(stage string, tx *sqlx.Tx) error {
		if stage != "indexed" {
			return nil
		}
		_, err := tx.ExecContext(t.Context(), "UPDATE tasks SET title = 'Reader write' WHERE id = 'original'")
		require.Error(t, err, "the main database remains read-only")
		_, err = repo.db.ExecContext(t.Context(), "UPDATE tasks SET title = 'After' WHERE id = 'original'")
		return err
	}
	page, err := repo.QuerySidebarTaskPage(t.Context(), "snapshot", sidebarTaskQuery(1), models.SidebarTaskViewPreferences{})
	require.NoError(t, err)
	require.Len(t, page.Tasks, 1)
	require.Equal(t, "Before", page.Tasks[0].Title, "hydration uses the same WAL snapshot as candidate evaluation")
	var title string
	require.NoError(t, repo.db.GetContext(t.Context(), &title, "SELECT title FROM tasks WHERE id = 'original'"))
	require.Equal(t, "After", title, "the writer proceeds while the reader snapshot is open")
}

func assertSidebarScratchAbsent(t *testing.T, repo *Repository) {
	t.Helper()
	conn, err := repo.ro.Connx(t.Context())
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()
	var count int
	require.NoError(t, conn.GetContext(t.Context(), &count, "SELECT COUNT(*) FROM sqlite_temp_master WHERE name IN ('kandev_sidebar_filtered', 'kandev_sidebar_preferences', 'kandev_sidebar_task_colors')"))
	require.Zero(t, count)
	require.NoError(t, conn.GetContext(t.Context(), &count, "SELECT COUNT(*) FROM sqlite_temp_master WHERE name = 'sqlite_stat1'"))
	if count > 0 {
		require.NoError(t, conn.GetContext(t.Context(), &count, "SELECT COUNT(*) FROM temp.sqlite_stat1 WHERE tbl IN ('kandev_sidebar_filtered', 'kandev_sidebar_preferences', 'kandev_sidebar_task_colors')"))
		require.Zero(t, count, "scratch statistics must not retain workspace data for the next borrower")
	}
}

func TestSidebarQueryQueueHydrationKeepsPageSnapshot(t *testing.T) {
	repo := newSidebarReaderPool(t)
	seedWorkspace(t, repo, "queue-snapshot")
	for index, id := range []string{"first", "second", "visible"} {
		require.NoError(t, repo.CreateTask(t.Context(), &models.Task{
			ID: id, WorkspaceID: "queue-snapshot", Title: id,
		}))
		_, err := repo.db.ExecContext(t.Context(), `UPDATE tasks SET workflow_step_id = 'destination',
			queued_for_step_id = 'destination', wip_admitted = 0, position = ? WHERE id = ?`, index, id)
		require.NoError(t, err)
	}
	repo.sidebarQueryStage = func(stage string, _ *sqlx.Tx) error {
		if stage != "page" {
			return nil
		}
		_, err := repo.db.ExecContext(t.Context(), "UPDATE tasks SET wip_admitted = 1 WHERE id = 'first'")
		return err
	}
	query := sidebarTaskQuery(1)
	query.Filters = []models.SidebarTaskViewClause{{Dimension: "titleMatch", Op: "matches", Value: []byte(`"visible"`)}}
	page, err := repo.QuerySidebarTaskPage(t.Context(), "queue-snapshot", query, models.SidebarTaskViewPreferences{})
	require.NoError(t, err)
	require.Len(t, page.Tasks, 1)
	var queue [2]int
	for _, entry := range page.Entries {
		if entry.TaskID == "visible" {
			queue = [2]int{entry.WIPQueuePosition, entry.WIPQueueTotal}
		}
	}
	require.Equal(t, [2]int{3, 3}, queue, "queue hydration retains the page snapshot despite concurrent admission")
	var admitted int
	require.NoError(t, repo.db.GetContext(t.Context(), &admitted, "SELECT wip_admitted FROM tasks WHERE id = 'first'"))
	require.Equal(t, 1, admitted, "the writer advances independently of the read snapshot")
}
