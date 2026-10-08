package sqlite

const workflowScriptRunsSchemaDDL = `
	CREATE TABLE IF NOT EXISTS workflow_script_runs (
		id TEXT PRIMARY KEY,
		occurrence_key TEXT NOT NULL UNIQUE,
		task_id TEXT NOT NULL,
		workflow_id TEXT NOT NULL DEFAULT '',
		workflow_step_id TEXT NOT NULL DEFAULT '',
		workflow_step_name TEXT NOT NULL DEFAULT '',
		trigger TEXT NOT NULL,
		action_position INTEGER NOT NULL,
		session_id TEXT NOT NULL DEFAULT '',
		execution_id TEXT NOT NULL DEFAULT '',
		command TEXT NOT NULL,
		timeout_seconds INTEGER NOT NULL,
		failure_policy TEXT NOT NULL,
		process_request_id TEXT NOT NULL,
		message_id TEXT NOT NULL DEFAULT '',
		process_id TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT 'pending',
		admission_attempted_at TIMESTAMP,
		exit_code INTEGER,
		output TEXT NOT NULL DEFAULT '',
		output_truncated INTEGER NOT NULL DEFAULT 0,
		failure_reason TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		started_at TIMESTAMP,
		completed_at TIMESTAMP,
		FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_workflow_script_runs_task
		ON workflow_script_runs(task_id, created_at DESC);
	CREATE INDEX IF NOT EXISTS idx_workflow_script_runs_status
		ON workflow_script_runs(status, created_at ASC);
`

func (r *Repository) initWorkflowScriptRunsSchema() error {
	_, err := r.db.Exec(workflowScriptRunsSchemaDDL)
	return err
}
