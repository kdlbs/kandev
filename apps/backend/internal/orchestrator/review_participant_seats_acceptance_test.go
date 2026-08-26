package orchestrator

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/db"
	officeengineadapters "github.com/kandev/kandev/internal/office/engine_adapters"
	officemodels "github.com/kandev/kandev/internal/office/models"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	workflowadapters "github.com/kandev/kandev/internal/workflow/adapters"
	"github.com/kandev/kandev/internal/workflow/engine"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	workflowrepository "github.com/kandev/kandev/internal/workflow/repository"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// This file exercises AC-OFFICE-REVIEW-SEATS-005.1/.2/.3 end to end against
// the real, materialized office-default template: a task arriving at Review
// gets a decision-required reviewer seat with no manual DB work, and that
// reviewer has a run queued on step entry. Two scenarios share the same
// wiring but reach Review by different routes:
//
//   - "product route": a live-session task transitions Work -> Review via
//     the engine (Service.processOnTurnCompleteViaEngine).
//   - "manual route": a task with zero task_sessions rows is moved directly
//     via Repository.UpdateTask, representing an operator move with no
//     agent turn in flight.
//
// Both routes commit through the same Repository.UpdateTask writer, which
// synchronously fires the WP1 step-entry dispatcher after commit — so both
// are expected to produce an identical reviewer seat and queued run.

// fakeReviewSeatsOfficeRepo is a minimal engine_adapters.OfficeRepo double
// with no CEO candidates, forcing SeatCasterAdapter's runner-fallback path.
// The casting algorithm itself is already covered by
// internal/office/engine_adapters/seat_caster_test.go; this test proves
// plumbing (DispatchStepEntry -> EnsureParticipantSeatCallback ->
// ParticipantSeatWriterAdapter -> real DB row), not casting logic.
type fakeReviewSeatsOfficeRepo struct {
	workspaceID string
}

func (f *fakeReviewSeatsOfficeRepo) GetTaskExecutionFields(
	_ context.Context, taskID string,
) (*officesqlite.TaskExecutionFields, error) {
	return &officesqlite.TaskExecutionFields{ID: taskID, WorkspaceID: f.workspaceID}, nil
}

func (f *fakeReviewSeatsOfficeRepo) ListAgentInstancesFiltered(
	_ context.Context, _ string, _ officesqlite.AgentListFilter,
) ([]*officemodels.AgentInstance, error) {
	return nil, nil
}

// testStepEntryDispatcher mirrors backendapp's production
// engineStepEntryDispatcherAdapter: it delegates to the same *engine.Engine
// instance the Service uses for HandleTrigger, so a manual UpdateTask call
// and an engine-driven transition dispatch identically.
type testStepEntryDispatcher struct {
	eng *engine.Engine
}

func (d *testStepEntryDispatcher) DispatchStepEntry(ctx context.Context, taskID, workflowID, stepID, entryID string) {
	d.eng.DispatchStepEntry(ctx, taskID, workflowID, stepID, entryID)
}

// reviewSeatsTestEnv wires a real task repository and a real workflow
// repository against one shared SQLite connection (mirroring
// backendapp.provideRepositories), a Service with every seat-writing engine
// dependency set, and the office-default template materialized for one
// workspace.
type reviewSeatsTestEnv struct {
	taskRepo     *sqliterepo.Repository
	workflowRepo *workflowrepository.Repository
	svc          *Service
	mockTaskRepo *mockTaskRepo
	runQueue     *fakeRunQueueAdapter
	workspaceID  string
	workflowID   string
	workStepID   string
	reviewStepID string
}

