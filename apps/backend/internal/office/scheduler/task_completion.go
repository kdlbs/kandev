package scheduler

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// RegisterTaskCompletionSubscriber connects committed prerequisite completion to
// Office dependent wakes, including tasks that have never had an agent session.
func (ss *SchedulerService) RegisterTaskCompletionSubscriber(eb bus.EventBus) error {
	_, err := eb.Subscribe(events.TaskStateChanged, ss.handleCommittedTaskCompletion)
	return err
}

func (ss *SchedulerService) handleCommittedTaskCompletion(ctx context.Context, event *bus.Event) error {
	var data struct {
		TaskID   string `json:"task_id"`
		OldState string `json:"old_state"`
		NewState string `json:"new_state"`
	}
	encoded, err := json.Marshal(event.Data)
	if err != nil {
		return fmt.Errorf("encode task completion: %w", err)
	}
	if err := json.Unmarshal(encoded, &data); err != nil {
		return fmt.Errorf("decode task completion: %w", err)
	}
	if data.TaskID == "" || data.OldState == "" || data.OldState == data.NewState ||
		(data.NewState != string(v1.TaskStateCompleted) &&
			data.NewState != string(v1.TaskStateFailed) &&
			data.NewState != string(v1.TaskStateCancelled)) {
		return nil
	}
	fields, err := ss.repo.GetTaskExecutionFields(ctx, data.TaskID)
	if err != nil {
		return fmt.Errorf("read terminal prerequisite: %w", err)
	}
	if fields.State != data.NewState {
		return nil
	}
	var queueErr error
	ss.cascadeBlockersResolved(ctx, &TaskSnapshot{ID: data.TaskID, WorkspaceID: fields.WorkspaceID}, func(agentID string, c RunContext) {
		dependent, err := ss.repo.GetTaskExecutionFields(ctx, c.TaskID)
		if err != nil {
			queueErr = fmt.Errorf("read completion dependent: %w", err)
			return
		}
		// A valid prerequisite may be Kanban; only Office targets are admitted.
		if !dependent.IsFromOffice || dependent.WorkspaceID != fields.WorkspaceID {
			return
		}
		if _, err := ss.QueueRunCtx(ctx, agentID, c); err != nil {
			queueErr = fmt.Errorf("queue completion dependent: %w", err)
		}
	})
	return queueErr
}
