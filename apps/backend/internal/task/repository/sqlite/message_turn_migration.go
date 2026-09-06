package sqlite

import "github.com/kandev/kandev/internal/db/dialect"

// migrateTaskSessionMessagesTurnNullable lets lifecycle-only messages remain
// durable without manufacturing a conversational turn. Existing SQLite
// installations need a table rebuild because SQLite cannot drop NOT NULL from
// a column in place; PostgreSQL can apply the equivalent ALTER directly.
func (r *Repository) migrateTaskSessionMessagesTurnNullable() error {
	if dialect.IsPostgres(r.db.DriverName()) {
		r.migrate.Apply(
			"task_session_messages.turn_id_nullable",
			`ALTER TABLE task_session_messages ALTER COLUMN turn_id DROP NOT NULL`,
		)
		return nil
	}

	return r.recreateTableNamed(
		"task_session_messages.recreate_turn_id_nullable",
		"task_session_messages",
		"turn_id TEXT NOT NULL",
		[]string{
			`CREATE TABLE task_session_messages_new (
				id TEXT PRIMARY KEY,
				task_session_id TEXT NOT NULL,
				task_id TEXT DEFAULT '',
				turn_id TEXT,
				author_type TEXT NOT NULL DEFAULT 'user',
				author_id TEXT DEFAULT '',
				content TEXT NOT NULL,
				requests_input INTEGER DEFAULT 0,
				type TEXT NOT NULL DEFAULT 'message',
				metadata TEXT DEFAULT '{}',
				created_at TIMESTAMP NOT NULL,
				updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				prompt_seq INTEGER NOT NULL DEFAULT 0,
				FOREIGN KEY (task_session_id) REFERENCES task_sessions(id) ON DELETE CASCADE,
				FOREIGN KEY (turn_id) REFERENCES task_session_turns(id) ON DELETE CASCADE
			)`,
			`INSERT INTO task_session_messages_new (
				id, task_session_id, task_id, turn_id, author_type, author_id,
				content, requests_input, type, metadata, created_at, updated_at,
				prompt_seq
			)
			SELECT id, task_session_id, task_id, turn_id, author_type, author_id,
				content, requests_input, type, metadata, created_at, updated_at,
				prompt_seq
			FROM task_session_messages`,
			`DROP TABLE task_session_messages`,
			`ALTER TABLE task_session_messages_new RENAME TO task_session_messages`,
			`CREATE INDEX IF NOT EXISTS idx_messages_session_id ON task_session_messages(task_session_id)`,
			`CREATE INDEX IF NOT EXISTS idx_messages_created_at ON task_session_messages(created_at)`,
			`CREATE INDEX IF NOT EXISTS idx_messages_session_created ON task_session_messages(task_session_id, created_at)`,
			`CREATE INDEX IF NOT EXISTS idx_messages_turn_id ON task_session_messages(turn_id)`,
			`CREATE INDEX IF NOT EXISTS idx_messages_task_author_created ON task_session_messages(task_id, author_type, type, created_at DESC)`,
			`CREATE INDEX IF NOT EXISTS idx_messages_session_updated ON task_session_messages(task_session_id, updated_at)`,
		},
	)
}
