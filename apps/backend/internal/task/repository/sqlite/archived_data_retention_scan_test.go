package sqlite

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
)

func cleanupJobScanFixture(t *testing.T) *sqlx.DB {
	t.Helper()
	d, err := sqlx.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, d.Close()) })
	d.SetMaxOpenConns(1)
	_, err = d.Exec(`
	CREATE TABLE task_resource_cleanup_jobs(id TEXT PRIMARY KEY,task_id TEXT NOT NULL,state TEXT NOT NULL,
	 completed_at TIMESTAMP,resource_snapshot TEXT NOT NULL DEFAULT '{}',workspace_protection TEXT NOT NULL DEFAULT 'unclassified');
	CREATE TABLE task_session_messages(id TEXT PRIMARY KEY,task_session_id TEXT,metadata TEXT,
	 task_id TEXT DEFAULT 'task',author_type TEXT DEFAULT 'agent',type TEXT DEFAULT 'tool_execute');
	INSERT INTO task_resource_cleanup_jobs VALUES('job','task','succeeded','2020-01-01','{"progress":"x"}','');
	`)
	require.NoError(t, err)
	return d
}

// A succeeded job is reduced only while no row of its task is still read by the
// workspace protection inventory (retention.LoadForTask).
func TestSucceededCleanupJobEligibleKeepsProtectedTasks(t *testing.T) {
	cutoff := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, tt := range []struct {
		name, mutation string
		want           bool
	}{
		{"ordinary succeeded job", "", true},
		{"recent completion", `UPDATE task_resource_cleanup_jobs SET completed_at='2026-06-01'`, false},
		{"not succeeded", `UPDATE task_resource_cleanup_jobs SET state='failed'`, false},
		{"retained workspace", `UPDATE task_resource_cleanup_jobs SET workspace_protection='retain'`, false},
		{"unknown protection", `UPDATE task_resource_cleanup_jobs SET workspace_protection='unknown'`, false},
		{"unclassified protection", `UPDATE task_resource_cleanup_jobs SET workspace_protection='unclassified'`, false},
		{"finished cleanup protection", `UPDATE task_resource_cleanup_jobs SET workspace_protection='cleanup'`, true},
		{"sibling retains workspace", `INSERT INTO task_resource_cleanup_jobs VALUES('other','task','succeeded','2020-01-01','{}','retain')`, false},
		{"sibling cleanup unfinished", `INSERT INTO task_resource_cleanup_jobs VALUES('other','task','running',NULL,'{}','cleanup')`, false},
		{"sibling cleanup finished", `INSERT INTO task_resource_cleanup_jobs VALUES('other','task','cancelled',NULL,'{}','cleanup')`, true},
		{"other task retains", `INSERT INTO task_resource_cleanup_jobs VALUES('other','task-2','succeeded','2020-01-01','{}','retain')`, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			d := cleanupJobScanFixture(t)
			if tt.mutation != "" {
				_, err := d.Exec(tt.mutation)
				require.NoError(t, err)
			}
			got, err := SucceededCleanupJobEligible(context.Background(), d, "sqlite3", "job", cutoff)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

// A body is loaded only while it still has the measured size, so a read outside
// the writer never pulls more than the bytes the batch admitted.
func TestRetentionBodyReadsRequireMeasuredSize(t *testing.T) {
	ctx := context.Background()
	d := cleanupJobScanFixture(t)
	size, err := CleanupJobSnapshotBytes(ctx, d, "job")
	require.NoError(t, err)
	body, err := CleanupJobSnapshotBody(ctx, d, "job", size)
	require.NoError(t, err)
	require.Equal(t, `{"progress":"x"}`, body)
	_, err = CleanupJobSnapshotBody(ctx, d, "job", size-1)
	require.ErrorIs(t, err, sql.ErrNoRows)

	_, err = d.Exec(`INSERT INTO task_session_messages(id,task_session_id,metadata) VALUES('m','s','{"a":1}')`)
	require.NoError(t, err)
	rows, err := NextArchivedMessageRows(ctx, d, "s", 0, 10, 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	raw, err := ArchivedMessageMetadata(ctx, d, rows[0].RowID, "m", rows[0].Bytes)
	require.NoError(t, err)
	require.Equal(t, `{"a":1}`, raw)
	_, err = d.Exec(`UPDATE task_session_messages SET metadata='{"a":12345}'`)
	require.NoError(t, err)
	_, err = ArchivedMessageMetadata(ctx, d, rows[0].RowID, "m", rows[0].Bytes)
	require.ErrorIs(t, err, sql.ErrNoRows)
}

func TestHistoryUpperMessageRowIDCapturesPassStart(t *testing.T) {
	ctx := context.Background()
	d := cleanupJobScanFixture(t)
	upper, err := HistoryUpperMessageRowID(ctx, d)
	require.NoError(t, err)
	require.Zero(t, upper)
	_, err = d.Exec(`INSERT INTO task_session_messages(id,task_session_id,metadata) VALUES('m1','s','{}'),('m2','s2','{}')`)
	require.NoError(t, err)
	upper, err = HistoryUpperMessageRowID(ctx, d)
	require.NoError(t, err)
	require.EqualValues(t, 2, upper)
}

// Message metadata is reduced only for a task archived by the cutoff or a
// terminal task that is inactive at the cutoff; an active, unarchived or
// recently touched task keeps its history.
func TestHistoryTaskEligibleRequiresArchivedOrFinished(t *testing.T) {
	cutoff := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, tt := range []struct {
		name, mutation string
		want           bool
	}{
		{"finished and inactive", "", true},
		{"active task", `UPDATE tasks SET state='IN_PROGRESS'`, false},
		{"live session", `UPDATE task_sessions SET state='RUNNING'`, false},
		{"recently touched", `UPDATE tasks SET updated_at='2026-02-01'`, false},
		{"archived by cutoff", `UPDATE tasks SET state='IN_PROGRESS',archived_at='2025-06-01T00:00:00Z'`, true},
		{"archived at cutoff", `UPDATE tasks SET state='IN_PROGRESS',archived_at='2026-01-01T00:00:00Z'`, true},
		{"archived after cutoff", `UPDATE tasks SET state='IN_PROGRESS',archived_at='2026-02-01T00:00:00Z'`, false},
		{"unreadable archive time", `UPDATE tasks SET state='IN_PROGRESS',archived_at='invalid'`, false},
		{"missing task", `DELETE FROM tasks`, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			d := payloadScanFixture(t)
			_, err := d.Exec(`ALTER TABLE tasks ADD COLUMN archived_at TIMESTAMP`)
			require.NoError(t, err)
			if tt.mutation != "" {
				_, err = d.Exec(tt.mutation)
				require.NoError(t, err)
			}
			got, err := HistoryTaskEligible(context.Background(), d, "sqlite3", "task", cutoff)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestHistoryTaskEligibleRejectsOtherDrivers(t *testing.T) {
	d := payloadScanFixture(t)
	_, err := HistoryTaskEligible(context.Background(), d, "postgres", "task", time.Now())
	require.Error(t, err)
}
