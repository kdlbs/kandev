package backendapp

import (
	"context"
	"time"

	taskservice "github.com/kandev/kandev/internal/task/service"
)

func (a pluginsTaskWriterAdapter) taskMutationIdentity(ctx context.Context, taskID string) (*taskservice.TaskMutationIdentity, error) {
	if reader, ok := a.svc.(interface {
		GetTaskMutationIdentity(context.Context, string) (*taskservice.TaskMutationIdentity, error)
	}); ok {
		return reader.GetTaskMutationIdentity(ctx, taskID)
	}
	// Preserve compatibility with existing adapter implementations and fixtures.
	task, err := a.svc.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	return &taskservice.TaskMutationIdentity{
		WorkspaceID: task.WorkspaceID, WorkflowID: task.WorkflowID,
		ResourceVersion: task.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}, nil
}
