package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/service"
)

const (
	taskStateFailed    = "FAILED"
	taskStateCancelled = "CANCELLED"
	taskStateCompleted = "COMPLETED"
)

func isFailedTaskState(state string) bool {
	return state == taskStateFailed || state == taskStateCancelled
}

// RegisterBlockerFailureSubscriber evaluates every task-service transition
// into FAILED or CANCELLED. Office status changes do not publish this event;
// reactToStatusChange evaluates those.
func (ss *SchedulerService) RegisterBlockerFailureSubscriber(eb bus.EventBus) error {
	_, err := eb.Subscribe(events.TaskStateChanged, ss.handleBlockerFailure)
	return err
}

func (ss *SchedulerService) handleBlockerFailure(ctx context.Context, event *bus.Event) error {
	var data struct {
		TaskID   string `json:"task_id"`
		OldState string `json:"old_state"`
		NewState string `json:"new_state"`
	}
	encoded, err := json.Marshal(event.Data)
	if err != nil {
		return fmt.Errorf("encode blocker failure: %w", err)
	}
	if err := json.Unmarshal(encoded, &data); err != nil {
		return fmt.Errorf("decode blocker failure: %w", err)
	}
	if data.TaskID == "" || data.OldState == "" ||
		!isFailedTaskState(data.NewState) || isFailedTaskState(data.OldState) {
		return nil
	}
	return ss.escalateBlockerFailure(ctx, data.TaskID, data.NewState)
}

// escalateBlockerFailure wakes the agent responsible for a blocker that has
// just entered failedState, once per committed transition, when the blocker
// still holds at least one Office dependent. The occurrence is the blocker
// row's committed updated_at, so every path that observes the same
// transition derives the same idempotency key.
func (ss *SchedulerService) escalateBlockerFailure(ctx context.Context, blockerID, failedState string) error {
	blocker, err := ss.repo.GetTaskExecutionFields(ctx, blockerID)
	if err != nil {
		return fmt.Errorf("read failed blocker: %w", err)
	}
	if blocker.State != failedState {
		return nil
	}
	committedAt, err := ss.repo.GetTaskUpdatedAt(ctx, blockerID)
	if err != nil {
		return fmt.Errorf("read failed blocker occurrence: %w", err)
	}
	held, err := ss.heldDependents(ctx, blocker)
	if err != nil || len(held) == 0 {
		return err
	}
	recipient, taskID, err := ss.blockerFailureRecipient(ctx, blocker)
	if err != nil {
		return err
	}
	if recipient == "" {
		ss.logger.Warn("office.blocker_failed.unescalated",
			zap.String("blocker_task_id", blocker.ID), zap.Strings("held_dependent_ids", held))
		return nil
	}
	key := fmt.Sprintf("%s:%s:%s:%s", RunReasonTaskBlockerFailed, blocker.ID, recipient,
		committedAt.UTC().Format(time.RFC3339Nano))
	if _, err := ss.QueueRunCtx(ctx, recipient, RunContext{
		Reason:         RunReasonTaskBlockerFailed,
		TaskID:         taskID,
		WorkspaceID:    blocker.WorkspaceID,
		IdempotencyKey: key,
		ExtraPayload: map[string]any{
			"blocker_task_id":    blocker.ID,
			"blocker_state":      blocker.State,
			"held_dependent_ids": held,
		},
	}); err != nil {
		return fmt.Errorf("queue blocker failure escalation: %w", err)
	}
	ss.logger.Info("office.blocker_failed.queued",
		zap.String("blocker_task_id", blocker.ID), zap.String("recipient", recipient),
		zap.Strings("held_dependent_ids", held))
	return nil
}

// heldDependents returns the unarchived, non-terminal Office dependents in
// the blocker's workspace, in edge order.
func (ss *SchedulerService) heldDependents(
	ctx context.Context, blocker *sqlite.TaskExecutionFields,
) ([]string, error) {
	ids, err := ss.repo.ListTasksBlockedBy(ctx, blocker.ID)
	if err != nil {
		return nil, fmt.Errorf("list failed blocker dependents: %w", err)
	}
	held := make([]string, 0, len(ids))
	for _, id := range ids {
		dependent, err := ss.repo.GetTaskExecutionFields(ctx, id)
		if errors.Is(err, sqlite.ErrTaskNotFound) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read failed blocker dependent: %w", err)
		}
		if dependent.IsFromOffice && !dependent.Archived &&
			dependent.WorkspaceID == blocker.WorkspaceID && dependent.State != taskStateCompleted && !isFailedTaskState(dependent.State) {
			held = append(held, id)
		}
	}
	return held, nil
}

// blockerFailureRecipient returns the runner of the blocker's parent task,
// woken on that parent, or else the workspace CEO with a taskless wake. An
// empty recipient means nobody is responsible.
func (ss *SchedulerService) blockerFailureRecipient(
	ctx context.Context, blocker *sqlite.TaskExecutionFields,
) (recipient, taskID string, err error) {
	parentID, err := ss.repo.GetTaskParentID(ctx, blocker.ID)
	if err != nil {
		return "", "", fmt.Errorf("read failed blocker parent: %w", err)
	}
	if parentID != "" {
		runner, err := ss.repo.GetTaskAssignee(ctx, parentID)
		if err != nil {
			return "", "", fmt.Errorf("read failed blocker parent runner: %w", err)
		}
		if runner != "" {
			return runner, parentID, nil
		}
	}
	ceos, err := ss.svc.ListAgentInstancesFiltered(ctx, blocker.WorkspaceID,
		service.AgentListFilter{Role: string(models.AgentRoleCEO)})
	if err != nil {
		return "", "", fmt.Errorf("list workspace CEO: %w", err)
	}
	if len(ceos) == 0 {
		return "", "", nil
	}
	return ceos[0].ID, "", nil
}
