package repository

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/workflow/models"
	"github.com/stretchr/testify/require"
)

func seedSelectionSteps(t *testing.T, repo *Repository, selected string) {
	t.Helper()
	for i, id := range []string{"a", "b"} {
		require.NoError(t, repo.CreateStep(context.Background(), &models.WorkflowStep{ID: id, WorkflowID: "wf-test", Name: id, Position: i, IsStartStep: id == selected}))
	}
}

func assertSelection(t *testing.T, repo *Repository, selected string) {
	t.Helper()
	for _, id := range []string{"a", "b"} {
		step, err := repo.GetStep(context.Background(), id)
		require.NoError(t, err)
		require.Equal(t, id == selected, step.IsStartStep, id)
	}
}

func openSelectionStore(t *testing.T, path string) *Repository {
	t.Helper()
	writer, err := db.OpenSQLite(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = writer.Close() })
	_, err = writer.Exec(`CREATE TABLE IF NOT EXISTS workflows (id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL DEFAULT '', workflow_template_id TEXT DEFAULT '', name TEXT NOT NULL, description TEXT DEFAULT '', created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL)`)
	require.NoError(t, err)
	_, err = writer.Exec(`INSERT OR IGNORE INTO workflows (id,name,created_at,updated_at) VALUES ('wf-test','Selection',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`)
	require.NoError(t, err)
	reader, err := db.OpenSQLiteReader(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = reader.Close() })
	repo, err := NewWithDB(sqlx.NewDb(writer, "sqlite3"), sqlx.NewDb(reader, "sqlite3"), nil)
	require.NoError(t, err)
	return repo
}

// @covers AC-TASKS-WORKFLOW-START-SELECTION-001.1, AC-TASKS-WORKFLOW-START-SELECTION-001.3
func TestWorkflowStartSelectionSQLiteIndependentStores(t *testing.T) {
	for _, initial := range []string{"a", "b"} {
		t.Run(initial, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "selection.db")
			first := openSelectionStore(t, path)
			second := openSelectionStore(t, path)
			seedSelectionSteps(t, first, initial)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			observed, err := second.GetStep(ctx, "a")
			require.NoError(t, err)
			observed.Name = "Renamed independently"
			holder, err := first.db.BeginTxx(ctx, nil)
			require.NoError(t, err)
			var workers sync.WaitGroup
			t.Cleanup(func() { _ = holder.Rollback(); cancel(); workers.Wait() })
			finished := make(chan error, 1)
			workers.Add(1)
			go func() {
				defer workers.Done()
				demoted, writeErr := second.UpdateStepWithDemotedStartStepsIntent(ctx, observed, nil)
				if writeErr == nil && len(demoted) != 0 {
					writeErr = fmt.Errorf("omitted update returned %d demoted steps", len(demoted))
				}
				finished <- writeErr
			}()
			waitSelectionBegin(t, ctx, finished)
			selected := "a"
			if initial == "a" {
				selected = "b"
			}
			_, err = holder.ExecContext(ctx, `UPDATE workflow_steps SET is_start_step=0 WHERE workflow_id='wf-test'`)
			require.NoError(t, err)
			_, err = holder.ExecContext(ctx, `UPDATE workflow_steps SET is_start_step=1 WHERE id=?`, selected)
			require.NoError(t, err)
			require.NoError(t, holder.Commit())
			require.NoError(t, <-finished)
			workers.Wait()
			assertSelection(t, first, selected)
			require.Equal(t, selected == "a", observed.IsStartStep)
			stored, err := first.GetStep(ctx, "a")
			require.NoError(t, err)
			require.Equal(t, observed.Name, stored.Name)
		})
	}
}

func waitSelectionBegin(t *testing.T, ctx context.Context, finished <-chan error) {
	t.Helper()
	for {
		buffer := make([]byte, 1<<20)
		for _, stack := range strings.Split(string(buffer[:runtime.Stack(buffer, true)]), "\n\n") {
			if strings.Contains(stack, "(*Repository).updateStepWithDemotedStartSteps(") && strings.Contains(stack, "(*SQLiteConn).begin(") {
				t.Log("independent SQLite store observed inside native BEGIN while holder owns writer")
				return
			}
		}
		select {
		case err := <-finished:
			t.Fatalf("update completed before held native writer release: %v", err)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		default:
			runtime.Gosched()
		}
	}
}

