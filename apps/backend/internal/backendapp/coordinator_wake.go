package backendapp

import (
	"context"
	"errors"
	"time"

	"github.com/kandev/kandev/internal/coordinator"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/task/statussummary"
)

// wakeTaskAPI is the part of the task service the wake reader adapts.
type wakeTaskAPI interface {
	GetTask(ctx context.Context, id string) (*taskmodels.Task, error)
	GetPrimarySession(ctx context.Context, taskID string) (*taskmodels.TaskSession, error)
	GetTaskSession(ctx context.Context, sessionID string) (*taskmodels.TaskSession, error)
	ListPendingInteractions(ctx context.Context, filter taskmodels.PendingInteractionFilter) ([]*taskmodels.Interaction, error)
	GetTaskStatusSummaries(ctx context.Context, taskIDs []string) (map[string]*statussummary.TaskStatusSummary, error)
}

// coordinatorWakeReader adapts the task service to coordinator.WakeSources.
type coordinatorWakeReader struct {
	tasks wakeTaskAPI
}

var _ coordinator.WakeSources = (*coordinatorWakeReader)(nil)

func (a *coordinatorWakeReader) PrimarySessionID(ctx context.Context, taskID string) (string, error) {
	session, err := a.tasks.GetPrimarySession(ctx, taskID)
	if errors.Is(err, repoerrors.ErrNoPrimarySession) || errors.Is(err, taskmodels.ErrTaskSessionNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return session.ID, nil
}

func (a *coordinatorWakeReader) pending(ctx context.Context, sessionID string, kind taskmodels.InteractionKind) ([]string, error) {
	items, err := a.tasks.ListPendingInteractions(ctx, taskmodels.PendingInteractionFilter{
		SessionIDs: []string{sessionID}, Kinds: []string{string(kind)},
	})
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	return ids, nil
}

func (a *coordinatorWakeReader) PendingQuestionID(ctx context.Context, sessionID string) (string, error) {
	ids, err := a.pending(ctx, sessionID, taskmodels.InteractionKindClarification)
	if err != nil || len(ids) == 0 {
		return "", err
	}
	return ids[0], nil
}

func (a *coordinatorWakeReader) PendingPermissionIDs(ctx context.Context, sessionID string) ([]string, error) {
	return a.pending(ctx, sessionID, taskmodels.InteractionKindPermission)
}

func (a *coordinatorWakeReader) ActiveErrorStamp(ctx context.Context, sessionID string) (string, error) {
	session, err := a.tasks.GetTaskSession(ctx, sessionID)
	if errors.Is(err, taskmodels.ErrTaskSessionNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	lastErr, ok := taskmodels.LoadLastAgentError(session.Metadata)
	if !ok || lastErr.IsDismissed() {
		return "", nil
	}
	return lastErr.Stamp(), nil
}

func (a *coordinatorWakeReader) TaskState(ctx context.Context, taskID string) (string, error) {
	task, err := a.tasks.GetTask(ctx, taskID)
	if errors.Is(err, repoerrors.ErrTaskNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return string(task.State), nil
}

func (a *coordinatorWakeReader) LastActivityAt(ctx context.Context, taskID string) (*time.Time, error) {
	summaries, err := a.tasks.GetTaskStatusSummaries(ctx, []string{taskID})
	if err != nil {
		return nil, err
	}
	if summary := summaries[taskID]; summary != nil {
		return summary.LastActivityAt, nil
	}
	return nil, nil
}
