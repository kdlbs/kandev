package pluginsdk

import (
	"context"

	pluginv1 "github.com/kandev/kandev/proto/kandev/plugin/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TaskStepTransition is one retained task movement. Identity is a decimal
// string to preserve the ledger's int64 value in browser and SDK clients.
type TaskStepTransition struct {
	ID                 string
	FromWorkflowID     *string
	FromWorkflowStepID *string
	ToWorkflowID       *string
	ToWorkflowStepID   *string
	Trigger            string
	OccurredAt         string
}

// WorkflowTransitionGroup counts one route through or across a workflow.
type WorkflowTransitionGroup struct {
	Kind       string
	FromStepID *string
	ToStepID   *string
	Count      int64
}

// TransitionHistoryReader is an optional Host extension so existing Host
// implementations remain source compatible.
type TransitionHistoryReader interface {
	ListTask(ctx context.Context, taskID string, page Page) ([]TaskStepTransition, *PageInfo, error)
	ListWorkflowGroups(ctx context.Context, workflowID string, page Page) ([]WorkflowTransitionGroup, *PageInfo, error)
}

type TransitionHistoryHost interface {
	Transitions() TransitionHistoryReader
}

// TransitionHistory returns a Host's optional transition-history reader.
func TransitionHistory(host Host) (TransitionHistoryReader, bool) {
	provider, ok := host.(TransitionHistoryHost)
	if !ok {
		return nil, false
	}
	return provider.Transitions(), true
}

func (h *grpcHostClient) Transitions() TransitionHistoryReader {
	return grpcTransitionHistoryReader{client: h.client}
}

type grpcTransitionHistoryReader struct{ client pluginv1.HostClient }

func (r grpcTransitionHistoryReader) ListTask(ctx context.Context, taskID string, page Page) ([]TaskStepTransition, *PageInfo, error) {
	response, err := r.client.ListTaskStepTransitions(ctx, &pluginv1.ListTaskStepTransitionsRequest{TaskId: taskID, Page: page.toProto()})
	if err != nil {
		return nil, nil, err
	}
	items := make([]TaskStepTransition, len(response.GetItems()))
	for i, item := range response.GetItems() {
		items[i] = taskStepTransitionFromProto(item)
	}
	return items, pageInfoFromProto(response.GetPageInfo()), nil
}

func (r grpcTransitionHistoryReader) ListWorkflowGroups(ctx context.Context, workflowID string, page Page) ([]WorkflowTransitionGroup, *PageInfo, error) {
	response, err := r.client.ListWorkflowTransitionGroups(ctx, &pluginv1.ListWorkflowTransitionGroupsRequest{WorkflowId: workflowID, Page: page.toProto()})
	if err != nil {
		return nil, nil, err
	}
	items := make([]WorkflowTransitionGroup, len(response.GetItems()))
	for i, item := range response.GetItems() {
		items[i] = workflowTransitionGroupFromProto(item)
	}
	return items, pageInfoFromProto(response.GetPageInfo()), nil
}

func (s *grpcHostServer) ListTaskStepTransitions(ctx context.Context, request *pluginv1.ListTaskStepTransitionsRequest) (*pluginv1.ListTaskStepTransitionsResponse, error) {
	reader, ok := TransitionHistory(s.impl)
	if !ok {
		return nil, status.Error(codes.Unimplemented, "transition history unavailable")
	}
	items, page, err := reader.ListTask(ctx, request.GetTaskId(), pageFromProto(request.GetPage()))
	if err != nil {
		return nil, err
	}
	converted := make([]*pluginv1.TaskStepTransition, len(items))
	for i, item := range items {
		converted[i] = item.toProto()
	}
	return &pluginv1.ListTaskStepTransitionsResponse{Items: converted, PageInfo: page.toProto()}, nil
}

func (s *grpcHostServer) ListWorkflowTransitionGroups(ctx context.Context, request *pluginv1.ListWorkflowTransitionGroupsRequest) (*pluginv1.ListWorkflowTransitionGroupsResponse, error) {
	reader, ok := TransitionHistory(s.impl)
	if !ok {
		return nil, status.Error(codes.Unimplemented, "transition history unavailable")
	}
	items, page, err := reader.ListWorkflowGroups(ctx, request.GetWorkflowId(), pageFromProto(request.GetPage()))
	if err != nil {
		return nil, err
	}
	converted := make([]*pluginv1.WorkflowTransitionGroup, len(items))
	for i, item := range items {
		converted[i] = item.toProto()
	}
	return &pluginv1.ListWorkflowTransitionGroupsResponse{Items: converted, PageInfo: page.toProto()}, nil
}

func (item TaskStepTransition) toProto() *pluginv1.TaskStepTransition {
	return &pluginv1.TaskStepTransition{
		Id: item.ID, FromWorkflowId: item.FromWorkflowID,
		FromWorkflowStepId: item.FromWorkflowStepID, ToWorkflowId: item.ToWorkflowID,
		ToWorkflowStepId: item.ToWorkflowStepID, Trigger: item.Trigger,
		OccurredAt: item.OccurredAt,
	}
}

func taskStepTransitionFromProto(item *pluginv1.TaskStepTransition) TaskStepTransition {
	if item == nil {
		return TaskStepTransition{}
	}
	return TaskStepTransition{
		ID: item.GetId(), FromWorkflowID: item.FromWorkflowId,
		FromWorkflowStepID: item.FromWorkflowStepId, ToWorkflowID: item.ToWorkflowId,
		ToWorkflowStepID: item.ToWorkflowStepId, Trigger: item.GetTrigger(),
		OccurredAt: item.GetOccurredAt(),
	}
}

func (item WorkflowTransitionGroup) toProto() *pluginv1.WorkflowTransitionGroup {
	return &pluginv1.WorkflowTransitionGroup{Kind: item.Kind, FromStepId: item.FromStepID, ToStepId: item.ToStepID, Count: item.Count}
}

func workflowTransitionGroupFromProto(item *pluginv1.WorkflowTransitionGroup) WorkflowTransitionGroup {
	if item == nil {
		return WorkflowTransitionGroup{}
	}
	return WorkflowTransitionGroup{Kind: item.GetKind(), FromStepID: item.FromStepId, ToStepID: item.ToStepId, Count: item.GetCount()}
}
