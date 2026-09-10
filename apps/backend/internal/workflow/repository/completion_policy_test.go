package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/workflow/models"
)

func TestCompletionPolicyMigration(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:?_foreign_keys=on")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	database := sqlx.NewDb(db, "sqlite3")
	t.Cleanup(func() { _ = database.Close() })

	_, err = database.Exec(`
		CREATE TABLE workflows (
			id TEXT PRIMARY KEY,
			workspace_id TEXT NOT NULL DEFAULT '',
			workflow_template_id TEXT DEFAULT '',
			name TEXT NOT NULL,
			description TEXT DEFAULT '',
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL
		);
		CREATE TABLE task_sessions (id TEXT PRIMARY KEY);
		CREATE TABLE workflow_templates (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT,
			is_system INTEGER DEFAULT 0,
			steps TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL
		);
		INSERT INTO workflows (id, name, created_at, updated_at)
		VALUES ('workflow-completion-migration', 'Legacy', datetime('now'), datetime('now'));
		INSERT INTO workflow_templates (id, name, description, steps, created_at, updated_at)
		VALUES ('legacy-template', 'Legacy template', '', '[
			{"id":"template-work","name":"Work","position":0,"custom":"preserve"},
			{"id":"template-done","name":"  dOnE  ","position":1}
		]', datetime('now'), datetime('now'));
		CREATE TABLE workflow_steps (
			id TEXT PRIMARY KEY,
			workflow_id TEXT NOT NULL,
			name TEXT NOT NULL,
			position INTEGER NOT NULL,
			color TEXT,
			prompt TEXT,
			events TEXT,
			allow_manual_move INTEGER DEFAULT 1,
			is_start_step INTEGER DEFAULT 0,
			show_in_command_panel INTEGER DEFAULT 1,
			auto_archive_after_hours INTEGER DEFAULT 0,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			FOREIGN KEY (workflow_id) REFERENCES workflows(id) ON DELETE CASCADE
		);
		INSERT INTO workflow_steps (id, workflow_id, name, position, created_at, updated_at)
		VALUES
			('legacy-work', 'workflow-completion-migration', 'Work', 0, datetime('now'), datetime('now')),
			('legacy-done', 'workflow-completion-migration', '  dOnE  ', 1, datetime('now'), datetime('now'));
	`)
	if err != nil {
		t.Fatalf("seed legacy schema: %v", err)
	}

	repo, err := NewWithDB(database, database, nil)
	if err != nil {
		t.Fatalf("initialize workflow repository: %v", err)
	}
	ctx := context.Background()
	work, err := repo.GetStep(ctx, "legacy-work")
	if err != nil {
		t.Fatalf("get legacy work step: %v", err)
	}
	done, err := repo.GetStep(ctx, "legacy-done")
	if err != nil {
		t.Fatalf("get legacy done step: %v", err)
	}
	if work.CompleteTaskOnEnter {
		t.Fatal("non-terminal legacy step was enabled")
	}
	if !done.CompleteTaskOnEnter {
		t.Fatal("legacy final Done step was not enabled")
	}
	template, err := repo.GetTemplate(ctx, "legacy-template")
	if err != nil {
		t.Fatalf("get migrated template: %v", err)
	}
	if template.Steps[0].CompleteTaskOnEnter {
		t.Fatal("non-final template step was enabled")
	}
	if !template.Steps[1].CompleteTaskOnEnter {
		t.Fatal("legacy final template step was not enabled")
	}
	var stepsJSON string
	if err := database.Get(&stepsJSON, `SELECT steps FROM workflow_templates WHERE id = 'legacy-template'`); err != nil {
		t.Fatalf("read migrated template JSON: %v", err)
	}
	var rawSteps []map[string]json.RawMessage
	if err := json.Unmarshal([]byte(stepsJSON), &rawSteps); err != nil {
		t.Fatalf("decode migrated template JSON: %v", err)
	}
	var custom string
	if err := json.Unmarshal(rawSteps[0]["custom"], &custom); err != nil {
		t.Fatalf("decode preserved template field: %v", err)
	}
	if custom != "preserve" {
		t.Fatalf("preserved template field = %q, want preserve", custom)
	}
	for i, rawStep := range rawSteps {
		if _, ok := rawStep["complete_task_on_enter"]; !ok {
			t.Fatalf("template step %d is missing completion policy", i)
		}
	}
}

