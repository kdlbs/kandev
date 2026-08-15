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
	ctx := context.Background()
	now := time.Now().UTC()
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`), "task-lsp-postgres", "workspace-lsp-postgres", "Postgres LSP", now, now); err != nil {
		t.Fatalf("seed postgres task: %v", err)
	}
	if _, err := db.Exec(`
		ALTER TABLE task_lsp_languages DROP COLUMN process_absent_generation
	`); err != nil {
		t.Fatalf("rewind task LSP table to legacy schema: %v", err)
	}
	legacyRows := []struct {
		language   string
		phase      lsp.Phase
		generation uint64
		errorCode  string
		wantAbsent uint64
	}{
		{language: "go", phase: lsp.PhaseOff, generation: 3, wantAbsent: 3},
		{language: "kotlin", phase: lsp.PhaseError, generation: 4,
			errorCode: "process_start_failed", wantAbsent: 4},
		{language: "python", phase: lsp.PhaseError, generation: 5,
			errorCode: "task_host_unreachable", wantAbsent: 0},
		{language: "rust", phase: lsp.PhaseReady, generation: 6, wantAbsent: 0},
	}
	for _, row := range legacyRows {
		if _, err := db.Exec(db.Rebind(`
			INSERT INTO task_lsp_languages (
				task_id, language, phase, generation, revision, last_transition_at,
				error_code, created_at, updated_at
			) VALUES (?, ?, ?, ?, 1, ?, ?, ?, ?)
		`), "task-lsp-postgres", row.language, row.phase, row.generation,
			now, row.errorCode, now, now); err != nil {
			t.Fatalf("seed legacy postgres row %s: %v", row.language, err)
		}
	}
	if err := repo.runMigrations(); err != nil {
		t.Fatalf("replay postgres migrations: %v", err)
	}
	if err := repo.runMigrations(); err != nil {
		t.Fatalf("replay postgres migrations twice: %v", err)
	}

	for _, row := range legacyRows {
		stored, found, err := repo.GetTaskLSPLanguage(ctx, "task-lsp-postgres", row.language)
		if err != nil || !found {
			t.Fatalf("read migrated postgres row %s: found=%v err=%v", row.language, found, err)
		}
		if stored.ProcessAbsentGeneration != row.wantAbsent {
			t.Fatalf("migrated postgres row %s absence = %d, want %d",
				row.language, stored.ProcessAbsentGeneration, row.wantAbsent)
		}
	}
	allocated, err := repo.AllocateTaskLSPGeneration(
		ctx,
		"task-lsp-postgres",
		"go",
		lsp.ActionStart,
		lsp.InitiatorUser,
		"user_start",
		now,
	)
	if err != nil {
		t.Fatalf("allocate postgres task LSP generation: %v", err)
	}
	if allocated.Generation != 4 || allocated.Revision != 2 || allocated.ProcessAbsentGeneration != 0 {
		t.Fatalf("postgres task LSP allocation = %#v", allocated)
	}

	if _, err := db.Exec(db.Rebind(`DELETE FROM tasks WHERE id = ?`), "task-lsp-postgres"); err != nil {
		t.Fatalf("delete postgres task: %v", err)
	}
	if _, found, err := repo.GetTaskLSPLanguage(ctx, "task-lsp-postgres", "go"); err != nil || found {
		t.Fatalf("postgres cascade result: found=%v err=%v", found, err)
	}
}
