package coordinator

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// noTurnStartCase is one row of noTurnStartPaths: a backend path that must
// never start a turn (send a prompt or launch an agent) for a coordinator
// conversation task (docs/specs/coordinator/system-design/copilot.md
// #attended-only). Each row exercises the real production path and asserts
// no session/agent start happened.
type noTurnStartCase struct {
	name string
	run  func(t *testing.T)
}

// noTurnStartPaths is appended to by whichever of tasks 03, 04 and 07 owns
// each backend path that could start a turn for a coordinator task, so the
// combined table is complete regardless of merge order. Task 03 (this work
// package) owns the conversation route, the startup cleanup pass and
// session recovery on restart; task 04 owns the stall and
// workspace.deleted subscribers; task 07 owns proposal decisions.
var noTurnStartPaths = []noTurnStartCase{
	{name: "conversation route", run: noTurnStartConversationRoute},
	{name: "startup cleanup pass", run: noTurnStartStartupCleanup},
	{name: "session recovery on restart", run: noTurnStartSessionRecovery},
	{name: "wake recovery duties", run: noTurnStartWakeBackstop},
	{name: "wake delivery with admission held", run: noTurnStartWakeAdmissionHeld},
	{name: "wake delivery with admission passed", run: noTurnStartWakeDelivery},
}

// TestCoordinatorConversationNoTurnStart proves that no backend path capable
// of starting a turn ever does so for a coordinator conversation task
// (copilot.md#attended-only): only an explicit manager message.add may.
func TestCoordinatorConversationNoTurnStart(t *testing.T) {
	for _, tc := range noTurnStartPaths {
		t.Run(tc.name, tc.run)
	}
}

// noTurnStartConversationRoute proves OpenConversation ensures a session
// without ever allowing it to auto-start an agent (copilot.md#conversation-
// task, step 6): it always passes AutoStart=false to SessionEnsurer.
func noTurnStartConversationRoute(t *testing.T) {
	deps := newConversationTestDeps(t)
	ctx := context.Background()

	if _, err := deps.svc.OpenConversation(ctx, testWorkspaceID, deps.coordinator.ID); err != nil {
		t.Fatalf("OpenConversation() error = %v", err)
	}
	if len(deps.sessions.calls) != 1 {
		t.Fatalf("EnsureSession calls = %d, want 1", len(deps.sessions.calls))
	}
	if deps.sessions.lastOpts.AutoStart == nil || *deps.sessions.lastOpts.AutoStart {
		t.Error("EnsureSession AutoStart option, want explicit false")
	}
}

// noTurnStartStartupCleanup proves the startup cleanup pass only archives
// and deletes tasks: it never calls the session ensurer
// (copilot.md#attended-only).
func noTurnStartStartupCleanup(t *testing.T) {
	deps := newConversationTestDeps(t)
	ctx := context.Background()

	orphan := &taskmodels.Task{
		ID:          "orphan-task",
		WorkspaceID: testWorkspaceID,
		Origin:      taskmodels.TaskOriginCoordinator,
		CreatedAt:   time.Now().UTC().Add(-time.Hour),
		Metadata:    map[string]interface{}{taskmodels.MetaKeyCoordinatorID: "does-not-exist"},
	}
	deps.tasks.tasks[orphan.ID] = orphan

	deps.svc.CleanupConversationTasks(ctx, time.Now().UTC())

	if len(deps.tasks.deletedIDs) != 1 || deps.tasks.deletedIDs[0] != orphan.ID {
		t.Fatalf("deletedIDs = %v, want [%s]: cleanup pass did not repair the orphaned task", deps.tasks.deletedIDs, orphan.ID)
	}
	if len(deps.sessions.calls) != 0 {
		t.Errorf("EnsureSession calls = %d, want 0: startup cleanup must never touch sessions", len(deps.sessions.calls))
	}
}