func TestCompletionPolicyReplayPreservesDisabled(t *testing.T) {
	repo, database := setupTestRepoWithDB(t)
	ctx := context.Background()
	step := &models.WorkflowStep{
		WorkflowID:          "wf-test",
		Name:                "Done",
		Position:            0,
		CompleteTaskOnEnter: true,
	}
	if err := repo.CreateStep(ctx, step); err != nil {
		t.Fatalf("create completion step: %v", err)
	}
	step.CompleteTaskOnEnter = false
	if err := repo.UpdateStep(ctx, step); err != nil {
		t.Fatalf("disable completion step: %v", err)
	}
	if err := repo.backfillCompletionPolicy(); err != nil {
		t.Fatalf("replay completion policy backfill: %v", err)
	}
	got, err := repo.GetStep(ctx, step.ID)
	if err != nil {
		t.Fatalf("reload disabled completion step: %v", err)
	}
	if got.CompleteTaskOnEnter {
		t.Fatal("replayed backfill overwrote an explicit false")
	}
	var marker string
	if err := database.Get(&marker, database.Rebind(`SELECT value FROM kandev_meta WHERE key = ?`), completionPolicyBackfillMetaKey); err != nil {
		t.Fatalf("read completion policy marker: %v", err)
	}
	if marker == "" {
		t.Fatal("completion policy marker is empty")
	}
}

func TestCompletionPolicyMigrationRollsBackAndRetries(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:?_foreign_keys=on")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	database := sqlx.NewDb(db, "sqlite3")
	t.Cleanup(func() { _ = database.Close() })

	_, err = database.Exec(`
		CREATE TABLE workflows (
			id TEXT PRIMARY KEY,
			workspace_id TEXT NOT NULL DEFAULT '',
			workflow_template_id TEXT DEFAULT '',
			name TEXT NOT NULL,
			description TEXT DEFAULT '',
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL
		);
		CREATE TABLE task_sessions (id TEXT PRIMARY KEY);
		CREATE TABLE kandev_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL DEFAULT '');
		INSERT INTO workflows (id, name, created_at, updated_at)
		VALUES ('workflow-completion-retry', 'Legacy', datetime('now'), datetime('now'));
		CREATE TABLE workflow_steps (
			id TEXT PRIMARY KEY,
			workflow_id TEXT NOT NULL,
			name TEXT NOT NULL,
			position INTEGER NOT NULL,
			color TEXT,
			prompt TEXT,
			events TEXT,
			allow_manual_move INTEGER DEFAULT 1,
			is_start_step INTEGER DEFAULT 0,
			show_in_command_panel INTEGER DEFAULT 1,
			auto_archive_after_hours INTEGER DEFAULT 0,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			FOREIGN KEY (workflow_id) REFERENCES workflows(id) ON DELETE CASCADE
		);
		INSERT INTO workflow_steps (id, workflow_id, name, position, created_at, updated_at)
		VALUES ('legacy-retry-done', 'workflow-completion-retry', 'Done', 0, datetime('now'), datetime('now'));
		CREATE TRIGGER fail_completion_policy_marker
		BEFORE INSERT ON kandev_meta
		WHEN NEW.key = 'workflow_complete_task_on_enter_backfill_v1'
		BEGIN
			SELECT RAISE(ABORT, 'injected completion policy failure');
		END;
	`)
	if err != nil {
		t.Fatalf("seed retry schema: %v", err)
	}

	if _, err := NewWithDB(database, database, nil); err == nil {
		t.Fatal("completion policy migration succeeded despite injected failure")
	}
	var enabled int
	if err := database.Get(&enabled, `SELECT complete_task_on_enter FROM workflow_steps WHERE id = 'legacy-retry-done'`); err != nil {
		t.Fatalf("read rolled-back completion policy: %v", err)
	}
	if enabled != 0 {
		t.Fatalf("rolled-back completion policy = %d, want 0", enabled)
	}
	var marker string
	if err := database.Get(&marker, database.Rebind(`SELECT value FROM kandev_meta WHERE key = ?`), completionPolicyBackfillMetaKey); err != sql.ErrNoRows {
		t.Fatalf("rolled-back marker error = %v, want sql.ErrNoRows", err)
	}

	if _, err := database.Exec(`DROP TRIGGER fail_completion_policy_marker`); err != nil {
		t.Fatalf("remove injected failure: %v", err)
	}
	repo, err := NewWithDB(database, database, nil)
	if err != nil {
		t.Fatalf("retry completion policy migration: %v", err)
	}
	step, err := repo.GetStep(context.Background(), "legacy-retry-done")
	if err != nil {
		t.Fatalf("read retried completion policy: %v", err)
	}
	if !step.CompleteTaskOnEnter {
		t.Fatal("retried completion policy was not enabled")
	}
	if err := database.Get(&marker, database.Rebind(`SELECT value FROM kandev_meta WHERE key = ?`), completionPolicyBackfillMetaKey); err != nil {
		t.Fatalf("read retried marker: %v", err)
	}
}
