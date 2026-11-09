package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
)

const legacyBackgroundWorkTable = `CREATE TABLE task_session_background_work (
	id TEXT NOT NULL,
	task_id TEXT NOT NULL,
	session_id TEXT NOT NULL,
	kind TEXT NOT NULL DEFAULT 'unknown',
	title TEXT NOT NULL DEFAULT '',
	state TEXT NOT NULL DEFAULT 'unknown',
	parent_work_id TEXT,
	origin_turn_id TEXT NOT NULL DEFAULT '',
	source_message_id TEXT NOT NULL DEFAULT '',
	source_call_id TEXT NOT NULL DEFAULT '',
	exit_code INTEGER,
	output TEXT NOT NULL DEFAULT '',
	output_truncated INTEGER NOT NULL DEFAULT 0,
	output_offset INTEGER NOT NULL DEFAULT 0,
	started_at TIMESTAMP,
	finished_at TIMESTAMP,
	revision INTEGER NOT NULL DEFAULT 0,
	created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	PRIMARY KEY (session_id, id)
)`

// TestBackgroundWorkCapabilitiesColumnMigratesLegacyTable pins the additive
// migration: an existing database with the pre-capability shape gains the
// column on boot and replaying the schema is safe.
func TestBackgroundWorkCapabilitiesColumnMigratesLegacyTable(t *testing.T) {
	raw, err := db.OpenSQLite(filepath.Join(t.TempDir(), "legacy-background.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	database := sqlx.NewDb(raw, "sqlite3")
	t.Cleanup(func() { _ = database.Close() })
	if _, err := database.Exec(legacyBackgroundWorkTable); err != nil {
		t.Fatalf("create legacy table: %v", err)
	}

	repo, err := NewWithDB(database, database, nil)
	if err != nil {
		t.Fatalf("initialize schema over legacy table: %v", err)
	}
	exists, err := db.ColumnExists(database, "task_session_background_work", "capabilities")
	if err != nil || !exists {
		t.Fatalf("capabilities column missing after migration: exists=%v err=%v", exists, err)
	}
	if err := repo.initBackgroundWorkSchema(); err != nil {
		t.Fatalf("replay background schema: %v", err)
	}
	if err := repo.runMigrations(context.Background()); err != nil {
		t.Fatalf("replay migrations: %v", err)
	}
}

// TestBackgroundWorkCapabilitiesPersistence pins that a recorded declaration
// survives a write/read and is not erased by a later update without one.
func TestBackgroundWorkCapabilitiesPersistence(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()

	wsID := uuid.New().String()
	taskID := uuid.New().String()
	sessionID := uuid.New().String()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: wsID, Name: "WS"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: wsID, Title: "Task"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: sessionID, TaskID: taskID}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	declaration := `{"discovery":"snapshot","output":"stream"}`
	workload := &models.BackgroundWorkload{
		ID:           "work-1",
		TaskID:       taskID,
		SessionID:    sessionID,
		Kind:         "shell",
		Title:        "build",
		State:        "running",
		Capabilities: declaration,
		StartedAt:    &now,
		Revision:     1,
	}
	if err := repo.UpsertBackgroundWorkload(ctx, workload); err != nil {
		t.Fatalf("UpsertBackgroundWorkload: %v", err)
	}

	// A later state update with no declaration must keep the stored one.
	update := *workload
	update.State = "unknown"
	update.Capabilities = ""
	update.Revision = 2
	if err := repo.UpsertBackgroundWorkload(ctx, &update); err != nil {
		t.Fatalf("UpsertBackgroundWorkload update: %v", err)
	}

	stored, err := repo.GetBackgroundWorkload(ctx, sessionID, "work-1")
	if err != nil || stored == nil {
		t.Fatalf("GetBackgroundWorkload: %v %v", stored, err)
	}
	if stored.Capabilities != declaration {
		t.Fatalf("capabilities = %q, want preserved declaration", stored.Capabilities)
	}
	if stored.State != "unknown" {
		t.Fatalf("state = %q, want unknown", stored.State)
	}
}

// TestBackgroundWorkSourceIdentitySurvivesUpdateWithoutIt pins that SQL does
// not erase immutable launch identity when an update omits it.
func TestBackgroundWorkSourceIdentitySurvivesUpdateWithoutIt(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()

	wsID := uuid.New().String()
	taskID := uuid.New().String()
	sessionID := uuid.New().String()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: wsID, Name: "WS"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: wsID, Title: "Task"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: sessionID, TaskID: taskID}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}

	parent := "parent-1"
	workload := &models.BackgroundWorkload{
		ID:              "work-1",
		TaskID:          taskID,
		SessionID:       sessionID,
		Kind:            "shell",
		Title:           "build",
		State:           "running",
		ParentWorkID:    &parent,
		SourceCallID:    "call-1",
		SourceMessageID: "message-1",
		Revision:        1,
	}
	if err := repo.UpsertBackgroundWorkload(ctx, workload); err != nil {
		t.Fatalf("UpsertBackgroundWorkload: %v", err)
	}

	update := *workload
	update.State = "unknown"
	update.ParentWorkID = nil
	update.SourceCallID = ""
	update.SourceMessageID = ""
	update.Revision = 2
	if err := repo.UpsertBackgroundWorkload(ctx, &update); err != nil {
		t.Fatalf("UpsertBackgroundWorkload update: %v", err)
	}

	stored, err := repo.GetBackgroundWorkload(ctx, sessionID, "work-1")
	if err != nil || stored == nil {
		t.Fatalf("GetBackgroundWorkload: %v %v", stored, err)
	}
	if stored.SourceCallID != "call-1" || stored.SourceMessageID != "message-1" {
		t.Fatalf("source identity erased: %+v", stored)
	}
	if stored.ParentWorkID == nil || *stored.ParentWorkID != parent {
		t.Fatalf("parent identity erased: %+v", stored.ParentWorkID)
	}
}
