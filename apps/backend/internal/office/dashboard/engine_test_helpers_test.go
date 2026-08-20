package dashboard_test

import (
	"context"

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
