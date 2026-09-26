package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

// failingCreateTurnRepo wraps a real TurnRepository and fails
// CreateTurnWithStepStamp unconditionally, so a test can exercise "turn
// persistence failed" without a real database-level failure.
type failingCreateTurnRepo struct {
	repository.TurnRepository
}

func (f *failingCreateTurnRepo) CreateTurnWithStepStamp(ctx context.Context, turn *models.Turn) (bool, error) {
	return false, errors.New("simulated turn create failure")
}

// setupStepStampTask creates a workspace/workflow/task fixture with the given
// workflow step (empty string means "no step").
func setupStepStampTask(t *testing.T, repo *sqliterepo.Repository, taskID, stepID string) {
	t.Helper()
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-stamp", Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-stamp", WorkspaceID: "ws-stamp", Name: "Workflow"}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	task := &models.Task{ID: taskID, WorkspaceID: "ws-stamp", Title: "Stamp Task", Priority: "medium"}
	if stepID != "" {
		task.WorkflowID = "wf-stamp"
		task.WorkflowStepID = stepID
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
}

func createStepStampSession(t *testing.T, repo *sqliterepo.Repository, id, taskID string) *models.TaskSession {
	t.Helper()
	ctx := context.Background()
	session := &models.TaskSession{
		ID:        id,
		TaskID:    taskID,
		State:     models.TaskSessionStateRunning,
		StartedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := repo.CreateTaskSession(ctx, session); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	return session
}

func TestStartTurnStampsWorkflowStepIDAtStart(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	setupStepStampTask(t, repo, "task-stamp-has-step", "step-a")
	session := createStepStampSession(t, repo, "session-stamp-has-step", "task-stamp-has-step")

	turn, err := svc.StartTurn(ctx, session.ID)
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}

	stamped, ok := turn.Metadata[models.TurnMetaKeyWorkflowStepIDAtStart]
	if !ok {
		t.Fatal("turn metadata missing workflow_step_id_at_start key")
	}
	if stamped != "step-a" {
		t.Fatalf("workflow_step_id_at_start = %v, want %q", stamped, "step-a")
	}
}

func TestStartTurnOmitsStampWhenTaskHasNoStep(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	setupStepStampTask(t, repo, "task-stamp-no-step", "")
	session := createStepStampSession(t, repo, "session-stamp-no-step", "task-stamp-no-step")

	turn, err := svc.StartTurn(ctx, session.ID)
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}

	if _, ok := turn.Metadata[models.TurnMetaKeyWorkflowStepIDAtStart]; ok {
		t.Fatalf("turn metadata carries workflow_step_id_at_start = %v, want key absent", turn.Metadata[models.TurnMetaKeyWorkflowStepIDAtStart])
	}
}

func TestStartTurnStampsWithNoRuntimeConfigToSnapshot(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	setupStepStampTask(t, repo, "task-stamp-no-runtime", "step-b")
	session := createStepStampSession(t, repo, "session-stamp-no-runtime", "task-stamp-no-runtime")

	turn, err := svc.StartTurn(ctx, session.ID)
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}

	if turn.Metadata[models.TurnMetaKeyWorkflowStepIDAtStart] != "step-b" {
		t.Fatalf("workflow_step_id_at_start = %v, want %q", turn.Metadata[models.TurnMetaKeyWorkflowStepIDAtStart], "step-b")
	}
	if _, ok := turn.Metadata[models.TurnMetaKeyRuntimeConfigSnapshot]; ok {
		t.Fatal("turn metadata carries runtime_config_snapshot, want absent when there is nothing to snapshot")
	}
}

func TestStartTurnStampIsImmutableAcrossMidTurnMove(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	setupStepStampTask(t, repo, "task-stamp-move", "step-a")
	session := createStepStampSession(t, repo, "session-stamp-move", "task-stamp-move")

	turn, err := svc.StartTurn(ctx, session.ID)
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	if turn.Metadata[models.TurnMetaKeyWorkflowStepIDAtStart] != "step-a" {
		t.Fatalf("initial stamp = %v, want %q", turn.Metadata[models.TurnMetaKeyWorkflowStepIDAtStart], "step-a")
	}

	task, err := repo.GetTask(ctx, "task-stamp-move")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	task.WorkflowStepID = "step-b"
	if err := repo.UpdateTask(ctx, task); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}

	stored, err := repo.GetTurn(ctx, turn.ID)
	if err != nil {
		t.Fatalf("GetTurn: %v", err)
	}
	if stored.Metadata[models.TurnMetaKeyWorkflowStepIDAtStart] != "step-a" {
		t.Fatalf("stamp after mid-turn move = %v, want unchanged %q", stored.Metadata[models.TurnMetaKeyWorkflowStepIDAtStart], "step-a")
	}
}

// "Task read fails during turn-start stamping degrades gracefully" was
// tested here via a wrapped TaskRepository until CreateTurnWithStepStamp
// moved the step read into the same repository transaction as the turn
// insert (see that method's doc comment) to close a race the turn-start
// read previously had against a concurrent step mover. s.tasks.GetTask is
// no longer part of this path at all, so the old fault-injection seam is
// dead; the equivalent guarantee — a missing/unreadable task row degrades
// to an unstamped turn rather than failing turn creation — is now pinned
// at the repository layer where the read actually happens:
// TestCreateTurnWithStepStampOmitsStampWhenTaskNotFound in
// internal/task/repository/sqlite/turn_step_stamp_test.go.

