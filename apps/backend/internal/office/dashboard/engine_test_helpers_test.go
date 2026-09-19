package dashboard_test

import (
	"context"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/common/logger"
	officeenginedispatcher "github.com/kandev/kandev/internal/office/engine_dispatcher"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	workflowadapters "github.com/kandev/kandev/internal/workflow/adapters"
	"github.com/kandev/kandev/internal/workflow/engine"
	workflowrepo "github.com/kandev/kandev/internal/workflow/repository"
)

// newTestEngineDispatcher wires a real workflow engine (AC-57: the slate
// construction/decision-write logic lives only in the engine, never
// reimplemented at the office tier, not even in tests) backed by the same
// workflow repository the dashboard tests already use for SetDecisionStore.
//
// fakeSessionResolver always reports "no active session" (AC-16a), since
// none of these tests insert a task_sessions row. That means
// RecordParticipantDecision still stamps and writes the decision through
// the real engine/store, but skips the AC-13-17 re-evaluation subpath — the
// same outcome a genuinely session-less task gets in production. The
// TransitionStore that subpath would need is therefore never invoked; the
// no-op below exists only to satisfy engine.New's signature.
func newTestEngineDispatcher(wfRepo *workflowrepo.Repository, log *logger.Logger) *officeenginedispatcher.Dispatcher {
	eng := engine.New(
		noopTransitionStore{},
		noopCallbackRegistry{},
		engine.WithParticipantStore(workflowadapters.NewParticipantAdapter(wfRepo)),
		engine.WithDecisionStore(workflowadapters.NewDecisionAdapter(wfRepo)),
	)
	return officeenginedispatcher.New(eng, fakeSessionResolver{}, log)
}

// fakeSessionResolver satisfies officeenginedispatcher.SessionResolver.
// Always reports no session, matching every dashboard test's task_sessions
// table, which is empty by default.
type fakeSessionResolver struct{}

func (fakeSessionResolver) GetActiveTaskSessionByTaskID(
	_ context.Context, _ string,
) (*taskmodels.TaskSession, error) {
	return nil, taskmodels.ErrTaskSessionNotFound
}

func (fakeSessionResolver) GetTaskSessionByTaskID(
	_ context.Context, _ string,
) (*taskmodels.TaskSession, error) {
	return nil, taskmodels.ErrTaskSessionNotFound
}

func (fakeSessionResolver) GetTaskSession(
	_ context.Context, _ string,
) (*taskmodels.TaskSession, error) {
	return nil, taskmodels.ErrTaskSessionNotFound
}

// noopCallbackRegistry satisfies engine.CallbackRegistry. Never consulted by
// RecordParticipantDecision/EvaluateStepQuorum (only HandleTrigger's action
// evaluation path uses it), so no test wires a real callback here.
type noopCallbackRegistry struct{}

func (noopCallbackRegistry) Get(_ engine.ActionKind) (engine.ActionCallback, bool) {
	return nil, false
}

// noopTransitionStore satisfies engine.TransitionStore. With
// fakeSessionResolver always reporting no active session,
// RecordParticipantDecision never reaches the re-evaluation subpath that
// would call this store, so every method here is unreachable in practice —
// it exists only to satisfy engine.New's required TransitionStore argument.
type noopTransitionStore struct{}

func (noopTransitionStore) LoadState(
	_ context.Context, _, _ string,
) (engine.MachineState, error) {
	return engine.MachineState{}, nil
}

func (noopTransitionStore) LoadStep(
	_ context.Context, _, _ string,
) (engine.StepSpec, error) {
	return engine.StepSpec{}, nil
}

func (noopTransitionStore) LoadNextStep(
	_ context.Context, _ string, _ int,
) (engine.StepSpec, error) {
	return engine.StepSpec{}, nil
}

func (noopTransitionStore) LoadPreviousStep(
	_ context.Context, _ string, _ int,
) (engine.StepSpec, error) {
	return engine.StepSpec{}, nil
}

func (noopTransitionStore) ApplyTransition(
	_ context.Context, _, _, _, _ string, _ engine.Trigger,
) error {
	return nil
}

func (noopTransitionStore) ApplyTransitionIfAtStep(
	_ context.Context, _, _, _, _ string, _ engine.Trigger,
) (bool, error) {
	return false, nil
}

func (noopTransitionStore) PersistData(
	_ context.Context, _ string, _ map[string]any,
) error {
	return nil
}

func (noopTransitionStore) IsOperationApplied(
	_ context.Context, _ string,
) (bool, error) {
	return false, nil
}

func (noopTransitionStore) MarkOperationApplied(_ context.Context, _ string) error {
	return nil
}

// dashboardTransitionStore is a real (non-noop) engine.TransitionStore
// backed by the same "tasks" table newTestDeps seeds. Unlike
// noopTransitionStore, ApplyTransitionIfAtStep here genuinely performs the
// AC-46/48 compare-and-swap against the tasks.workflow_step_id column, so a
// test using this store can observe a guarded transition actually moving a
// task off its current step through the same column the rest of the office
// tier (GetTaskWorkflowStepID, the quorum badge, task detail rendering)
// reads. Exists to unlock the AC-63/10 re-evaluation subpath that
// fakeSessionResolver/noopTransitionStore deliberately never reach.
type dashboardTransitionStore struct {
	db         *sqlx.DB
	workflowID string
	steps      map[string]engine.StepSpec
	applied    map[string]bool
}

