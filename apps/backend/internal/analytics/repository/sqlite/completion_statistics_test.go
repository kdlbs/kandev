package sqlite

import (
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// @covers AC-TASKS-COMPLETION-STATS-001.1, AC-TASKS-COMPLETION-STATS-001.3, AC-TASKS-COMPLETION-STATS-001.8
func TestCompletionStatisticsCommittedState(t *testing.T) {
	cases := []struct {
		name    string
		state   v1.TaskState
		step    string
		checked bool
		want    int
	}{
		{"unchecked_final_todo", v1.TaskStateTODO, "final", false, 0},
		{"checked_final_completed", v1.TaskStateCompleted, "final", true, 1},
		{"checked_final_todo", v1.TaskStateTODO, "final", true, 0},
		{"final_in_progress", v1.TaskStateInProgress, "final", false, 0},
		{"final_failed", v1.TaskStateFailed, "final", true, 0},
		{"final_cancelled", v1.TaskStateCancelled, "final", true, 0},
		{"checked_nonfinal_todo", v1.TaskStateTODO, "middle", false, 0},
		{"completed_elsewhere", v1.TaskStateCompleted, "middle", false, 1},
		{"completed_without_step", v1.TaskStateCompleted, "", false, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newCompletionFixture(t)
			f.final.CompleteTaskOnEnter = tc.checked
			require.NoError(t, f.workflows.UpdateStep(f.ctx, f.final))
			f.assertOrder("final", tc.checked)
			task := f.task("task", tc.state, tc.step)
			done := f.today.Add(-12 * time.Hour)
			f.session("session", task.ID, "a", done.Add(-time.Hour), &done)
			before, err := f.tasks.GetTask(f.ctx, task.ID)
			require.NoError(t, err)
			require.Equal(t, tc.state, before.State)
			f.assertCounts(tc.want)
			after, err := f.tasks.GetTask(f.ctx, task.ID)
			require.NoError(t, err)
			assert.Equal(t, before, after, "statistics must not mutate persisted task")
		})
	}
}

// @covers AC-TASKS-COMPLETION-STATS-001.2, AC-TASKS-COMPLETION-STATS-001.8
func TestCompletionStatisticsWorkflowEdits(t *testing.T) {
	for _, state := range []v1.TaskState{v1.TaskStateTODO, v1.TaskStateCompleted} {
		t.Run(string(state), func(t *testing.T) {
			f := newCompletionFixture(t)
			task := f.task("resident", state, "middle")
			done := f.today.Add(-12 * time.Hour)
			f.session("session", task.ID, "a", done.Add(-time.Hour), &done)
			before, err := f.tasks.GetTask(f.ctx, task.ID)
			require.NoError(t, err)
			want := 0
			if state == v1.TaskStateCompleted {
				want = 1
			}
			f.assertCounts(want)
			f.middle.Position = 3
			f.middle.Name = "Completed"
			require.NoError(t, f.workflows.UpdateStep(f.ctx, f.middle))
			f.assertOrder("middle", true)
			f.assertCounts(want)
			f.middle.CompleteTaskOnEnter = false
			f.middle.Name = "Unfinished"
			require.NoError(t, f.workflows.UpdateStep(f.ctx, f.middle))
			f.assertOrder("middle", false)
			f.assertCounts(want)
			after, err := f.tasks.GetTask(f.ctx, task.ID)
			require.NoError(t, err)
			assert.Equal(t, state, after.State)
			assert.Equal(t, "middle", after.WorkflowStepID)
			assert.Equal(t, before, after, "workflow edits and statistics preserve resident task")
		})
	}
}

