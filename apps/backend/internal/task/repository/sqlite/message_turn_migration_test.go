package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"

	dbutil "github.com/kandev/kandev/internal/db"
)

func TestMigrateTaskSessionMessagesTurnNullableRebuildsLegacySQLiteTable(t *testing.T) {
	dbConn, err := dbutil.OpenSQLite(filepath.Join(t.TempDir(), "message-turn-nullable.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`
		CREATE TABLE task_sessions (id TEXT PRIMARY KEY);
		CREATE TABLE task_session_turns (id TEXT PRIMARY KEY);
		INSERT INTO task_sessions (id) VALUES ('session');
		INSERT INTO task_session_turns (id) VALUES ('turn');`); err != nil {
		t.Fatalf("create message dependencies: %v", err)
	}

	_, err = db.Exec(`
		CREATE TABLE task_session_messages (
			id TEXT PRIMARY KEY,
			task_session_id TEXT NOT NULL,
			task_id TEXT DEFAULT '',
			turn_id TEXT NOT NULL,
			author_type TEXT NOT NULL DEFAULT 'user',
			author_id TEXT DEFAULT '',
			content TEXT NOT NULL,
			requests_input INTEGER DEFAULT 0,
			type TEXT NOT NULL DEFAULT 'message',
			metadata TEXT DEFAULT '{}',
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			prompt_seq INTEGER NOT NULL DEFAULT 0
		)`)
	if err != nil {
		t.Fatalf("create legacy message table: %v", err)
	}
	_, err = db.Exec(`
		INSERT INTO task_session_messages
			(id, task_session_id, task_id, turn_id, author_type, author_id, content, created_at)
		VALUES ('existing', 'session', 'task', 'turn', 'agent', '', 'output', CURRENT_TIMESTAMP)`)
	if err != nil {
		t.Fatalf("insert legacy message: %v", err)
	}

	repo := &Repository{
		db:      db,
		ro:      db,
		migrate: dbutil.NewMigrateLogger(db, nil),
	}
	if err := repo.migrateTaskSessionMessagesTurnNullable(); err != nil {
		t.Fatalf("migrate nullable turn: %v", err)
	}

	var notNull int
	if err := db.Get(&notNull, `
		SELECT "notnull" FROM pragma_table_info('task_session_messages') WHERE name = 'turn_id'`); err != nil {
		t.Fatalf("read turn_id nullability: %v", err)
	}
	if notNull != 0 {
		t.Fatalf("turn_id notnull = %d, want nullable", notNull)
	}
	if _, err := db.Exec(`
		INSERT INTO task_session_messages
			(id, task_session_id, task_id, turn_id, author_type, author_id, content, created_at)
		VALUES ('lifecycle', 'session', 'task', NULL, 'agent', '', 'script', CURRENT_TIMESTAMP)`); err != nil {
		t.Fatalf("insert lifecycle message without turn: %v", err)
	}

	var content string
	if err := db.GetContext(context.Background(), &content,
		`SELECT content FROM task_session_messages WHERE id = 'existing'`); err != nil {
		t.Fatalf("read migrated message: %v", err)
	}
	if content != "output" {
		t.Fatalf("migrated content = %q, want output", content)
	}
}
