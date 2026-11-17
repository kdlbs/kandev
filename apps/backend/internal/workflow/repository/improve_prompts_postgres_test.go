package repository

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestImprovePromptPostgres(t *testing.T) {
	dsn := os.Getenv("KANDEV_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("KANDEV_TEST_POSTGRES_DSN is not set")
	}
	database := openIsolatedPostgresMultiConn(t, dsn, 1)
	ctx := context.Background()
	if _, err := database.Exec(`
		CREATE TABLE workflows (
			id TEXT PRIMARY KEY,
			workspace_id TEXT NOT NULL DEFAULT '',
			workflow_template_id TEXT NOT NULL DEFAULT '',
			is_system INTEGER NOT NULL DEFAULT 0,
			name TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL
		);
		CREATE TABLE task_sessions (id TEXT PRIMARY KEY);
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := NewWithDB(database, database, nil); err != nil {
		t.Fatal(err)
	}
	previous := readPreviousImprovePrompts(t)
	current := currentImprovePrompts(t)
	var stepsJSON string
	if err := database.Get(&stepsJSON, database.Rebind(`SELECT steps FROM workflow_templates WHERE id = ?`), improveKandevTemplateID); err != nil {
		t.Fatal(err)
	}
	stepsJSON = setPromptInRawTemplate(t, stepsJSON, previous)
	if _, err := database.ExecContext(ctx, database.Rebind(`UPDATE workflow_templates SET steps = ? WHERE id = ?`), stepsJSON, improveKandevTemplateID); err != nil {
		t.Fatal(err)
	}
	created := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	if _, err := database.ExecContext(ctx, database.Rebind(`
		INSERT INTO workflows (id, workspace_id, workflow_template_id, is_system, name, created_at, updated_at)
		VALUES (?, 'workspace-1', ?, 0, 'reused Improve workflow', ?, ?)
	`), "wf-postgres", improveKandevTemplateID, created, created); err != nil {
		t.Fatal(err)
	}
	for position, step := range []struct{ id, name string }{{"improve", "Improve"}, {"test", "Test"}} {
		if _, err := database.ExecContext(ctx, database.Rebind(`
			INSERT INTO workflow_steps (id, workflow_id, name, position, prompt, created_at, updated_at)
			VALUES (?, 'wf-postgres', ?, ?, ?, ?, ?)
		`), "wf-postgres-"+step.id, step.name, position, previous[step.id], created, created); err != nil {
			t.Fatal(err)
		}
	}

	reopened, err := NewWithDB(database, database, nil)
	if err != nil {
		t.Fatalf("reopen PostgreSQL workflow repository: %v", err)
	}
	storedTemplate, err := reopened.GetTemplate(ctx, improveKandevTemplateID)
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range storedTemplate.Steps {
		if want, ok := current[step.ID]; ok && step.Prompt != want {
			t.Fatalf("PostgreSQL template prompt %q = %q, want %q", step.ID, step.Prompt, want)
		}
	}
	for _, step := range []string{"improve", "test"} {
		var prompt string
		if err := database.Get(&prompt, database.Rebind(`SELECT prompt FROM workflow_steps WHERE id = ?`), "wf-postgres-"+step); err != nil {
			t.Fatal(err)
		}
		if prompt != current[step] {
			t.Fatalf("PostgreSQL workflow step %q prompt = %q, want current", step, prompt)
		}
	}
}
