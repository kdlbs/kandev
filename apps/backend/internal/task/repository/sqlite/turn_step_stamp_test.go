package sqlite

// CreateTurnWithStepStamp coverage: the turn-start step stamp is read and
// the turn row inserted inside one transaction, so the read is serialized
// against concurrent step movers (see turn_step_stamp_postgres_test.go for
// the concurrency proof) rather than a plain unlocked read taken before a
// separate insert. This file covers the single-writer correctness cases;
// task/service/service_turns_step_stamp_test.go covers the service-layer
// wiring (runtime-config-snapshot metadata, StartTurn/createCompletedTurn).

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	dbutil "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
)

func newTurnStepStampTestRepo(t *testing.T) *Repository {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "turn-step-stamp.db")
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
	return repo
}

func seedTurnStepStampSession(t *testing.T, repo *Repository, taskID, stepID, sessionID string) {
	t.Helper()
	ctx := t.Context()
	now := time.Now().UTC()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-turn-stamp", Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	task := &models.Task{ID: taskID, WorkspaceID: "ws-turn-stamp", Title: "Turn stamp task", Priority: "medium"}
	if stepID != "" {
		if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-turn-stamp", WorkspaceID: "ws-turn-stamp", Name: "Workflow"}); err != nil {
			t.Fatalf("CreateWorkflow: %v", err)
		}
		task.WorkflowID = "wf-turn-stamp"
		task.WorkflowStepID = stepID
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: taskID, State: models.TaskSessionStateRunning,
		StartedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
}

func TestCreateTurnWithStepStampStampsCurrentStep(t *testing.T) {
	repo := newTurnStepStampTestRepo(t)
	seedTurnStepStampSession(t, repo, "task-has-step", "step-a", "session-has-step")

	turn := &models.Turn{TaskSessionID: "session-has-step", TaskID: "task-has-step"}
	stamped, err := repo.CreateTurnWithStepStamp(t.Context(), turn)
	if err != nil {
		t.Fatalf("CreateTurnWithStepStamp: %v", err)
	}
	if !stamped {
		t.Fatal("stamped = false, want true")
	}
	if turn.Metadata[models.TurnMetaKeyWorkflowStepIDAtStart] != "step-a" {
		t.Fatalf("stamp = %v, want %q", turn.Metadata[models.TurnMetaKeyWorkflowStepIDAtStart], "step-a")
	}

	stored, err := repo.GetTurn(t.Context(), turn.ID)
	if err != nil {
		t.Fatalf("GetTurn: %v", err)
	}
	if stored.Metadata[models.TurnMetaKeyWorkflowStepIDAtStart] != "step-a" {
		t.Fatalf("persisted stamp = %v, want %q", stored.Metadata[models.TurnMetaKeyWorkflowStepIDAtStart], "step-a")
	}
}

func TestCreateTurnWithStepStampOmitsStampWhenTaskHasNoStep(t *testing.T) {
	repo := newTurnStepStampTestRepo(t)
	seedTurnStepStampSession(t, repo, "task-no-step", "", "session-no-step")

	turn := &models.Turn{TaskSessionID: "session-no-step", TaskID: "task-no-step"}
	stamped, err := repo.CreateTurnWithStepStamp(t.Context(), turn)
	if err != nil {
		t.Fatalf("CreateTurnWithStepStamp: %v", err)
	}
	if stamped {
		t.Fatal("stamped = true, want false (task has no workflow step)")
	}
	if _, ok := turn.Metadata[models.TurnMetaKeyWorkflowStepIDAtStart]; ok {
		t.Fatalf("turn metadata carries a stamp: %v", turn.Metadata)
	}
}

// TestCreateTurnWithStepStampOmitsStampWhenTaskNotFound pins the spec's
// failure mode "the task row cannot be read when a turn starts: the turn is
// still created, the stamp key is absent, turn creation MUST NOT fail" for
// the realistic case — the turn's own task_id names a task that doesn't
// exist. The session's own task exists (task_sessions.task_id has no FK
// requirement on task_session_turns.task_id, so this is legal and lets the
// test stay deterministic without corrupting the database to force a
// genuine transactional read error).
func TestCreateTurnWithStepStampOmitsStampWhenTaskNotFound(t *testing.T) {
	repo := newTurnStepStampTestRepo(t)
	seedTurnStepStampSession(t, repo, "task-real", "step-a", "session-dangling-task")

	turn := &models.Turn{TaskSessionID: "session-dangling-task", TaskID: "task-does-not-exist"}
	stamped, err := repo.CreateTurnWithStepStamp(t.Context(), turn)
	if err != nil {
		t.Fatalf("CreateTurnWithStepStamp should not fail when the task row is missing: %v", err)
	}
	if stamped {
		t.Fatal("stamped = true, want false (task row does not exist)")
	}

	stored, err := repo.GetTurn(t.Context(), turn.ID)
	if err != nil {
		t.Fatalf("GetTurn: turn was not persisted despite the missing task row: %v", err)
	}
	if _, ok := stored.Metadata[models.TurnMetaKeyWorkflowStepIDAtStart]; ok {
		t.Fatalf("persisted turn carries a stamp: %v", stored.Metadata)
	}
}

func TestCreateTurnWithStepStampPreservesCallerSuppliedMetadata(t *testing.T) {
	repo := newTurnStepStampTestRepo(t)
	seedTurnStepStampSession(t, repo, "task-preserve-meta", "step-a", "session-preserve-meta")

	turn := &models.Turn{
		TaskSessionID: "session-preserve-meta",
		TaskID:        "task-preserve-meta",
		Metadata:      map[string]interface{}{"runtime_config_snapshot": map[string]interface{}{"model": "opus"}},
	}
	stamped, err := repo.CreateTurnWithStepStamp(t.Context(), turn)
	if err != nil {
		t.Fatalf("CreateTurnWithStepStamp: %v", err)
	}
	if !stamped {
		t.Fatal("stamped = false, want true")
	}

	stored, err := repo.GetTurn(t.Context(), turn.ID)
	if err != nil {
		t.Fatalf("GetTurn: %v", err)
	}
	if stored.Metadata[models.TurnMetaKeyWorkflowStepIDAtStart] != "step-a" {
		t.Fatalf("stamp = %v, want %q", stored.Metadata[models.TurnMetaKeyWorkflowStepIDAtStart], "step-a")
	}
	snapshot, ok := stored.Metadata["runtime_config_snapshot"].(map[string]interface{})
	if !ok || snapshot["model"] != "opus" {
		t.Fatalf("runtime_config_snapshot lost: %v", stored.Metadata)
	}
}
