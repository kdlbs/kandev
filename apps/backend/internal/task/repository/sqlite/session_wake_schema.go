package sqlite

const taskSessionWakeSchemaDDL = `
	CREATE TABLE IF NOT EXISTS task_session_wakes (
		id TEXT PRIMARY KEY,
		task_id TEXT NOT NULL,
		session_id TEXT NOT NULL,
		marker TEXT NOT NULL,
		prompt TEXT NOT NULL,
		cron_expression TEXT NOT NULL,
		timezone TEXT NOT NULL,
		next_run_at TIMESTAMP NOT NULL,
		expires_at TIMESTAMP NOT NULL,
		last_attempt_at TIMESTAMP,
		last_fired_at TIMESTAMP,
		last_delivery_status TEXT NOT NULL DEFAULT 'pending',
		last_error TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		UNIQUE(task_id, session_id, marker),
		FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE,
		FOREIGN KEY (session_id) REFERENCES task_sessions(id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_task_session_wakes_due
		ON task_session_wakes(next_run_at, expires_at);
`

func (r *Repository) initTaskSessionWakeSchema() error {
	_, err := r.db.Exec(taskSessionWakeSchemaDDL)
	return err
}
