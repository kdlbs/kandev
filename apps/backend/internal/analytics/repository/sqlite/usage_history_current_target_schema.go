package sqlite

import (
	"context"

	"github.com/kandev/kandev/internal/db"
)

func (r *Repository) ensureUsageCurrentTargetSchema(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS usage_collection_current_targets (
			scope_id TEXT NOT NULL,subject_id TEXT NOT NULL,binding_id TEXT NOT NULL,
			task_id TEXT NOT NULL,session_id TEXT NOT NULL,external_session_id TEXT NOT NULL,PRIMARY KEY(scope_id,subject_id))`,
		`CREATE INDEX IF NOT EXISTS idx_usage_current_task ON usage_collection_current_targets(scope_id,task_id,session_id)`,
		`CREATE INDEX IF NOT EXISTS idx_usage_current_transcript ON usage_collection_current_targets(scope_id,external_session_id,subject_id)`,
		`CREATE TABLE IF NOT EXISTS usage_collection_source_guards (
			source_hash TEXT PRIMARY KEY,scope_hash TEXT NOT NULL,mixed INTEGER NOT NULL DEFAULT 0,erased INTEGER NOT NULL DEFAULT 0)`,
		`CREATE INDEX IF NOT EXISTS idx_usage_source_guard_scope ON usage_collection_source_guards(scope_hash)`,
		`CREATE TABLE IF NOT EXISTS usage_collection_guard_erasure (scope_id TEXT PRIMARY KEY,last_sequence BIGINT NOT NULL DEFAULT 0,complete INTEGER NOT NULL DEFAULT 0)`,
		`CREATE TABLE IF NOT EXISTS usage_collection_guard_workspaces (workspace_hash TEXT NOT NULL,source_hash TEXT NOT NULL,PRIMARY KEY(workspace_hash,source_hash))`,
		`CREATE INDEX IF NOT EXISTS idx_usage_source_guard_membership ON usage_collection_guard_workspaces(source_hash,workspace_hash)`,
		`CREATE INDEX IF NOT EXISTS idx_usage_source_subject_identity ON usage_subjects(task_id,task_incarnation_id,session_id,session_incarnation_id,scope_id)`,
		`CREATE INDEX IF NOT EXISTS idx_usage_source_binding_transcript ON usage_collection_bindings(external_session_id,scope_id)`,
		`CREATE INDEX IF NOT EXISTS idx_usage_collection_subject_version ON usage_collection_bindings(subject_id,sequence)`,
		`CREATE INDEX IF NOT EXISTS idx_usage_bucket_subject_coverage ON session_usage_buckets(scope_id,subject_id,retired,coverage_start,coverage_end)`,
	}
	for _, statement := range statements {
		if _, err := r.db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return r.ensureUsageLiveSourceIndexes(ctx)
}

func (r *Repository) ensureUsageLiveSourceIndexes(ctx context.Context) error {
	has, err := db.ColumnExistsContext(ctx, r.db, "task_sessions", "metadata")
	if err != nil {
		return err
	}
	if has {
		if _, err := r.db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_usage_live_transcript ON task_sessions ((`+usageLiveTranscriptExpression(r.db.DriverName(), "metadata")+`))`); err != nil {
			return err
		}
	}
	has, err = db.TableExistsContext(ctx, r.db, "executors_running")
	if err != nil || !has {
		return err
	}
	has, err = db.ColumnExistsContext(ctx, r.db, "executors_running", "resume_token")
	if err != nil || !has {
		return err
	}
	_, err = r.db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_usage_live_resume ON executors_running(resume_token,session_id)`)
	return err
}
