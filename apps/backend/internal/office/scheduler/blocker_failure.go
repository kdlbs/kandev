package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/service"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

const (
	taskStateFailed    = "FAILED"
	taskStateCancelled = "CANCELLED"
)

func isFailedTaskState(state string) bool {
	return state == taskStateFailed || state == taskStateCancelled
}

func isTerminalTaskState(state string) bool {
	return taskmodels.IsTerminalTaskState(v1.TaskState(state))
}

// RegisterBlockerFailureSubscriber evaluates every task-service transition
// into FAILED or CANCELLED and every task a tree cancel moves to CANCELLED.
// Office status changes publish neither event; reactToStatusChange evaluates
// those.
func (ss *SchedulerService) RegisterBlockerFailureSubscriber(eb bus.EventBus) error {
	if _, err := eb.Subscribe(events.TaskStateChanged, ss.handleBlockerFailure); err != nil {
		return err
	}
	_, err := eb.Subscribe(events.OfficeTaskTreeHoldCreated, ss.handleTreeCancelBlockerFailures)
	return err
}

// handleTreeCancelBlockerFailures evaluates each task a cancel hold moved to
// CANCELLED. Members that were already FAILED or CANCELLED made no failure
// transition.
func (ss *SchedulerService) handleTreeCancelBlockerFailures(ctx context.Context, event *bus.Event) error {
	var hold models.TreeHold
	encoded, err := json.Marshal(event.Data)
	if err != nil {
		return fmt.Errorf("encode tree hold: %w", err)
	}
	if err := json.Unmarshal(encoded, &hold); err != nil {
		return fmt.Errorf("decode tree hold: %w", err)
	}
	if hold.Mode != models.TreeHoldModeCancel {
		return nil
	}
	members, err := ss.repo.ListHoldMembers(ctx, hold.ID)
	if err != nil {
		return fmt.Errorf("list cancelled tree members: %w", err)
	}
	var errs []error
	for _, member := range members {
		if member.SkipReason != "" || isFailedTaskState(member.TaskStatus) {
			continue
		}
		errs = append(errs, ss.escalateBlockerFailure(ctx, member.TaskID, taskStateCancelled, "tree:"+hold.ID))
	}
	return errors.Join(errs...)
}

func (ss *SchedulerService) handleBlockerFailure(ctx context.Context, event *bus.Event) error {
	var data struct {
		TaskID    string `json:"task_id"`
		OldState  string `json:"old_state"`
		NewState  string `json:"new_state"`
		UpdatedAt string `json:"updated_at"`
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
	if data.UpdatedAt == "" {
		return errors.New("task state event is missing its committed updated_at")
	}
	return ss.escalateBlockerFailure(ctx, data.TaskID, data.NewState, "task-state:"+data.UpdatedAt)
}

// escalateBlockerFailure wakes the agent responsible for a blocker that has
// just entered failedState, once per committed transition, when the blocker
// still holds at least one Office dependent. occurrenceID comes from the
// committed state transition or tree hold that caused this check. Do not
// derive it from the mutable task row or the current recipient.
func (ss *SchedulerService) escalateBlockerFailure(ctx context.Context, blockerID, failedState, occurrenceID string) error {
	if occurrenceID == "" {
		return errors.New("blocker failure occurrence identity is empty")
	}
	blocker, err := ss.repo.GetTaskExecutionFields(ctx, blockerID)
	if err != nil {
		return fmt.Errorf("read failed blocker: %w", err)
	}
	if blocker.State != failedState {
		return nil
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
	if err := ss.svc.QueueBlockerFailureWakeup(ctx, recipient, taskID, blocker.ID, blocker.State, occurrenceID, held); err != nil {
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
			dependent.WorkspaceID == blocker.WorkspaceID && !isTerminalTaskState(dependent.State) {
			held = append(held, id)
		}
	}
	return held, nil
}

// blockerFailureRecipient returns the runner of the blocker's live parent
// task, woken on that parent, or else the workspace CEO with a taskless wake.
// A completed, failed or cancelled parent no longer coordinates its children.
// An empty recipient means nobody is responsible.
func (ss *SchedulerService) blockerFailureRecipient(
	ctx context.Context, blocker *sqlite.TaskExecutionFields,
) (recipient, taskID string, err error) {
	parentID, err := ss.repo.GetTaskParentID(ctx, blocker.ID)
	if err != nil {
		return "", "", fmt.Errorf("read failed blocker parent: %w", err)
	}
	if parentID != "" {
		parent, err := ss.repo.GetTaskExecutionFields(ctx, parentID)
		if err != nil && !errors.Is(err, sqlite.ErrTaskNotFound) {
			return "", "", fmt.Errorf("read failed blocker parent: %w", err)
		}
		if parent != nil && parent.AssigneeAgentProfileID != "" && !isTerminalTaskState(parent.State) {
			return parent.AssigneeAgentProfileID, parentID, nil
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