func newReviewSeatsTestEnv(t *testing.T) *reviewSeatsTestEnv {
	t.Helper()
	ctx := context.Background()
	log := testLogger()

	tmpDir := t.TempDir()
	dbConn, err := db.OpenSQLite(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = sqlxDB.Close() })

	taskRepo, cleanup, err := repository.Provide(sqlxDB, sqlxDB, log)
	if err != nil {
		t.Fatalf("create task repository: %v", err)
	}
	t.Cleanup(func() { _ = cleanup() })

	workflowRepo, err := workflowrepository.NewWithDB(sqlxDB, sqlxDB, log)
	if err != nil {
		t.Fatalf("create workflow repository: %v", err)
	}

	workspaceID := "ws-review-seats"
	now := time.Now().UTC()
	if err := taskRepo.CreateWorkspace(ctx, &models.Workspace{
		ID: workspaceID, Name: "Review Seats", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	workflowID, err := taskRepo.EnsureOfficeDefaultWorkflow(ctx, workspaceID)
	if err != nil {
		t.Fatalf("materialize office-default workflow: %v", err)
	}

	steps, err := workflowRepo.ListStepsByWorkflow(ctx, workflowID)
	if err != nil {
		t.Fatalf("list workflow steps: %v", err)
	}
	sg := newMockStepGetter()
	var workStepID, reviewStepID string
	for _, s := range steps {
		sg.steps[s.ID] = s
		switch s.Name {
		case "Work":
			workStepID = s.ID
		case "Review":
			reviewStepID = s.ID
		}
	}
	if workStepID == "" || reviewStepID == "" {
		t.Fatalf("office-default template missing Work/Review step (got %d steps)", len(steps))
	}

	agentMgr := &mockAgentManager{repoForExecutionLookup: taskRepo, isAgentRunning: true}
	runQueue := &fakeRunQueueAdapter{calls: make(chan engine.QueueRunRequest, 8)}
	officeRepo := &fakeReviewSeatsOfficeRepo{workspaceID: workspaceID}
	mockTasks := newMockTaskRepo()

	svc := &Service{
		logger:       log,
		repo:         taskRepo,
		taskRepo:     mockTasks,
		agentManager: agentMgr,
		messageQueue: messagequeue.NewServiceMemory(log),
		executor:     executor.NewExecutor(agentMgr, taskRepo, log, executor.ExecutorConfig{}),
	}
	svc.SetWorkflowStepGetter(sg)
	svc.SetEngineRunQueue(runQueue)
	svc.SetEngineParticipantStore(workflowadapters.NewParticipantAdapter(workflowRepo))
	svc.SetEngineDecisionStore(workflowadapters.NewDecisionAdapter(workflowRepo))
	svc.SetEngineParticipantSeatWriter(workflowadapters.NewParticipantSeatWriterAdapter(workflowRepo))
	svc.SetEngineParticipantSeatCaster(officeengineadapters.NewSeatCasterAdapter(officeRepo, workflowRepo))

	taskRepo.SetStepEntryDispatcher(&testStepEntryDispatcher{eng: svc.WorkflowEngine()})

	return &reviewSeatsTestEnv{
		taskRepo:     taskRepo,
		workflowRepo: workflowRepo,
		svc:          svc,
		mockTaskRepo: mockTasks,
		runQueue:     runQueue,
		workspaceID:  workspaceID,
		workflowID:   workflowID,
		workStepID:   workStepID,
		reviewStepID: reviewStepID,
	}
}

// assertReviewerSeatQueued asserts AC-OFFICE-REVIEW-SEATS-005.1/.2/.3: a
// decision-required reviewer seat exists for taskID at Review, and a run
// was queued for that seat's agent.
func assertReviewerSeatQueued(t *testing.T, ctx context.Context, env *reviewSeatsTestEnv, taskID string) {
	t.Helper()

	participants, err := env.workflowRepo.ListStepParticipantsForTask(ctx, env.reviewStepID, taskID)
	if err != nil {
		t.Fatalf("list step participants: %v", err)
	}
	var reviewer *wfmodels.WorkflowStepParticipant
	for _, p := range participants {
		if string(p.Role) == "reviewer" {
			reviewer = p
			break
		}
	}
	if reviewer == nil {
		t.Fatalf("no reviewer seat found for task %s at Review step; participants=%+v", taskID, participants)
	}
	if !reviewer.DecisionRequired {
		t.Errorf("reviewer seat DecisionRequired = false, want true (AC-OFFICE-REVIEW-SEATS-005.1/.3)")
	}
	if reviewer.AgentProfileID == "" {
		t.Errorf("reviewer seat has no agent_profile_id")
	}

	select {
	case req := <-env.runQueue.calls:
		if req.TaskID != taskID {
			t.Errorf("queued run task id = %q, want %q", req.TaskID, taskID)
		}
		if req.AgentProfileID != reviewer.AgentProfileID {
			t.Errorf("queued run agent = %q, want reviewer seat's %q", req.AgentProfileID, reviewer.AgentProfileID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for reviewer run to be queued (AC-OFFICE-REVIEW-SEATS-005.2)")
	}
}

// TestReviewParticipantSeats_ProductRouteQueuesReviewerRun covers the
// product route: a live-session task's on_turn_complete transition (Work ->
// Review, via the engine) must land a decision-required reviewer seat and
// queue that reviewer's run.
func TestReviewParticipantSeats_ProductRouteQueuesReviewerRun(t *testing.T) {
	env := newReviewSeatsTestEnv(t)
	ctx := context.Background()

	taskID := "task-product-route"
	now := time.Now().UTC()
	task := &models.Task{
		ID:             taskID,
		WorkspaceID:    env.workspaceID,
		WorkflowID:     env.workflowID,
		WorkflowStepID: env.workStepID,
		Title:          "Product route review seat",
		State:          v1.TaskStateInProgress,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := env.taskRepo.CreateTask(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	// writeTaskReviewState (called along the on_turn_complete path once the
	// transition lands) writes tasks.state through the scheduler-facing
	// mockTaskRepo, a separate mock from the real repo above — seed it so
	// that write finds the task.
	seedMockTaskState(env.mockTaskRepo, taskID, v1.TaskStateInProgress)
	// ResolveCurrentRunner's tier-3 fallback (most-recently-assigned runner
	// row for the task at any step) needs a seeded runner row since Review
	// declares no step-level agent_profile_id and gets no Review-scoped
	// runner row here.
	if err := env.workflowRepo.SetTaskRunner(ctx, env.workStepID, taskID, "agent-primary"); err != nil {
		t.Fatalf("seed runner: %v", err)
	}

	sessionID := "session-product-route"
	session := &models.TaskSession{
		ID: sessionID, TaskID: taskID, State: models.TaskSessionStateRunning,
		StartedAt: now, UpdatedAt: now,
	}
	if err := env.taskRepo.CreateTaskSession(ctx, session); err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := env.taskRepo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: sessionID, SessionID: sessionID, TaskID: taskID, AgentExecutionID: "exec-1", Status: "ready",
	}); err != nil {
		t.Fatalf("seed executors_running: %v", err)
	}

	// Work declares auto_advance_requires_signal: true (ADR 0015) — seed the
	// pending-completion bag entry so the gate in allowEngineSignalCompletion
	// lets the turn-complete trigger through, mirroring a real
	// step_complete_kandev call.
	if err := env.taskRepo.SetSessionMetadataKey(ctx, sessionID, models.SessionMetaKeyPendingStepCompletion, models.PendingStepCompletionSignal{
		StepID:     env.workStepID,
		Source:     "agent",
		Summary:    "ready for review",
		SignaledAt: now,
	}); err != nil {
		t.Fatalf("seed pending step completion signal: %v", err)
	}

	loadedSession, err := env.taskRepo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("load session: %v", err)
	}

	transitioned := env.svc.processOnTurnCompleteViaEngine(ctx, taskID, loadedSession)
	if !transitioned {
		t.Fatalf("expected Work -> Review transition")
	}

	updatedTask, err := env.taskRepo.GetTask(ctx, taskID)
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if updatedTask.WorkflowStepID != env.reviewStepID {
		t.Fatalf("task step = %q, want Review (%q)", updatedTask.WorkflowStepID, env.reviewStepID)
	}

	assertReviewerSeatQueued(t, ctx, env, taskID)
}

// TestReviewParticipantSeats_ManualMoveNoSessionQueuesReviewerRun covers the
// manual route: a task with zero task_sessions rows, moved directly via
// Repository.UpdateTask (representing an operator move with no agent turn
// in flight), must still land a decision-required reviewer seat and queue
// that reviewer's run — the step-entry dispatcher is session-independent.
func TestReviewParticipantSeats_ManualMoveNoSessionQueuesReviewerRun(t *testing.T) {
	env := newReviewSeatsTestEnv(t)
	ctx := context.Background()

	taskID := "task-manual-route"
	now := time.Now().UTC()
	task := &models.Task{
		ID:             taskID,
		WorkspaceID:    env.workspaceID,
		WorkflowID:     env.workflowID,
		WorkflowStepID: env.workStepID,
		Title:          "Manual route review seat",
		State:          v1.TaskStateInProgress,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := env.taskRepo.CreateTask(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := env.workflowRepo.SetTaskRunner(ctx, env.workStepID, taskID, "agent-primary"); err != nil {
		t.Fatalf("seed runner: %v", err)
	}

	loaded, err := env.taskRepo.GetTask(ctx, taskID)
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	loaded.WorkflowStepID = env.reviewStepID
	loaded.UpdatedAt = time.Now().UTC()
	if err := env.taskRepo.UpdateTask(ctx, loaded); err != nil {
		t.Fatalf("manual move task to Review: %v", err)
	}

	assertReviewerSeatQueued(t, ctx, env, taskID)
}