// @covers AC-TASKS-COMPLETION-STATS-001.4, AC-TASKS-COMPLETION-STATS-001.5, AC-TASKS-COMPLETION-STATS-001.6, AC-TASKS-COMPLETION-STATS-001.7
func TestCompletionStatisticsCompatibility(t *testing.T) {
	f := newCompletionFixture(t)
	yesterday := f.today.Add(-12 * time.Hour)
	f.task("shared", v1.TaskStateCompleted, "middle")
	f.link("shared", "b")
	f.link("shared", "deleted")
	f.session("shared-session", "shared", "b", yesterday.Add(-time.Hour), &yesterday)
	f.task("unfinished", v1.TaskStateTODO, "final")
	for _, state := range []v1.TaskState{v1.TaskStateTODO, v1.TaskStateFailed, v1.TaskStateCancelled, v1.TaskStateCompleted} {
		id := "archived-" + string(state)
		f.task(id, state, "middle")
		f.archive(id, yesterday)
	}
	seedCompletionExclusions(f, yesterday)
	f.task("old", v1.TaskStateCompleted, "middle")
	// Creation writers stamp now. Backdate only the clock under range test.
	_, err := f.writer.Exec(`UPDATE tasks SET created_at = ? WHERE id = ?`, f.today.Add(-48*time.Hour), "old")
	require.NoError(t, err)
	f.session("old-session", "old", "a", f.today, &f.today)
	for _, commit := range []*taskmodels.SessionCommit{
		{SessionID: "shared-session", CommitSHA: "new", CommittedAt: f.today, FilesChanged: 2, Insertions: 3, Deletions: 1},
		{SessionID: "old-session", CommitSHA: "old", CommittedAt: yesterday, FilesChanged: 8},
	} {
		inserted, createErr := f.tasks.CreateSessionCommit(f.ctx, commit)
		require.NoError(t, createErr)
		require.True(t, inserted)
	}
	f.assertCounts(6)
	global, err := f.stats.GetGlobalStats(f.ctx, "main", &f.today)
	require.NoError(t, err)
	assert.Equal(t, 6, global.TotalTasks)
	assert.Equal(t, 5, global.CompletedTasks)
	assert.Equal(t, 1, global.TotalSessions, "session clock is independent of task creation")
	repos := f.repositories(&f.today)
	assert.Equal(t, 6, repos["a"].TotalTasks)
	assert.Equal(t, 5, repos["a"].CompletedTasks)
	assert.Equal(t, 1, repos["a"].SessionCount)
	assert.Zero(t, repos["a"].TotalCommits)
	assert.Equal(t, 1, repos["b"].TotalTasks)
	assert.Equal(t, 1, repos["b"].CompletedTasks)
	assert.Zero(t, repos["b"].SessionCount)
	assert.Equal(t, 1, repos["b"].TotalCommits, "commit uses commit time and session repository")
	assert.Equal(t, 2, repos["b"].TotalFilesChanged)
	assert.Equal(t, 3, repos["b"].TotalInsertions)
	assert.Equal(t, 1, repos["b"].TotalDeletions)
	assert.Zero(t, repos["empty"].TotalTasks)
	assert.Zero(t, repos["empty"].CompletedTasks)
}

func seedCompletionExclusions(f *completionFixture, completed time.Time) {
	f.t.Helper()
	for _, task := range []*taskmodels.Task{
		{ID: "ephemeral", WorkspaceID: "main", IsEphemeral: true},
		{ID: "automation", WorkspaceID: "main", Origin: taskmodels.TaskOriginAutomationRun},
		{ID: "other-task", WorkspaceID: "other"},
	} {
		task.Title = task.ID
		task.State = v1.TaskStateCompleted
		require.NoError(f.t, f.tasks.CreateTask(f.ctx, task))
		if task.WorkspaceID == "main" {
			f.link(task.ID, "a")
		}
		f.session(task.ID+"-session", task.ID, "", completed.Add(-time.Hour), &completed)
	}
}

// @covers AC-TASKS-COMPLETION-STATS-001.3, AC-TASKS-COMPLETION-STATS-001.6, AC-TASKS-COMPLETION-STATS-001.7
func TestCompletionStatisticsTimestampBuckets(t *testing.T) {
	f := newCompletionFixture(t)
	older := f.today.Add(-60 * time.Hour)
	latest := f.today.Add(-36 * time.Hour)
	archiveTime := f.today.Add(-12 * time.Hour)
	for _, id := range []string{"latest-session", "archive-only", "no-time", "out-of-horizon"} {
		f.task(id, v1.TaskStateCompleted, "middle")
	}
	f.session("older", "latest-session", "a", older.Add(-time.Hour), &older)
	f.session("latest", "latest-session", "a", latest.Add(-time.Hour), &latest)
	f.archive("latest-session", archiveTime)
	f.session("unfinished-session", "archive-only", "a", archiveTime, nil)
	f.archive("archive-only", archiveTime)
	outside := f.today.AddDate(0, 0, -7)
	f.session("outside", "out-of-horizon", "a", outside.Add(-time.Hour), &outside)
	global, err := f.stats.GetGlobalStats(f.ctx, "main", nil)
	require.NoError(t, err)
	assert.Equal(t, 4, global.CompletedTasks)
	assert.Equal(t, 4, f.repositories(nil)["a"].CompletedTasks)
	buckets := f.activity(7)
	for date, count := range buckets {
		want := 0
		if date == latest.Format(time.DateOnly) || date == archiveTime.Format(time.DateOnly) {
			want = 1
		}
		assert.Equal(t, want, count, date)
	}
}

func TestCompletionStatisticsQueryDialects(t *testing.T) {
	for _, driver := range []string{"sqlite3", "pgx"} {
		t.Run(driver, func(t *testing.T) {
			query := sqlx.Rebind(sqlx.BindType(driver), buildRepositoryStatsQuery(driver))
			assert.Contains(t, query, "scope.state = 'COMPLETED'")
			assert.Contains(t, query, "scope.archived_at IS NOT NULL")
			assert.NotContains(t, query, "workflow_steps")
			assert.NotContains(t, query, "workflow_step_id")
			if driver == "pgx" {
				assert.Contains(t, query, "$1")
				assert.False(t, strings.Contains(query, "?"))
			}
		})
	}
}
