package plugins

import (
	"context"
	"errors"
	"strconv"
	"time"

	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (h *pluginHost) Transitions() pluginsdk.TransitionHistoryReader {
	return hostTransitionHistoryReader{host: h}
}

type hostTransitionHistoryReader struct{ host *pluginHost }

func (r hostTransitionHistoryReader) ListTask(ctx context.Context, taskID string, page pluginsdk.Page) ([]pluginsdk.TaskStepTransition, *pluginsdk.PageInfo, error) {
	if !r.host.capabilities.CanRead(resourceTasks) {
		return nil, nil, permissionDenied(apiReadCapability(resourceTasks))
	}
	task, err := r.host.fetchTaskForScopeCheck(ctx, taskID)
	if err != nil {
		return nil, nil, err
	}
	if task == nil {
		return nil, nil, status.Error(codes.NotFound, "task not found")
	}
	source, ok := r.host.taskData.(transitionDataSource)
	if !ok {
		return nil, nil, status.Error(codes.Unimplemented, "transition history unavailable")
	}
	items, next, err := source.ListTaskStepTransitions(ctx, taskID, normalizePageLimit(page.Limit), page.Cursor)
	if err != nil {
		return nil, nil, hostTransitionReadError(err)
	}
	result := make([]pluginsdk.TaskStepTransition, len(items))
	for i, item := range items {
		result[i] = pluginsdk.TaskStepTransition{
			ID: strconv.FormatInt(item.ID, 10), FromWorkflowID: item.FromWorkflowID,
			FromWorkflowStepID: item.FromWorkflowStepID, ToWorkflowID: item.ToWorkflowID,
			ToWorkflowStepID: item.ToWorkflowStepID, Trigger: item.Trigger,
			OccurredAt: item.OccurredAt.UTC().Format(time.RFC3339Nano),
		}
	}
	return result, &pluginsdk.PageInfo{NextCursor: next, HasMore: next != ""}, nil
}

func (r hostTransitionHistoryReader) ListWorkflowGroups(ctx context.Context, workflowID string, page pluginsdk.Page) ([]pluginsdk.WorkflowTransitionGroup, *pluginsdk.PageInfo, error) {
	if !r.host.capabilities.CanRead(resourceTasks) {
		return nil, nil, permissionDenied(apiReadCapability(resourceTasks))
	}
	if !r.host.capabilities.CanRead(resourceWorkflows) {
		return nil, nil, permissionDenied(apiReadCapability(resourceWorkflows))
	}
	source, ok := r.host.taskData.(transitionDataSource)
	if !ok {
		return nil, nil, status.Error(codes.Unimplemented, "transition history unavailable")
	}
	items, next, err := source.ListWorkflowTransitionGroups(ctx, workflowID, normalizePageLimit(page.Limit), page.Cursor)
	if err != nil {
		return nil, nil, hostTransitionReadError(err)
	}
	result := make([]pluginsdk.WorkflowTransitionGroup, len(items))
	for i, item := range items {
		result[i] = pluginsdk.WorkflowTransitionGroup{Kind: item.Kind, FromStepID: item.FromStepID, ToStepID: item.ToStepID, Count: item.Count}
	}
	return result, &pluginsdk.PageInfo{NextCursor: next, HasMore: next != ""}, nil
}

func hostTransitionReadError(err error) error {
	if errors.Is(err, taskservice.ErrInvalidTransitionCursor) {
		return status.Error(codes.InvalidArgument, "invalid transition cursor")
	}
	return err
}