func newDashboardTransitionStore(db *sqlx.DB) *dashboardTransitionStore {
	return &dashboardTransitionStore{
		db:         db,
		workflowID: "wf-test",
		steps:      map[string]engine.StepSpec{},
		applied:    map[string]bool{},
	}
}

func (s *dashboardTransitionStore) LoadState(
	ctx context.Context, taskID, sessionID string,
) (engine.MachineState, error) {
	var stepID string
	if err := s.db.QueryRowContext(ctx,
		`SELECT workflow_step_id FROM tasks WHERE id = ?`, taskID,
	).Scan(&stepID); err != nil {
		return engine.MachineState{}, err
	}
	return engine.MachineState{
		TaskID: taskID, SessionID: sessionID,
		WorkflowID: s.workflowID, CurrentStepID: stepID,
	}, nil
}

func (s *dashboardTransitionStore) LoadStep(
	_ context.Context, _, stepID string,
) (engine.StepSpec, error) {
	return s.steps[stepID], nil
}

func (s *dashboardTransitionStore) LoadNextStep(
	_ context.Context, _ string, _ int,
) (engine.StepSpec, error) {
	return engine.StepSpec{}, nil
}

func (s *dashboardTransitionStore) LoadPreviousStep(
	_ context.Context, _ string, _ int,
) (engine.StepSpec, error) {
	return engine.StepSpec{}, nil
}

func (s *dashboardTransitionStore) ApplyTransition(
	ctx context.Context, taskID, _, _, toStepID string, _ engine.Trigger,
) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE tasks SET workflow_step_id = ? WHERE id = ?`, toStepID, taskID)
	return err
}

// ApplyTransitionIfAtStep is the genuine CAS this store exists to provide:
// the UPDATE only lands when the task is still at expectedStepID, mirroring
// the production repository's guard against a concurrent mover.
func (s *dashboardTransitionStore) ApplyTransitionIfAtStep(
	ctx context.Context, taskID, _, expectedStepID, toStepID string, _ engine.Trigger,
) (bool, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE tasks SET workflow_step_id = ? WHERE id = ? AND workflow_step_id = ?`,
		toStepID, taskID, expectedStepID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (s *dashboardTransitionStore) PersistData(
	_ context.Context, _ string, _ map[string]any,
) error {
	return nil
}

func (s *dashboardTransitionStore) IsOperationApplied(
	_ context.Context, op string,
) (bool, error) {
	return s.applied[op], nil
}

func (s *dashboardTransitionStore) MarkOperationApplied(_ context.Context, op string) error {
	s.applied[op] = true
	return nil
}

// singleTaskSessionResolver reports an active session for exactly one
// configured task id and "no session" for every other task, letting a test
// drive RecordParticipantDecision's re-evaluation subpath for that one task
// without disturbing every other dashboard test's session-less assumption.
type singleTaskSessionResolver struct {
	taskID  string
	session *taskmodels.TaskSession
}

func (r singleTaskSessionResolver) GetActiveTaskSessionByTaskID(
	_ context.Context, taskID string,
) (*taskmodels.TaskSession, error) {
	if taskID == r.taskID {
		return r.session, nil
	}
	return nil, taskmodels.ErrTaskSessionNotFound
}

func (r singleTaskSessionResolver) GetTaskSessionByTaskID(
	ctx context.Context, taskID string,
) (*taskmodels.TaskSession, error) {
	return r.GetActiveTaskSessionByTaskID(ctx, taskID)
}

func (r singleTaskSessionResolver) GetTaskSession(
	_ context.Context, id string,
) (*taskmodels.TaskSession, error) {
	if r.session != nil && id == r.session.ID {
		return r.session, nil
	}
	return nil, taskmodels.ErrTaskSessionNotFound
}

// newTestEngineDispatcherWithReevaluation is like newTestEngineDispatcher but
// wires a real TransitionStore and a session resolver that reports an
// active session for taskID, unlocking the AC-13-17/AC-65 re-evaluation
// subpath — the AC-63/10 "human veto actually unsticks a reviewer-guarded
// step" path newTestEngineDispatcher's noop/no-session combination cannot
// reach.
func newTestEngineDispatcherWithReevaluation(
	wfRepo *workflowrepo.Repository, log *logger.Logger,
	store *dashboardTransitionStore, taskID string, session *taskmodels.TaskSession,
) *officeenginedispatcher.Dispatcher {
	eng := engine.New(
		store,
		noopCallbackRegistry{},
		engine.WithParticipantStore(workflowadapters.NewParticipantAdapter(wfRepo)),
		engine.WithDecisionStore(workflowadapters.NewDecisionAdapter(wfRepo)),
	)
	return officeenginedispatcher.New(eng, singleTaskSessionResolver{taskID: taskID, session: session}, log)
}
