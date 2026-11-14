package sqlite

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/hierarchy"
)

func TestTaskExecutionGradeStaleLegacyWriterAndRollback(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "grade-ws", Name: "Grade"}); err != nil {
		t.Fatal(err)
	}
	task := &models.Task{ID: "grade-task", WorkspaceID: "grade-ws", Title: "Original"}
	if err := json.Unmarshal([]byte(`{"execution_grade":"Initial"}`), task); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	stale, err := repo.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	var update models.TaskFieldUpdate
	if err := json.Unmarshal([]byte(`{"ExecutionGrade":"Concurrent"}`), &update); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.UpdateTaskFieldsWithParentAdmission(ctx, task.ID, update, hierarchy.ValidateParent); err != nil {
		t.Fatal(err)
	}
	stale.Title = "Renamed"
	if err := repo.UpdateTask(ctx, stale); err != nil {
		t.Fatal(err)
	}
	var grade string
	if err := repo.DB().QueryRow(`SELECT execution_grade FROM tasks WHERE id = ?`, task.ID).Scan(&grade); err != nil {
		t.Fatal(err)
	}
	if grade != "Concurrent" {
		t.Fatalf("stale writer grade=%q", grade)
	}
	if err := json.Unmarshal([]byte(`{"ExecutionGrade":"Rejected"}`), &update); err != nil {
		t.Fatal(err)
	}
	update.Metadata = map[string]any{"invalid": func() {}}
	if _, err := repo.UpdateTaskFieldsWithParentAdmission(ctx, task.ID, update, hierarchy.ValidateParent); err == nil {
		t.Fatal("invalid metadata write succeeded")
	}
	if err := repo.DB().QueryRow(`SELECT execution_grade FROM tasks WHERE id = ?`, task.ID).Scan(&grade); err != nil {
		t.Fatal(err)
	}
	if grade != "Concurrent" {
		t.Fatalf("rollback leaked grade=%q", grade)
	}
}

func TestTaskExecutionGradeLegacyFKRebuild(t *testing.T) {
	for _, want := range []string{"", "Existing"} {
		t.Run("grade_"+want, func(t *testing.T) {
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
			wip_admitted INTEGER NOT NULL DEFAULT 1,
			queued_for_step_id TEXT NOT NULL DEFAULT '',
			queued_at TIMESTAMP,
			metadata TEXT DEFAULT '{}',
			is_ephemeral INTEGER NOT NULL DEFAULT 0,
			parent_id TEXT DEFAULT '',
			autopilot_enabled INTEGER NOT NULL DEFAULT 0,
			archived_at TIMESTAMP,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			FOREIGN KEY (workflow_id) REFERENCES workflows(id)
		);
		INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
			VALUES ('task-legacy-fk', 'ws-legacy', 'Legacy FK task', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);
	`); err != nil {
				t.Fatal(err)
			}
			if want != "" {
				if _, err := db.Exec(`ALTER TABLE tasks ADD COLUMN execution_grade TEXT NOT NULL DEFAULT ''`); err != nil {
					t.Fatal(err)
				}
				if _, err := db.Exec(`UPDATE tasks SET execution_grade = ?`, want); err != nil {
					t.Fatal(err)
				}
			}
			for range 2 {
				if _, err := NewWithDB(db, db, nil); err != nil {
					t.Fatal(err)
				}
				var grade, title string
				if err := db.QueryRow(`SELECT execution_grade, title FROM tasks WHERE id = 'task-legacy-fk'`).Scan(&grade, &title); err != nil {
					t.Fatal(err)
				}
				if grade != want || title != "Legacy FK task" {
					t.Fatalf("grade=%q title=%q", grade, title)
				}
			}
		})
	}
}
