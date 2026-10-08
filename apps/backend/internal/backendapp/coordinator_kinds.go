package backendapp

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/coordinator"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// kindTaskAPI is the part of the task service the kind reader adapts.
type kindTaskAPI interface {
	GetTask(ctx context.Context, id string) (*taskmodels.Task, error)
	GetPrimarySession(ctx context.Context, taskID string) (*taskmodels.TaskSession, error)
	GetExecutorRunningBySessionID(ctx context.Context, sessionID string) (*taskmodels.ExecutorRunning, error)
}

// coordinatorKindReader adapts the task service and the runtime to
// coordinator.KindTaskReader.
type coordinatorKindReader struct {
	tasks    kindTaskAPI
	liveExec func(sessionID string) bool
}

var _ coordinator.KindTaskReader = (*coordinatorKindReader)(nil)

func (a *coordinatorKindReader) GetTarget(ctx context.Context, taskID string) (*coordinator.TargetTask, error) {
	task, err := a.tasks.GetTask(ctx, taskID)
	if errors.Is(err, repoerrors.ErrTaskNotFound) {
		return nil, coordinator.ErrTaskNotFound
	}
	if err != nil {
		return nil, err
	}
	target := &coordinator.TargetTask{
		ID: task.ID, WorkspaceID: task.WorkspaceID, WorkflowID: task.WorkflowID,
		WorkflowStepID: task.WorkflowStepID, Origin: task.Origin, ArchivedAt: task.ArchivedAt,
	}
	session, err := a.tasks.GetPrimarySession(ctx, taskID)
	if errors.Is(err, repoerrors.ErrNoPrimarySession) || errors.Is(err, taskmodels.ErrTaskSessionNotFound) {
		return target, nil
	}
	if err != nil {
		return nil, err
	}
	record, err := a.tasks.GetExecutorRunningBySessionID(ctx, session.ID)
	hasRecord := err == nil && record != nil
	if err != nil && !isNoExecutorRecord(err) {
		return nil, err
	}
	target.Primary = &coordinator.TargetSession{
		ID: session.ID, State: string(session.State), HasExecutorRecord: hasRecord,
		RecoveryPending: taskmodels.HasInterruptedRecoveryPending(session.Metadata),
	}
	return target, nil
}

func isNoExecutorRecord(err error) bool {
	return errors.Is(err, taskmodels.ErrExecutorRunningNotFound)
}

func (a *coordinatorKindReader) HasLiveExecution(_ context.Context, sessionID string) bool {
	return a.liveExec(sessionID)
}

// coordinatorResumer adapts the orchestrator's session resume to
// coordinator.TaskResumer: a nil execution means the launch was deferred.
type coordinatorResumer struct {
	resume func(ctx context.Context, taskID, sessionID string) (*executor.TaskExecution, error)
}

var _ coordinator.TaskResumer = (*coordinatorResumer)(nil)

func (a *coordinatorResumer) ResumeTaskSession(ctx context.Context, taskID, sessionID string) (bool, error) {
	exec, err := a.resume(ctx, taskID, sessionID)
	if err != nil {
		return false, err
	}
	return exec != nil, nil
}
