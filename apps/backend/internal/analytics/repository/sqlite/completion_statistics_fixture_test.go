package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/analytics/models"
	"github.com/kandev/kandev/internal/db"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
	workflowrepo "github.com/kandev/kandev/internal/workflow/repository"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type completionFixture struct {
	t         *testing.T
	ctx       context.Context
	writer    *sqlx.DB
	tasks     *tasksqlite.Repository
	workflows *workflowrepo.Repository
	stats     *Repository
	today     time.Time
	middle    *workflowmodels.WorkflowStep
	final     *workflowmodels.WorkflowStep
}

func newCompletionFixture(t *testing.T) *completionFixture {
	t.Helper()
	raw, err := db.OpenSQLite(filepath.Join(t.TempDir(), "completion.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, raw.Close()) })
	writer := sqlx.NewDb(raw, "sqlite3")
	tasks, err := tasksqlite.NewWithDB(writer, writer, nil)
	require.NoError(t, err)
	workflows, err := workflowrepo.NewWithDB(writer, writer, nil)
	require.NoError(t, err)
	stats, err := NewWithDB(writer, writer)
	require.NoError(t, err)
	f := &completionFixture{t: t, ctx: context.Background(), writer: writer,
		tasks: tasks, workflows: workflows, stats: stats, today: time.Now().UTC().Truncate(24 * time.Hour)}
	for _, id := range []string{"main", "other"} {
		require.NoError(t, tasks.CreateWorkspace(f.ctx, &taskmodels.Workspace{ID: id, Name: id}))
	}
	require.NoError(t, tasks.CreateWorkflow(f.ctx, &taskmodels.Workflow{ID: "flow", WorkspaceID: "main", Name: "Completion"}))
	f.middle = &workflowmodels.WorkflowStep{ID: "middle", WorkflowID: "flow", Name: "Review", Position: 1, CompleteTaskOnEnter: true}
	f.final = &workflowmodels.WorkflowStep{ID: "final", WorkflowID: "flow", Name: "Done", Position: 2}
	for _, step := range []*workflowmodels.WorkflowStep{
		{ID: "first", WorkflowID: "flow", Name: "Start", Position: 0}, f.middle, f.final,
	} {
		require.NoError(t, workflows.CreateStep(f.ctx, step))
	}
	f.assertOrder("final", false)
	for _, id := range []string{"a", "b", "deleted", "empty"} {
		repository := &taskmodels.Repository{ID: id, WorkspaceID: "main", Name: id, SourceType: "local"}
		if id == "deleted" {
			repository.DeletedAt = &f.today
		}
		require.NoError(t, tasks.CreateRepository(f.ctx, repository))
	}
	return f
}

func (f *completionFixture) assertOrder(finalID string, completes bool) {
	f.t.Helper()
	steps, err := f.workflows.ListStepsByWorkflow(f.ctx, "flow")
	require.NoError(f.t, err)
	require.Len(f.t, steps, 3)
	last := steps[len(steps)-1]
	require.Equal(f.t, finalID, last.ID)
	require.Equal(f.t, completes, workflowmodels.IsTerminalStep(last, nil))
	require.False(f.t, workflowmodels.IsTerminalStep(steps[1], last))
}

func (f *completionFixture) task(id string, state v1.TaskState, step string) *taskmodels.Task {
	f.t.Helper()
	task := &taskmodels.Task{ID: id, WorkspaceID: "main", WorkflowID: "flow", WorkflowStepID: step,
		Title: id, State: state, Metadata: map[string]interface{}{"sentinel": "preserved"}}
	require.NoError(f.t, f.tasks.CreateTask(f.ctx, task))
	f.link(id, "a")
	return task
}

func (f *completionFixture) link(taskID, repositoryID string) {
	f.t.Helper()
	require.NoError(f.t, f.tasks.CreateTaskRepository(f.ctx, &taskmodels.TaskRepository{
		ID: taskID + "-" + repositoryID, TaskID: taskID, RepositoryID: repositoryID,
	}))
}

func (f *completionFixture) session(id, taskID, repositoryID string, started time.Time, completed *time.Time) {
	f.t.Helper()
	require.NoError(f.t, f.tasks.CreateTaskSession(f.ctx, &taskmodels.TaskSession{
		ID: id, TaskID: taskID, RepositoryID: repositoryID, StartedAt: started,
		CompletedAt: completed, State: taskmodels.TaskSessionStateCompleted,
	}))
}

func (f *completionFixture) archive(id string, when time.Time) {
	f.t.Helper()
	require.NoError(f.t, f.tasks.ArchiveTask(f.ctx, id))
	// Archive's writer uses wall time; only backdate its timestamp for bucket fixtures.
	_, err := f.writer.Exec(`UPDATE tasks SET archived_at = ? WHERE id = ?`, when, id)
	require.NoError(f.t, err)
}

func (f *completionFixture) activity(days int) map[string]int {
	f.t.Helper()
	rows, err := f.stats.GetCompletedTaskActivity(f.ctx, "main", days)
	require.NoError(f.t, err)
	require.Len(f.t, rows, days)
	result := make(map[string]int, days)
	for i, row := range rows {
		expected := f.today.AddDate(0, 0, i-days+1).Format(time.DateOnly)
		assert.Equal(f.t, expected, row.Date)
		result[row.Date] = row.CompletedTasks
	}
	return result
}

func (f *completionFixture) repositories(start *time.Time) map[string]*models.RepositoryStats {
	f.t.Helper()
	rows, err := f.stats.GetRepositoryStats(f.ctx, "main", start)
	require.NoError(f.t, err)
	result := make(map[string]*models.RepositoryStats, len(rows))
	for _, row := range rows {
		result[row.RepositoryID] = row
	}
	require.Len(f.t, result, 3)
	require.Contains(f.t, result, "a")
	require.Contains(f.t, result, "b")
	require.Contains(f.t, result, "empty")
	return result
}

func (f *completionFixture) assertCounts(completed int) {
	f.t.Helper()
	global, err := f.stats.GetGlobalStats(f.ctx, "main", nil)
	require.NoError(f.t, err)
	assert.Equal(f.t, completed, global.CompletedTasks, "global completion")
	assert.Equal(f.t, completed, f.repositories(nil)["a"].CompletedTasks, "repository completion")
	total := 0
	for _, count := range f.activity(7) {
		total += count
	}
	assert.Equal(f.t, completed, total, "completed activity")
}
