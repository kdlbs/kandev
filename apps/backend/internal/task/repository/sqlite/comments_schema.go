package sqlite

import "github.com/jmoiron/sqlx"

func (r *Repository) initTaskCommentsSchema() error { return EnsureCommentsSchema(r.db) }

// EnsureCommentsSchema initializes canonical task comments for shared-store callers.
func EnsureCommentsSchema(writer *sqlx.DB) error {
	_, err := writer.Exec(`	CREATE TABLE IF NOT EXISTS task_comments (
		id TEXT PRIMARY KEY,
		task_id TEXT NOT NULL,
		author_type TEXT NOT NULL,
		author_id TEXT NOT NULL,
		body TEXT NOT NULL,
		source TEXT NOT NULL DEFAULT 'user',
		reply_channel_id TEXT DEFAULT '',
		created_at TIMESTAMP NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_task_comments_task_created ON task_comments(task_id, created_at);`)
	return err
}