// noTurnStartSessionRecovery proves that the orchestrator's own restart-time
// session-status/recovery path (GetTaskSessionStatus, which backstops both
// startup recovery and reconnect) reports auto-resume as blocked for a
// coordinator conversation task, rather than starting one
// (copilot.md#attended-only). It exercises the real orchestrator.Service
// (not a fake), backed by a real SQLite task repository, so a future
// regression in autoResumeEligibility's coordinator check is caught here
// too, not only by internal/orchestrator's own unit test.
func noTurnStartSessionRecovery(t *testing.T) {
	ctx := context.Background()
	repo := newOrchestratorTestRepo(t)
	log := newTestLogger(t)

	task := &taskmodels.Task{
		ID:          "coordinator-task-recovery",
		WorkspaceID: testWorkspaceID,
		Title:       "Coordinator: Recovery",
		Origin:      taskmodels.TaskOriginCoordinator,
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	session := &taskmodels.TaskSession{
		ID:     "coordinator-session-recovery",
		TaskID: task.ID,
		State:  taskmodels.TaskSessionStateWaitingForInput,
	}
	if err := repo.CreateTaskSession(ctx, session); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	eventBus := bus.NewMemoryEventBus(log)
	svc := orchestrator.NewService(
		orchestrator.DefaultServiceConfig(), eventBus, nil, &schedulerRepoAdapter{repo: repo}, repo, nil, nil, nil, log,
	)

	status, err := svc.GetTaskSessionStatus(ctx, task.ID, session.ID)
	if err != nil {
		t.Fatalf("GetTaskSessionStatus() error = %v", err)
	}
	if status.AutoResumeAllowed {
		t.Error("AutoResumeAllowed = true, want false for a coordinator conversation task")
	}
	if status.AutoResumeBlockedReason != "coordinator_message_only" {
		t.Errorf("AutoResumeBlockedReason = %q, want %q", status.AutoResumeBlockedReason, "coordinator_message_only")
	}
}

// newOrchestratorTestRepo builds a real, schema-initialized SQLite task
// repository for noTurnStartSessionRecovery: the coordinator-origin check
// this test guards lives in internal/orchestrator and is only reachable
// through the orchestrator.Service's real repoStore dependency, not through
// this package's fakes.
func newOrchestratorTestRepo(t *testing.T) *sqliterepo.Repository {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "orchestrator-recovery.db")

	writerConn, err := db.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open writer: %v", err)
	}
	t.Cleanup(func() { _ = writerConn.Close() })
	writer := sqlx.NewDb(writerConn, "sqlite3")

	readerConn, err := db.OpenSQLiteReader(dbPath)
	if err != nil {
		t.Fatalf("open reader: %v", err)
	}
	t.Cleanup(func() { _ = readerConn.Close() })
	reader := sqlx.NewDb(readerConn, "sqlite3")

	repo, err := sqliterepo.NewWithDB(writer, reader, newTestLogger(t))
	if err != nil {
		t.Fatalf("new task repository: %v", err)
	}
	return repo
}

// schedulerRepoAdapter narrows *sqliterepo.Repository to
// scheduler.TaskRepository. The scheduler's queue-processing loop is never
// started by noTurnStartSessionRecovery, so only GetTask is ever actually
// called; the state-transition methods are wired for interface satisfaction
// (mirroring internal/backendapp's production adapter).
type schedulerRepoAdapter struct {
	repo *sqliterepo.Repository
}

func (a *schedulerRepoAdapter) GetTask(ctx context.Context, taskID string) (*v1.Task, error) {
	task, err := a.repo.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	return task.ToAPI(), nil
}

func (a *schedulerRepoAdapter) UpdateTaskState(ctx context.Context, taskID string, state v1.TaskState) error {
	return a.repo.UpdateTaskState(ctx, taskID, state)
}

func (a *schedulerRepoAdapter) UpdateTaskStateIfCurrentIn(
	ctx context.Context, taskID string, state v1.TaskState, allowed []v1.TaskState,
) (bool, error) {
	_, updated, err := a.repo.UpdateTaskStateIfCurrentIn(ctx, taskID, state, allowed)
	return updated, err
}

func (a *schedulerRepoAdapter) UpdateTaskStateIfNotArchived(
	ctx context.Context, taskID string, state v1.TaskState,
) (bool, error) {
	_, updated, err := a.repo.UpdateTaskStateIfNotArchived(ctx, taskID, state)
	return updated, err
}

func (a *schedulerRepoAdapter) UpdateTaskStateIfSessionState(
	ctx context.Context,
	taskID, sessionID string,
	expectedSessionState taskmodels.TaskSessionState,
	state v1.TaskState,
) (bool, error) {
	_, updated, err := a.repo.UpdateTaskStateIfSessionState(ctx, taskID, sessionID, expectedSessionState, state)
	return updated, err
}
