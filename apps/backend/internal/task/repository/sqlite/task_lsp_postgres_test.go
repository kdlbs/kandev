package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/lsp"
	"github.com/kandev/kandev/internal/testutil"
)

func TestPostgresTaskLSPSchemaReplay(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("initialize postgres schema: %v", err)
	}
	if _, err := db.Exec(`DROP TABLE task_lsp_languages`); err != nil {
		t.Fatalf("drop task LSP table to simulate legacy schema: %v", err)
	}
	if err := repo.runMigrations(); err != nil {
		t.Fatalf("replay postgres migrations: %v", err)
	}
	if err := repo.runMigrations(); err != nil {
		t.Fatalf("replay postgres migrations twice: %v", err)
	}

	ctx := context.Background()
	now := time.Now().UTC()
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`), "task-lsp-postgres", "workspace-lsp-postgres", "Postgres LSP", now, now); err != nil {
		t.Fatalf("seed postgres task: %v", err)
	}
	state := lsp.DefaultTaskLanguageState("task-lsp-postgres", "kotlin")
	state.Policy = lsp.PolicyKeepWarm
	state.Detected = true
	state.DetectionState = lsp.DetectionComplete
	state.LastTransitionAt = now
	stored, err := repo.CompareAndUpdateTaskLSPLanguage(ctx, state, 0)
	if err != nil {
		t.Fatalf("insert postgres task LSP state: %v", err)
	}
	if stored.Revision != 1 {
		t.Fatalf("postgres task LSP revision = %d", stored.Revision)
	}
	allocated, err := repo.AllocateTaskLSPGeneration(
		ctx,
		state.TaskID,
		state.Language,
		lsp.ActionStart,
		lsp.InitiatorUser,
		"user_start",
		now,
	)
	if err != nil {
		t.Fatalf("allocate postgres task LSP generation: %v", err)
	}
	if allocated.Generation != 1 || allocated.Revision != 2 {
		t.Fatalf("postgres task LSP allocation = %#v", allocated)
	}

	if _, err := db.Exec(db.Rebind(`DELETE FROM tasks WHERE id = ?`), state.TaskID); err != nil {
		t.Fatalf("delete postgres task: %v", err)
	}
	if _, found, err := repo.GetTaskLSPLanguage(ctx, state.TaskID, state.Language); err != nil || found {
		t.Fatalf("postgres cascade result: found=%v err=%v", found, err)
	}
}
