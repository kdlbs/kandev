package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	dbutil "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/lsp"
)

func TestTaskLSPSchemaReplayCompositeKeyAndCascade(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "task-lsp.db")
	dbConn, err := dbutil.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })

	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("initialize schema: %v", err)
	}

	var tableName string
	if err := db.Get(&tableName, `
		SELECT name FROM sqlite_master
		WHERE type = 'table' AND name = 'task_lsp_languages'
	`); err != nil {
		t.Fatalf("task LSP language table is missing: %v", err)
	}

	now := time.Now().UTC()
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`), "task-lsp-schema", "workspace-lsp-schema", "LSP schema", now, now); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO task_lsp_languages (
			task_id, language, last_transition_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?)
	`), "task-lsp-schema", "kotlin", now, now, now); err != nil {
		t.Fatalf("insert Kotlin row: %v", err)
	}
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO task_lsp_languages (
			task_id, language, last_transition_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?)
	`), "task-lsp-schema", "rust", now, now, now); err != nil {
		t.Fatalf("composite key rejected second language: %v", err)
	}
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO task_lsp_languages (
			task_id, language, last_transition_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?)
	`), "task-lsp-schema", "kotlin", now, now, now); err == nil {
		t.Fatal("duplicate task/language row was accepted")
	}
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO task_lsp_languages (
			task_id, language, policy, last_transition_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?)
	`), "task-lsp-schema", "invalid-policy", "session_owned", now, now, now); err == nil {
		t.Fatal("invalid task LSP policy was accepted")
	}

	if err := repo.runMigrations(); err != nil {
		t.Fatalf("replay migrations: %v", err)
	}
	if err := repo.runMigrations(); err != nil {
		t.Fatalf("replay migrations twice: %v", err)
	}
	if _, err := db.ExecContext(context.Background(), db.Rebind(`DELETE FROM tasks WHERE id = ?`), "task-lsp-schema"); err != nil {
		t.Fatalf("delete task: %v", err)
	}
	var count int
	if err := db.Get(&count, db.Rebind(`SELECT COUNT(*) FROM task_lsp_languages WHERE task_id = ?`), "task-lsp-schema"); err != nil {
		t.Fatalf("count cascade rows: %v", err)
	}
	if count != 0 {
		t.Fatalf("task LSP rows after task deletion = %d, want 0", count)
	}
}

func TestTaskLSPPolicyEvidenceRoundTripAndRevisionCAS(t *testing.T) {
	repo, db := newTaskLSPTestRepo(t)
	seedTaskForStatusSummary(t, db, "task-lsp-roundtrip", "workspace-lsp-roundtrip")
	ctx := context.Background()

	missing, found, err := repo.GetTaskLSPLanguage(ctx, "task-lsp-roundtrip", "kotlin")
	if err != nil {
		t.Fatalf("load absent language: %v", err)
	}
	if found || missing == nil || missing.Policy != lsp.PolicyInherit || missing.Phase != lsp.PhaseOff {
		t.Fatalf("absent language = %#v, found=%v", missing, found)
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	processStarted := now.Add(time.Second)
	initializeStarted := now.Add(2 * time.Second)
	readyAt := now.Add(3 * time.Second)
	actionAt := now.Add(4 * time.Second)
	runtimeStartedAt := now.Add(-time.Minute)
	next := lsp.TaskLanguageState{
		TaskID:                "task-lsp-roundtrip",
		Language:              "kotlin",
		Policy:                lsp.PolicyKeepWarm,
		Detected:              true,
		DetectionState:        lsp.DetectionPartial,
		DetectionScannedAt:    &now,
		DetectionTruncated:    true,
		Phase:                 lsp.PhaseReady,
		Generation:            7,
		RuntimeIncarnation:    "task-host-a",
		RuntimeStartedAt:      &runtimeStartedAt,
		RuntimeRevision:       12,
		ProcessStartedAt:      &processStarted,
		InitializeStartedAt:   &initializeStarted,
		ReadyAt:               &readyAt,
		LastTransitionAt:      readyAt,
		LastAction:            lsp.ActionRestart,
		LastActionAt:          &actionAt,
		LastStopReason:        "task_environment_stopped",
		LastRestartReason:     "user_restart",
		LastInitiator:         lsp.InitiatorUser,
		RestartRequired:       true,
		RestartRequiredReason: "workspace_roots_changed",
		ErrorCode:             "initialize_failed",
		ErrorMessage:          "task-host detail",
	}
	stored, err := repo.CompareAndUpdateTaskLSPLanguage(ctx, next, 0)
	if err != nil {
		t.Fatalf("insert task LSP state: %v", err)
	}
	if stored.Revision != 1 || stored.Generation != 7 || stored.CreatedAt.IsZero() || stored.UpdatedAt.IsZero() {
		t.Fatalf("stored state = %#v", stored)
	}

	loaded, found, err := repo.GetTaskLSPLanguage(ctx, next.TaskID, next.Language)
	if err != nil || !found {
		t.Fatalf("reload task LSP state: found=%v err=%v", found, err)
	}
	if loaded.Policy != next.Policy || loaded.DetectionState != next.DetectionState ||
		loaded.Generation != 7 || loaded.LastRestartReason != next.LastRestartReason ||
		loaded.RuntimeIncarnation != next.RuntimeIncarnation || loaded.RuntimeRevision != next.RuntimeRevision ||
		loaded.RuntimeStartedAt == nil || !loaded.RuntimeStartedAt.Equal(runtimeStartedAt) ||
		loaded.RestartRequiredReason != next.RestartRequiredReason || loaded.ErrorMessage != next.ErrorMessage {
		t.Fatalf("round-tripped state = %#v", loaded)
	}

	next = *loaded
	next.Phase = lsp.PhaseStopping
	if _, err := repo.CompareAndUpdateTaskLSPLanguage(ctx, next, 0); !errors.Is(err, lsp.ErrRevisionConflict) {
		t.Fatalf("stale revision error = %v, want ErrRevisionConflict", err)
	}
	updated, err := repo.CompareAndUpdateTaskLSPLanguage(ctx, next, 1)
	if err != nil {
		t.Fatalf("update task LSP state: %v", err)
	}
	if updated.Revision != 2 || updated.Phase != lsp.PhaseStopping {
		t.Fatalf("updated state = %#v", updated)
	}

	if _, err := db.Exec(db.Rebind(`UPDATE tasks SET archived_at = ? WHERE id = ?`), now, next.TaskID); err != nil {
		t.Fatalf("archive task: %v", err)
	}
	if _, found, err := repo.GetTaskLSPLanguage(ctx, next.TaskID, next.Language); err != nil || !found {
		t.Fatalf("archive discarded policy/evidence: found=%v err=%v", found, err)
	}
}

func TestTaskLSPGenerationAllocationIsMonotonic(t *testing.T) {
	repo, db := newTaskLSPTestRepo(t)
	seedTaskForStatusSummary(t, db, "task-lsp-generation", "workspace-lsp-generation")
	ctx := context.Background()
	acceptedAt := time.Now().UTC().Truncate(time.Microsecond)

	first, err := repo.AllocateTaskLSPGeneration(
		ctx,
		"task-lsp-generation",
		"kotlin",
		lsp.ActionStart,
		lsp.InitiatorUser,
		"user_start",
		acceptedAt,
	)
	if err != nil {
		t.Fatalf("allocate first generation: %v", err)
	}
	runtimeStartedAt := acceptedAt.Add(-time.Minute)
	withRuntime := *first
	withRuntime.RuntimeIncarnation = "task-host-before-restart"
	withRuntime.RuntimeStartedAt = &runtimeStartedAt
	withRuntime.RuntimeRevision = 9
	if _, err := repo.CompareAndUpdateTaskLSPLanguage(ctx, withRuntime, first.Revision); err != nil {
		t.Fatalf("seed runtime ordering evidence: %v", err)
	}
	second, err := repo.AllocateTaskLSPGeneration(
		ctx,
		"task-lsp-generation",
		"kotlin",
		lsp.ActionRestart,
		lsp.InitiatorAutomatic,
		"crash_recovery",
		acceptedAt.Add(time.Second),
	)
	if err != nil {
		t.Fatalf("allocate second generation: %v", err)
	}
	if first.Generation != 1 || first.Revision != 1 {
		t.Fatalf("first allocation = generation %d revision %d", first.Generation, first.Revision)
	}
	if second.Generation != 2 || second.Revision != 3 || second.LastRestartReason != "crash_recovery" {
		t.Fatalf("second allocation = %#v", second)
	}
	if second.RuntimeIncarnation != "" || second.RuntimeStartedAt != nil || second.RuntimeRevision != 0 {
		t.Fatalf("new generation retained runtime ordering evidence: %#v", second)
	}
	if second.Phase != lsp.PhaseStarting || second.LastInitiator != lsp.InitiatorAutomatic {
		t.Fatalf("second allocation evidence = %#v", second)
	}

	rows, err := repo.ListTaskLSPLanguages(ctx, "task-lsp-generation")
	if err != nil {
		t.Fatalf("list task LSP languages: %v", err)
	}
	if len(rows) != 1 || rows[0].Generation != 2 {
		t.Fatalf("listed task LSP languages = %#v", rows)
	}
}

func TestTaskLSPCASReturnsItsOwnWriteWithoutConcurrentReread(t *testing.T) {
	repo, db := newTaskLSPTestRepo(t)
	seedTaskForStatusSummary(t, db, "task-lsp-returning", "workspace-lsp-returning")
	ctx := context.Background()
	if _, err := db.Exec(`
		CREATE TRIGGER task_lsp_after_insert AFTER INSERT ON task_lsp_languages
		BEGIN
			UPDATE task_lsp_languages
			SET revision = revision + 10, error_message = 'after-insert'
			WHERE task_id = NEW.task_id AND language = NEW.language;
		END
	`); err != nil {
		t.Fatalf("create insert trigger: %v", err)
	}
	state := lsp.DefaultTaskLanguageState("task-lsp-returning", "go")
	inserted, err := repo.CompareAndUpdateTaskLSPLanguage(ctx, state, 0)
	if err != nil {
		t.Fatalf("insert state: %v", err)
	}
	if inserted.Revision != 1 || inserted.ErrorMessage != "" {
		t.Fatalf("insert returned a later writer's row: %#v", inserted)
	}
	if _, err := db.Exec(`DROP TRIGGER task_lsp_after_insert`); err != nil {
		t.Fatal(err)
	}
	loaded, found, err := repo.GetTaskLSPLanguage(ctx, state.TaskID, state.Language)
	if err != nil || !found {
		t.Fatalf("load triggered row: found=%v err=%v", found, err)
	}
	if _, err := db.Exec(`
		CREATE TRIGGER task_lsp_after_update AFTER UPDATE ON task_lsp_languages
		BEGIN
			UPDATE task_lsp_languages
			SET revision = revision + 10, error_message = 'after-update'
			WHERE task_id = NEW.task_id AND language = NEW.language;
		END
	`); err != nil {
		t.Fatalf("create update trigger: %v", err)
	}
	loaded.ErrorMessage = "caller-write"
	updated, err := repo.CompareAndUpdateTaskLSPLanguage(ctx, *loaded, loaded.Revision)
	if err != nil {
		t.Fatalf("update state: %v", err)
	}
	if updated.Revision != loaded.Revision+1 || updated.ErrorMessage != "caller-write" {
		t.Fatalf("update returned a later writer's row: %#v", updated)
	}
}

func newTaskLSPTestRepo(t *testing.T) (*Repository, *sqlx.DB) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "task-lsp-repository.db")
	dbConn, err := dbutil.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("initialize schema: %v", err)
	}
	return repo, db
}