// TestStartTurnDoesNotRecordStampMetricWhenCreateTurnFails pins Review round
// 4's must-fix: steptelemetry.RecordTurnStamp used to fire inside
// turnStartMetadata, before CreateTurn ever ran, so a turn-creation failure
// still counted as "stamped"/"unstamped" in the telemetry_turn_stamps_total
// counter and its log line — contradicting the writer-health spec's "of
// turns created" framing. The metric must be recorded only once the turn is
// actually persisted.
func TestStartTurnDoesNotRecordStampMetricWhenCreateTurnFails(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	setupStepStampTask(t, repo, "task-stamp-create-fails", "step-a")
	session := createStepStampSession(t, repo, "session-stamp-create-fails", "task-stamp-create-fails")

	core, logs := observer.New(zapcore.DebugLevel)
	observedLog, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("NewFromZap: %v", err)
	}
	svc.logger = observedLog
	svc.turns = &failingCreateTurnRepo{TurnRepository: repo}

	if _, err := svc.StartTurn(ctx, session.ID); err == nil {
		t.Fatal("expected StartTurn to surface the injected CreateTurn failure")
	}

	if entries := logs.FilterMessage("telemetry.metric.turn_stamped").All(); len(entries) != 0 {
		t.Fatalf("turn_stamped metric fired despite CreateTurn failing: %d entries", len(entries))
	}
}

func TestTurnStampAndLedgerRowCanDisagreeWithoutError(t *testing.T) {
	// GIVEN a turn stamped with step S and a ledger row moving the task to T
	// a moment after that turn started, WHEN both are read, THEN they
	// disagree, and neither is treated as an error.
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	setupStepStampTask(t, repo, "task-stamp-vs-ledger", "step-a")
	session := createStepStampSession(t, repo, "session-stamp-vs-ledger", "task-stamp-vs-ledger")

	turn, err := svc.StartTurn(ctx, session.ID)
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	if turn.Metadata[models.TurnMetaKeyWorkflowStepIDAtStart] != "step-a" {
		t.Fatalf("turn stamp = %v, want %q", turn.Metadata[models.TurnMetaKeyWorkflowStepIDAtStart], "step-a")
	}

	task, err := repo.GetTask(ctx, "task-stamp-vs-ledger")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	task.WorkflowStepID = "step-b"
	if err := repo.UpdateTask(ctx, task); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}

	var toStep *string
	row := repo.DB().QueryRowContext(ctx, `
		SELECT to_workflow_step_id FROM task_step_transitions
		WHERE task_id = ? ORDER BY occurred_at DESC, id DESC LIMIT 1
	`, "task-stamp-vs-ledger")
	if err := row.Scan(&toStep); err != nil {
		t.Fatalf("scan last ledger row: %v", err)
	}
	if toStep == nil || *toStep != "step-b" {
		t.Fatalf("last ledger row's to_workflow_step_id = %v, want %q", toStep, "step-b")
	}

	stored, err := repo.GetTurn(ctx, turn.ID)
	if err != nil {
		t.Fatalf("GetTurn: %v", err)
	}
	if stored.Metadata[models.TurnMetaKeyWorkflowStepIDAtStart] == *toStep {
		t.Fatalf("expected turn stamp %q and ledger's last step %q to disagree",
			stored.Metadata[models.TurnMetaKeyWorkflowStepIDAtStart], *toStep)
	}
}

func TestCreateCompletedTurnCarriesSameStamp(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	setupStepStampTask(t, repo, "task-stamp-completed", "step-c")
	session := createStepStampSession(t, repo, "session-stamp-completed", "task-stamp-completed")

	turn, err := svc.createCompletedTurn(ctx, session)
	if err != nil {
		t.Fatalf("createCompletedTurn: %v", err)
	}
	if turn.Metadata[models.TurnMetaKeyWorkflowStepIDAtStart] != "step-c" {
		t.Fatalf("synthetic completed turn stamp = %v, want %q", turn.Metadata[models.TurnMetaKeyWorkflowStepIDAtStart], "step-c")
	}
}

func TestCreateCompletedTurnMarksLifecycleOnly(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	setupStepStampTask(t, repo, "task-lifecycle-only", "step-d")
	session := createStepStampSession(t, repo, "session-lifecycle-only", "task-lifecycle-only")

	turn, err := svc.createCompletedTurn(ctx, session)
	if err != nil {
		t.Fatalf("createCompletedTurn: %v", err)
	}
	if turn.Metadata[models.TurnMetaKeyLifecycleOnly] != true {
		t.Fatalf("synthetic completed turn lifecycle_only = %v, want true", turn.Metadata[models.TurnMetaKeyLifecycleOnly])
	}

	stored, err := repo.GetTurn(ctx, turn.ID)
	if err != nil {
		t.Fatalf("GetTurn: %v", err)
	}
	if stored.Metadata[models.TurnMetaKeyLifecycleOnly] != true {
		t.Fatalf("persisted completed turn lifecycle_only = %v, want true", stored.Metadata[models.TurnMetaKeyLifecycleOnly])
	}
}
