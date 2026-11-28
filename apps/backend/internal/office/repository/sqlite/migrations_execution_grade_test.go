package sqlite_test

import (
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

func TestTaskExecutionGradeOfficePriorityRebuild(t *testing.T) {
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`
		CREATE TABLE tasks (
			id TEXT PRIMARY KEY,
			workspace_id TEXT NOT NULL DEFAULT '',
			workflow_id TEXT NOT NULL DEFAULT '',
			workflow_step_id TEXT NOT NULL DEFAULT '',
			title TEXT NOT NULL,
			description TEXT DEFAULT '',
			state TEXT DEFAULT 'TODO',
			priority INTEGER DEFAULT 0,
			position INTEGER DEFAULT 0,
			metadata TEXT DEFAULT '{}',
			is_ephemeral INTEGER NOT NULL DEFAULT 0,
			parent_id TEXT DEFAULT '',
			archived_at TIMESTAMP,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			origin TEXT DEFAULT 'manual',
			project_id TEXT DEFAULT '',
			labels TEXT DEFAULT '[]',
			identifier TEXT,
			assignee_user_id TEXT NOT NULL DEFAULT '',
			assignment_generation INTEGER NOT NULL DEFAULT 0
		);
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`ALTER TABLE tasks ADD COLUMN execution_grade TEXT NOT NULL DEFAULT ''`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO tasks (id,title,execution_grade,assignment_generation) VALUES ('grade-task','Preserved','Existing',3)`); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := sqlite.NewWithDB(db, db, nil); err != nil {
			t.Fatal(err)
		}
		var grade, title string
		var generation int
		if err := db.QueryRow(`SELECT execution_grade, title, assignment_generation FROM tasks WHERE id='grade-task'`).Scan(&grade, &title, &generation); err != nil {
			t.Fatal(err)
		}
		if grade != "Existing" || title != "Preserved" || generation != 3 {
			t.Fatalf("grade=%q title=%q generation=%d", grade, title, generation)
		}
	}
}