// @covers AC-TASKS-WORKFLOW-START-SELECTION-001.4
func TestWorkflowStartSelectionRepositoryRollback(t *testing.T) {
	t.Run("target write", func(t *testing.T) {
		runSelectionRollback(t, setupTestRepo(t), `CREATE TRIGGER reject_selection BEFORE UPDATE OF name ON workflow_steps WHEN NEW.id='b' BEGIN SELECT RAISE(ABORT,'injected target failure'); END`)
	})
	t.Run("flag capture", func(t *testing.T) {
		runSelectionRollback(t, setupTestRepo(t), `CREATE TRIGGER reject_selection AFTER UPDATE OF name ON workflow_steps WHEN NEW.id='b' BEGIN UPDATE workflow_steps SET is_start_step=NULL WHERE id=NEW.id; END`)
	})
}

func runSelectionRollback(t *testing.T, repo *Repository, trigger string) {
	t.Helper()
	ctx := context.Background()
	seedSelectionSteps(t, repo, "a")
	before, err := repo.ListStepsByWorkflow(ctx, "wf-test")
	require.NoError(t, err)
	_, err = repo.db.Exec(trigger)
	require.NoError(t, err)
	target, err := repo.GetStep(ctx, "b")
	require.NoError(t, err)
	promote := true
	demoted, err := repo.UpdateStepWithDemotedStartStepsIntent(ctx, target, &promote)
	require.Error(t, err)
	require.Empty(t, demoted)
	after, err := repo.ListStepsByWorkflow(ctx, "wf-test")
	require.NoError(t, err)
	require.Equal(t, before, after)
	target.ID = "missing"
	demoted, err = repo.UpdateStepWithDemotedStartStepsIntent(ctx, target, &promote)
	require.Error(t, err)
	require.Empty(t, demoted)
	after, err = repo.ListStepsByWorkflow(ctx, "wf-test")
	require.NoError(t, err)
	require.Equal(t, before, after)
}

// @covers AC-TASKS-WORKFLOW-START-SELECTION-001.6
func TestWorkflowStartSelectionLegacyAndExact(t *testing.T) {
	repo := setupTestRepo(t)
	runSelectionLegacyAndExact(t, repo)
}

func runSelectionLegacyAndExact(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	seedSelectionSteps(t, repo, "a")
	a, err := repo.GetStep(ctx, "a")
	require.NoError(t, err)
	a.IsStartStep = false
	require.NoError(t, repo.UpdateStep(ctx, a))
	assertSelection(t, repo, "")
	a.IsStartStep = true
	_, err = repo.UpdateStepWithDemotedStartSteps(ctx, a)
	require.NoError(t, err)
	assertSelection(t, repo, "a")
	b, err := repo.GetStep(ctx, "b")
	require.NoError(t, err)
	wfVersion := time.Now().UTC()
	_, err = repo.db.ExecContext(ctx, repo.db.Rebind(`UPDATE workflows SET updated_at=? WHERE id='wf-test'`), wfVersion)
	require.NoError(t, err)
	require.NoError(t, repo.db.QueryRowContext(ctx, `SELECT updated_at FROM workflows WHERE id='wf-test'`).Scan(&wfVersion))
	before, err := repo.ListStepsByWorkflow(ctx, "wf-test")
	require.NoError(t, err)
	b.IsStartStep = true
	currentStepVersion := b.UpdatedAt
	for _, versions := range [][2]time.Time{{wfVersion.Add(-time.Hour), b.UpdatedAt}, {wfVersion, b.UpdatedAt.Add(-time.Hour)}} {
		demoted, writeErr := repo.UpdateStepWithDemotedStartStepsIfUnchanged(ctx, b, versions[0], versions[1])
		require.ErrorIs(t, writeErr, repoerrors.ErrTaskVersionConflict)
		require.Empty(t, demoted)
		after, getErr := repo.ListStepsByWorkflow(ctx, "wf-test")
		require.NoError(t, getErr)
		require.Equal(t, before, after)
	}
	_, err = repo.UpdateStepWithDemotedStartStepsIfUnchanged(ctx, b, wfVersion, currentStepVersion)
	require.NoError(t, err)
	assertSelection(t, repo, "b")
}
