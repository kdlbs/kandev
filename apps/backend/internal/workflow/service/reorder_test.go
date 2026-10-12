package service

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/workflow/models"
	"github.com/kandev/kandev/internal/workflow/repository"
)

// @covers AC-TASKS-WORKFLOW-STEP-ORDERING-001.3
func TestReorderStepsRollsBackSecondWriteFailure(t *testing.T) {
	svc, database := setupTestService(t)
	ctx := context.Background()
	insertWorkflow(t, database, "wf", "Workflow")
	for position, id := range []string{"a", "b"} {
		require.NoError(t, svc.CreateStep(ctx, &models.WorkflowStep{ID: id, WorkflowID: "wf", Name: id, Position: position}))
	}
	before, err := svc.ListStepsByWorkflow(ctx, "wf")
	require.NoError(t, err)
	_, err = database.Exec(`CREATE TRIGGER reject_second_reorder BEFORE UPDATE OF position ON workflow_steps WHEN NEW.id = 'a' BEGIN SELECT RAISE(ABORT, 'injected second write failure'); END`)
	require.NoError(t, err)
	require.Error(t, svc.ReorderSteps(ctx, "wf", []string{"b", "a"}))
	b, err := svc.GetStep(ctx, "b")
	require.NoError(t, err)
	require.Equal(t, 1, b.Position, "failed reorder must retain the first row's original position")
	after, err := svc.ListStepsByWorkflow(ctx, "wf")
	require.NoError(t, err)
	require.Equal(t, before, after)
}

// @covers AC-TASKS-WORKFLOW-STEP-ORDERING-001.2, AC-TASKS-COMPLETION-001.4
func TestReorderStepsPreservesConcurrentEdit(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "workflow.db") + "?_journal_mode=WAL&_busy_timeout=5000"
	writer, err := sqlx.Open("sqlite3", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = writer.Close() })
	writer.SetMaxOpenConns(1)
	reader, err := sqlx.Open("sqlite3", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = reader.Close() })
	_, err = writer.Exec(`CREATE TABLE workflows (id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL DEFAULT '', workflow_template_id TEXT DEFAULT '', name TEXT NOT NULL, description TEXT DEFAULT '', created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL)`)
	require.NoError(t, err)
	repo, err := repository.NewWithDB(writer, reader, nil)
	require.NoError(t, err)
	insertWorkflow(t, writer, "wf", "Workflow")
	base, _ := setupTestService(t)
	svc := NewService(repo, base.logger)
	t.Cleanup(func() { _ = svc.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for position, id := range []string{"a", "b"} {
		require.NoError(t, svc.CreateStep(ctx, &models.WorkflowStep{ID: id, WorkflowID: "wf", Name: id, Position: position, Prompt: "original"}))
	}
	seeded, err := svc.ListStepsByWorkflow(ctx, "wf")
	require.NoError(t, err)
	require.Len(t, seeded, 2)
	connection, err := writer.Conn(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = connection.Close() })
	before := writer.Stats().WaitCount
	finished := make(chan error, 1)
	go func() { finished <- svc.ReorderSteps(ctx, "wf", []string{"b", "a"}) }()
	for writer.Stats().WaitCount == before {
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		default:
			runtime.Gosched()
		}
	}
	_, err = reader.ExecContext(ctx, `UPDATE workflow_steps SET prompt = 'edited concurrently', agent_profile_id = 'edited-profile', color = '#123456', complete_task_on_enter = 1 WHERE id = 'b'`)
	require.NoError(t, err)
	require.NoError(t, connection.Close())
	require.NoError(t, <-finished)
	b, err := svc.GetStep(ctx, "b")
	require.NoError(t, err)
	require.Equal(t, "edited concurrently", b.Prompt)
	require.True(t, b.CompleteTaskOnEnter)
	require.Equal(t, "edited-profile", b.AgentProfileID)
	require.Equal(t, "#123456", b.Color)
	require.Equal(t, 0, b.Position)
}

// @covers AC-TASKS-WORKFLOW-STEP-ORDERING-001.4, AC-TASKS-WORKFLOW-STEP-ORDERING-001.5
func TestReorderStepsRejectsInvalidMembership(t *testing.T) {
	for name, ids := range map[string][]string{
		"duplicate": {"b", "b"}, "omitted": {"b"}, "empty": {},
		"missing": {"b", "missing"}, "foreign": {"b", "foreign"},
	} {
		t.Run(name, func(t *testing.T) {
			svc, database := setupTestService(t)
			ctx := context.Background()
			insertWorkflow(t, database, "wf", "Workflow")
			insertWorkflow(t, database, "other", "Other")
			for position, id := range []string{"a", "b"} {
				require.NoError(t, svc.CreateStep(ctx, &models.WorkflowStep{ID: id, WorkflowID: "wf", Name: id, Position: position}))
			}
			require.NoError(t, svc.CreateStep(ctx, &models.WorkflowStep{ID: "foreign", WorkflowID: "other", Name: "Other", Position: 7}))
			before, err := svc.ListStepsByWorkflow(ctx, "wf")
			require.NoError(t, err)
			err = svc.ReorderSteps(ctx, "wf", ids)
			require.Error(t, err)
			if name == "foreign" || name == "missing" {
				require.ErrorIs(t, err, ErrNotVisible)
			}
			after, err := svc.ListStepsByWorkflow(ctx, "wf")
			require.NoError(t, err)
			require.Equal(t, before, after)
			foreign, err := svc.GetStep(ctx, "foreign")
			require.NoError(t, err)
			require.Equal(t, 7, foreign.Position)
		})
	}
}
