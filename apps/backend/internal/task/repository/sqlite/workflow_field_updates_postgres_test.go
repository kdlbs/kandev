package sqlite_test

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	internaldb "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/testutil"
)

func workflowPostgresPair(t *testing.T) (*tasksqlite.Repository, *tasksqlite.Repository, *sqlx.DB) {
	t.Helper()
	dsn := testutil.PostgresDSNFromEnv(t)
	first := testutil.OpenIsolatedPostgres(t, dsn)
	a, err := tasksqlite.NewWithDB(first, first, nil)
	require.NoError(t, err)
	var schema string
	require.NoError(t, first.QueryRow(`SELECT current_schema()`).Scan(&schema))
	open := func() *sqlx.DB {
		raw, err := internaldb.OpenPostgres(hierarchySchemaDSN(dsn, schema), 1, 1)
		require.NoError(t, err)
		db := sqlx.NewDb(raw, "pgx")
		t.Cleanup(func() { require.NoError(t, db.Close()) })
		require.NoError(t, db.Ping())
		return db
	}
	second, observer := open(), open()
	return a, tasksqlite.NewWithInitializedDB(second, second, nil), observer
}

func waitWorkflowRowLock(t *testing.T, ctx context.Context, observer *sqlx.DB, pid int) {
	t.Helper()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var waiting bool
		err := observer.QueryRowContext(ctx, `SELECT EXISTS (
			SELECT 1 FROM pg_locks WHERE pid=$1 AND locktype='transactionid' AND NOT granted
		)`, pid).Scan(&waiting)
		require.NoError(t, err)
		if waiting {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("workflow writer never physically waited on the row", ctx.Err())
		case <-ticker.C:
		}
	}
}

// @covers AC-TASKS-FIELD-UPDATES-002.1
func TestPostgresWorkflowFieldUpdatesPhysicalConcurrency(t *testing.T) {
	for held := range 2 {
		t.Run(fmt.Sprintf("held_field_%d", held), func(t *testing.T) {
			a, b, observer := workflowPostgresPair(t)
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			require.NoError(t, a.CreateWorkflow(ctx, &models.Workflow{ID: "physical", WorkspaceID: "ws", Name: "before name", Prompt: "before prompt"}))
			pidA, pidB := hierarchyBackendPID(t, a.DB()), hierarchyBackendPID(t, b.DB())
			require.NotEqual(t, pidA, pidB)
			tx, err := a.DB().BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
			require.NoError(t, err)
			var workers sync.WaitGroup
			defer func() { _ = tx.Rollback(); cancel(); workers.Wait() }()
			_, err = tx.ExecContext(ctx, `SELECT id FROM workflows WHERE id='physical' FOR UPDATE`)
			require.NoError(t, err)
			name, prompt := "saved name", "saved prompt"
			updates := [2]models.WorkflowFieldUpdate{{Name: &name}, {Prompt: &prompt}}
			type outcome struct {
				row *models.Workflow
				err error
			}
			done := make(chan outcome, 1)
			workers.Go(func() { row, err := b.UpdateWorkflowFields(ctx, "physical", updates[held]); done <- outcome{row, err} })
			waitWorkflowRowLock(t, ctx, observer, pidB)
			t.Logf("independent backend PIDs %d / %d; writer physically waiting on workflow row", pidA, pidB)
			if held == 0 {
				_, err = tx.ExecContext(ctx, `UPDATE workflows SET prompt=$1,updated_at=$2 WHERE id='physical'`, prompt, time.Now().UTC())
			} else {
				_, err = tx.ExecContext(ctx, `UPDATE workflows SET name=$1,updated_at=$2 WHERE id='physical'`, name, time.Now().UTC())
			}
			require.NoError(t, err)
			require.NoError(t, tx.Commit())
			result := <-done
			require.NoError(t, result.err)
			require.Equal(t, name, result.row.Name)
			require.Equal(t, prompt, result.row.Prompt)
			stored, err := a.GetWorkflow(ctx, "physical")
			require.NoError(t, err)
			require.Equal(t, result.row, stored)
		})
	}
}

// @covers AC-TASKS-FIELD-UPDATES-002.2, AC-TASKS-FIELD-UPDATES-002.3, AC-TASKS-FIELD-UPDATES-002.5
func TestPostgresWorkflowFieldUpdatesCompatibility(t *testing.T) {
	a, b, _ := workflowPostgresPair(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	seed := &models.Workflow{ID: "compatibility", WorkspaceID: "ws", Name: "before", Prompt: "before prompt", Hidden: true, Source: models.WorkflowSourceGitHub, SourcePath: "before.yaml"}
	require.NoError(t, a.CreateWorkflow(ctx, seed))
	before, err := a.GetWorkflow(ctx, seed.ID)
	require.NoError(t, err)
	empty, hidden := "", false
	updated, err := b.UpdateWorkflowFields(ctx, seed.ID, models.WorkflowFieldUpdate{
		Prompt: &empty, AgentProfileID: &empty, Hidden: &hidden, Source: &empty, SourcePath: &empty,
	})
	require.NoError(t, err)
	require.Empty(t, updated.Prompt)
	require.Empty(t, updated.AgentProfileID)
	require.False(t, updated.Hidden)
	require.Equal(t, models.WorkflowSourceManual, updated.Source)
	require.Empty(t, updated.SourcePath)
	require.Equal(t, before.Name, updated.Name)
	require.ErrorIs(t, a.UpdateWorkflowIfUnchanged(ctx, before, before.UpdatedAt), repoerrors.ErrTaskVersionConflict)
	updated.Name = "exact name"
	require.NoError(t, a.UpdateWorkflowIfUnchanged(ctx, updated, updated.UpdatedAt))
	exact, err := a.GetWorkflow(ctx, seed.ID)
	require.NoError(t, err)
	require.Equal(t, "exact name", exact.Name)
	_, err = a.DB().ExecContext(ctx, `ALTER TABLE workflows ADD CONSTRAINT reject_workflow_prompt CHECK (prompt <> 'rejected prompt')`)
	require.NoError(t, err)
	name, prompt := "rejected name", "rejected prompt"
	_, err = b.UpdateWorkflowFields(ctx, seed.ID, models.WorkflowFieldUpdate{Name: &name, Prompt: &prompt})
	require.Error(t, err)
	stored, err := a.GetWorkflow(ctx, seed.ID)
	require.NoError(t, err)
	require.Equal(t, exact, stored, "constraint-aborted statement preserves all fields and timestamp")
}
