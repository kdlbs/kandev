package pluginsdk

import (
	"context"

	pluginv1 "github.com/kandev/kandev/proto/kandev/plugin/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type grpcExactQueryManager struct{ client pluginv1.HostClient }

func (h *grpcHostClient) ExactQueries() ExactQueryManager {
	return grpcExactQueryManager{client: h.client}
}

func (s *grpcHostServer) exactQueryManager() (ExactQueryManager, error) {
	if s.exactQueries == nil || s.exactQueries.ExactQueries() == nil {
		return nil, status.Error(codes.Unimplemented, "exact Host queries are unavailable")
	}
	return s.exactQueries.ExactQueries(), nil
}

func (s *grpcHostServer) ListWorkspacesExact(ctx context.Context, req *pluginv1.ListWorkspacesExactRequest) (*pluginv1.ListWorkspacesExactResponse, error) {
	manager, err := s.exactQueryManager()
	if err != nil {
		return nil, err
	}
	page, err := manager.ListWorkspaces(ctx, ExactWorkspaceQuery{RequestID: req.GetRequestId(), WorkspaceID: req.GetWorkspaceId(), Page: exactReadPageFromProto(req.GetPage())})
	if err != nil {
		return nil, err
	}
	items := make([]*pluginv1.ExactWorkspaceObservation, len(page.Items))
	for i, item := range page.Items {
		items[i] = &pluginv1.ExactWorkspaceObservation{Workspace: item.Workspace.toProto(), ResourceVersion: item.ResourceVersion, ObservedAt: item.ObservedAt}
	}
	return &pluginv1.ListWorkspacesExactResponse{Items: items, PageInfo: exactReadPageInfoToProto(page.PageInfo)}, nil
}

func (h grpcExactQueryManager) ListWorkspaces(ctx context.Context, q ExactWorkspaceQuery) (ExactWorkspacePage, error) {
	r, err := h.client.ListWorkspacesExact(ctx, &pluginv1.ListWorkspacesExactRequest{RequestId: q.RequestID, WorkspaceId: q.WorkspaceID, Page: exactReadPageToProto(q.Page)})
	if err != nil {
		return ExactWorkspacePage{}, err
	}
	items := make([]ExactWorkspaceObservation, len(r.GetItems()))
	for i, item := range r.GetItems() {
		items[i] = ExactWorkspaceObservation{Workspace: workspaceFromProto(item.GetWorkspace()), ResourceVersion: item.GetResourceVersion(), ObservedAt: item.GetObservedAt()}
	}
	return ExactWorkspacePage{Items: items, PageInfo: exactReadPageInfoFromProto(r.GetPageInfo())}, nil
}

func (s *grpcHostServer) ListWorkflowsExact(ctx context.Context, req *pluginv1.ListWorkflowsExactRequest) (*pluginv1.ListWorkflowsExactResponse, error) {
	manager, err := s.exactQueryManager()
	if err != nil {
		return nil, err
	}
	page, err := manager.ListWorkflows(ctx, ExactWorkflowQuery{RequestID: req.GetRequestId(), WorkspaceID: req.GetWorkspaceId(), Page: exactReadPageFromProto(req.GetPage())})
	if err != nil {
		return nil, err
	}
	items := make([]*pluginv1.ExactWorkflowObservation, len(page.Items))
	for i, item := range page.Items {
		items[i] = &pluginv1.ExactWorkflowObservation{Workflow: item.Workflow.toProto(), ResourceVersion: item.ResourceVersion, ObservedAt: item.ObservedAt}
	}
	return &pluginv1.ListWorkflowsExactResponse{Items: items, PageInfo: exactReadPageInfoToProto(page.PageInfo)}, nil
}
func (h grpcExactQueryManager) ListWorkflows(ctx context.Context, q ExactWorkflowQuery) (ExactWorkflowPage, error) {
	r, err := h.client.ListWorkflowsExact(ctx, &pluginv1.ListWorkflowsExactRequest{RequestId: q.RequestID, WorkspaceId: q.WorkspaceID, Page: exactReadPageToProto(q.Page)})
	if err != nil {
		return ExactWorkflowPage{}, err
	}
	items := make([]ExactWorkflowObservation, len(r.GetItems()))
	for i, item := range r.GetItems() {
		items[i] = ExactWorkflowObservation{Workflow: workflowFromProto(item.GetWorkflow()), ResourceVersion: item.GetResourceVersion(), ObservedAt: item.GetObservedAt()}
	}
	return ExactWorkflowPage{Items: items, PageInfo: exactReadPageInfoFromProto(r.GetPageInfo())}, nil
}

func (s *grpcHostServer) ListWorkflowStepsExact(ctx context.Context, req *pluginv1.ListWorkflowStepsExactRequest) (*pluginv1.ListWorkflowStepsExactResponse, error) {
	manager, err := s.exactQueryManager()
	if err != nil {
		return nil, err
	}
	page, err := manager.ListWorkflowSteps(ctx, ExactWorkflowStepQuery{RequestID: req.GetRequestId(), WorkspaceID: req.GetWorkspaceId(), WorkflowID: req.GetWorkflowId(), Page: exactReadPageFromProto(req.GetPage())})
	if err != nil {
		return nil, err
	}
	items := make([]*pluginv1.ExactWorkflowStepObservation, len(page.Items))
	for i, item := range page.Items {
		items[i] = &pluginv1.ExactWorkflowStepObservation{Step: item.Step.toProto(), ResourceVersion: item.ResourceVersion, ObservedAt: item.ObservedAt}
	}
	return &pluginv1.ListWorkflowStepsExactResponse{Items: items, PageInfo: exactReadPageInfoToProto(page.PageInfo)}, nil
}
func (h grpcExactQueryManager) ListWorkflowSteps(ctx context.Context, q ExactWorkflowStepQuery) (ExactWorkflowStepPage, error) {
	r, err := h.client.ListWorkflowStepsExact(ctx, &pluginv1.ListWorkflowStepsExactRequest{RequestId: q.RequestID, WorkspaceId: q.WorkspaceID, WorkflowId: q.WorkflowID, Page: exactReadPageToProto(q.Page)})
	if err != nil {
		return ExactWorkflowStepPage{}, err
	}
	items := make([]ExactWorkflowStepObservation, len(r.GetItems()))
	for i, item := range r.GetItems() {
		items[i] = ExactWorkflowStepObservation{Step: workflowStepFromProto(item.GetStep()), ResourceVersion: item.GetResourceVersion(), ObservedAt: item.GetObservedAt()}
	}
	return ExactWorkflowStepPage{Items: items, PageInfo: exactReadPageInfoFromProto(r.GetPageInfo())}, nil
}

func (s *grpcHostServer) ListTasksExact(ctx context.Context, req *pluginv1.ListTasksExactRequest) (*pluginv1.ListTasksExactResponse, error) {
	manager, err := s.exactQueryManager()
	if err != nil {
		return nil, err
	}
	page, err := manager.ListTasks(ctx, ExactTaskQuery{RequestID: req.GetRequestId(), WorkspaceID: req.GetWorkspaceId(), Filter: taskFilterFromProto(req.GetFilter()), Page: exactReadPageFromProto(req.GetPage())})
	if err != nil {
		return nil, err
	}
	items := make([]*pluginv1.ExactTaskObservation, len(page.Items))
	for i, item := range page.Items {
		items[i], err = exactTaskObservationToProto(item)
		if err != nil {
			return nil, err
		}
	}
	return &pluginv1.ListTasksExactResponse{Items: items, PageInfo: exactReadPageInfoToProto(page.PageInfo)}, nil
}
func (h grpcExactQueryManager) ListTasks(ctx context.Context, q ExactTaskQuery) (ExactTaskPage, error) {
	r, err := h.client.ListTasksExact(ctx, &pluginv1.ListTasksExactRequest{RequestId: q.RequestID, WorkspaceId: q.WorkspaceID, Filter: q.Filter.toProto(), Page: exactReadPageToProto(q.Page)})
	if err != nil {
		return ExactTaskPage{}, err
	}
	items := make([]ExactTaskObservation, len(r.GetItems()))
	for i, item := range r.GetItems() {
		items[i], err = exactTaskObservationFromProto(item)
		if err != nil {
			return ExactTaskPage{}, err
		}
	}
	return ExactTaskPage{Items: items, PageInfo: exactReadPageInfoFromProto(r.GetPageInfo())}, nil
}

func (s *grpcHostServer) GetTaskExact(ctx context.Context, req *pluginv1.GetTaskExactRequest) (*pluginv1.GetTaskExactResponse, error) {
	manager, err := s.exactQueryManager()
	if err != nil {
		return nil, err
	}
	item, receipt, err := manager.GetTask(ctx, ExactTaskGetQuery{RequestID: req.GetRequestId(), WorkspaceID: req.GetWorkspaceId(), TaskID: req.GetTaskId()})
	if err != nil {
		return nil, err
	}
	wire, err := exactTaskObservationToProto(item)
	if err != nil {
		return nil, err
	}
	return &pluginv1.GetTaskExactResponse{Item: wire, Receipt: hostReadReceiptToProto(receipt)}, nil
}
func (h grpcExactQueryManager) GetTask(ctx context.Context, q ExactTaskGetQuery) (ExactTaskObservation, HostReadReceipt, error) {
	r, err := h.client.GetTaskExact(ctx, &pluginv1.GetTaskExactRequest{RequestId: q.RequestID, WorkspaceId: q.WorkspaceID, TaskId: q.TaskID})
	if err != nil {
		return ExactTaskObservation{}, HostReadReceipt{}, err
	}
	item, err := exactTaskObservationFromProto(r.GetItem())
	return item, hostReadReceiptFromProto(r.GetReceipt()), err
}

func (s *grpcHostServer) ListSessionsExact(ctx context.Context, req *pluginv1.ListSessionsExactRequest) (*pluginv1.ListSessionsExactResponse, error) {
	manager, err := s.exactQueryManager()
	if err != nil {
		return nil, err
	}
	page, err := manager.ListSessions(ctx, ExactSessionQuery{RequestID: req.GetRequestId(), WorkspaceID: req.GetWorkspaceId(), Filter: sessionFilterFromProto(req.GetFilter()), Page: exactReadPageFromProto(req.GetPage())})
	if err != nil {
		return nil, err
	}
	items := make([]*pluginv1.ExactSessionObservation, len(page.Items))
	for i, item := range page.Items {
		items[i] = &pluginv1.ExactSessionObservation{
			Session: item.Session.toProto(), ExecutionState: item.ExecutionState,
			ResourceVersion: item.ResourceVersion, ObservedAt: item.ObservedAt, ExecutionId: item.ExecutionID,
		}
	}
	return &pluginv1.ListSessionsExactResponse{Items: items, PageInfo: exactReadPageInfoToProto(page.PageInfo)}, nil
}
func (h grpcExactQueryManager) ListSessions(ctx context.Context, q ExactSessionQuery) (ExactSessionPage, error) {
	r, err := h.client.ListSessionsExact(ctx, &pluginv1.ListSessionsExactRequest{RequestId: q.RequestID, WorkspaceId: q.WorkspaceID, Filter: q.Filter.toProto(), Page: exactReadPageToProto(q.Page)})
	if err != nil {
		return ExactSessionPage{}, err
	}
	items := make([]ExactSessionObservation, len(r.GetItems()))
	for i, item := range r.GetItems() {
		items[i] = ExactSessionObservation{
			Session: sessionFromProto(item.GetSession()), ExecutionState: item.GetExecutionState(),
			ResourceVersion: item.GetResourceVersion(), ObservedAt: item.GetObservedAt(), ExecutionID: item.GetExecutionId(),
		}
	}
	return ExactSessionPage{Items: items, PageInfo: exactReadPageInfoFromProto(r.GetPageInfo())}, nil
}

func (s *grpcHostServer) ListPendingInteractionsExact(ctx context.Context, req *pluginv1.ListPendingInteractionsExactRequest) (*pluginv1.ListPendingInteractionsExactResponse, error) {
	manager, err := s.exactQueryManager()
	if err != nil {
		return nil, err
	}
	page, err := manager.ListPendingInteractions(ctx, ExactInteractionQuery{RequestID: req.GetRequestId(), WorkspaceID: req.GetWorkspaceId(), Filter: interactionFilterFromProto(req.GetFilter()), Page: exactReadPageFromProto(req.GetPage())})
	if err != nil {
		return nil, err
	}
	items := make([]*pluginv1.ExactInteractionObservation, len(page.Items))
	for i, item := range page.Items {
		items[i] = &pluginv1.ExactInteractionObservation{Interaction: item.Interaction.toProto(), ResourceVersion: item.ResourceVersion, ObservedAt: item.ObservedAt}
	}
	return &pluginv1.ListPendingInteractionsExactResponse{Items: items, PageInfo: exactReadPageInfoToProto(page.PageInfo)}, nil
}
func (h grpcExactQueryManager) ListPendingInteractions(ctx context.Context, q ExactInteractionQuery) (ExactInteractionPage, error) {
	r, err := h.client.ListPendingInteractionsExact(ctx, &pluginv1.ListPendingInteractionsExactRequest{RequestId: q.RequestID, WorkspaceId: q.WorkspaceID, Filter: q.Filter.toProto(), Page: exactReadPageToProto(q.Page)})
	if err != nil {
		return ExactInteractionPage{}, err
	}
	items := make([]ExactInteractionObservation, len(r.GetItems()))
	for i, item := range r.GetItems() {
		items[i] = ExactInteractionObservation{Interaction: interactionFromProto(item.GetInteraction()), ResourceVersion: item.GetResourceVersion(), ObservedAt: item.GetObservedAt()}
	}
	return ExactInteractionPage{Items: items, PageInfo: exactReadPageInfoFromProto(r.GetPageInfo())}, nil
}

func (s *grpcHostServer) GetInteractionExact(ctx context.Context, req *pluginv1.GetInteractionExactRequest) (*pluginv1.GetInteractionExactResponse, error) {
	manager, err := s.exactQueryManager()
	if err != nil {
		return nil, err
	}
	item, receipt, err := manager.GetInteraction(ctx, ExactInteractionGetQuery{RequestID: req.GetRequestId(), WorkspaceID: req.GetWorkspaceId(), InteractionID: req.GetInteractionId()})
	if err != nil {
		return nil, err
	}
	return &pluginv1.GetInteractionExactResponse{Item: &pluginv1.ExactInteractionObservation{Interaction: item.Interaction.toProto(), ResourceVersion: item.ResourceVersion, ObservedAt: item.ObservedAt}, Receipt: hostReadReceiptToProto(receipt)}, nil
}
func (h grpcExactQueryManager) GetInteraction(ctx context.Context, q ExactInteractionGetQuery) (ExactInteractionObservation, HostReadReceipt, error) {
	r, err := h.client.GetInteractionExact(ctx, &pluginv1.GetInteractionExactRequest{RequestId: q.RequestID, WorkspaceId: q.WorkspaceID, InteractionId: q.InteractionID})
	if err != nil {
		return ExactInteractionObservation{}, HostReadReceipt{}, err
	}
	p := r.GetItem()
	return ExactInteractionObservation{Interaction: interactionFromProto(p.GetInteraction()), ResourceVersion: p.GetResourceVersion(), ObservedAt: p.GetObservedAt()}, hostReadReceiptFromProto(r.GetReceipt()), nil
}

func (s *grpcHostServer) ListSanitizedMessagesExact(ctx context.Context, req *pluginv1.ListSanitizedMessagesExactRequest) (*pluginv1.ListSanitizedMessagesExactResponse, error) {
	manager, err := s.exactQueryManager()
	if err != nil {
		return nil, err
	}
	page, err := manager.ListSanitizedMessages(ctx, ExactMessageQuery{RequestID: req.GetRequestId(), WorkspaceID: req.GetWorkspaceId(), Filter: messageFilterFromProto(req.GetFilter()), Page: exactReadPageFromProto(req.GetPage())})
	if err != nil {
		return nil, err
	}
	items := make([]*pluginv1.ExactMessageObservation, len(page.Items))
	for i, item := range page.Items {
		items[i] = &pluginv1.ExactMessageObservation{Message: item.Message.toProto(), ResourceVersion: item.ResourceVersion, ObservedAt: item.ObservedAt, Sanitization: item.Sanitization}
	}
	return &pluginv1.ListSanitizedMessagesExactResponse{Items: items, PageInfo: exactReadPageInfoToProto(page.PageInfo)}, nil
}
func (h grpcExactQueryManager) ListSanitizedMessages(ctx context.Context, q ExactMessageQuery) (ExactMessagePage, error) {
	r, err := h.client.ListSanitizedMessagesExact(ctx, &pluginv1.ListSanitizedMessagesExactRequest{RequestId: q.RequestID, WorkspaceId: q.WorkspaceID, Filter: q.Filter.toProto(), Page: exactReadPageToProto(q.Page)})
	if err != nil {
		return ExactMessagePage{}, err
	}
	items := make([]ExactMessageObservation, len(r.GetItems()))
	for i, item := range r.GetItems() {
		items[i] = ExactMessageObservation{Message: messageFromProto(item.GetMessage()), ResourceVersion: item.GetResourceVersion(), ObservedAt: item.GetObservedAt(), Sanitization: item.GetSanitization()}
	}
	return ExactMessagePage{Items: items, PageInfo: exactReadPageInfoFromProto(r.GetPageInfo())}, nil
}

func (s *grpcHostServer) ListTaskRelationsExact(ctx context.Context, req *pluginv1.ListTaskRelationsExactRequest) (*pluginv1.ListTaskRelationsExactResponse, error) {
	manager, err := s.exactQueryManager()
	if err != nil {
		return nil, err
	}
	page, err := manager.ListTaskRelations(ctx, ExactTaskRelationsQuery{RequestID: req.GetRequestId(), WorkspaceID: req.GetWorkspaceId(), TaskID: req.GetTaskId(), Page: exactReadPageFromProto(req.GetPage())})
	if err != nil {
		return nil, err
	}
	return &pluginv1.ListTaskRelationsExactResponse{Items: taskRelationsToProto(page.Items), PageInfo: exactReadPageInfoToProto(page.PageInfo)}, nil
}
func (s *grpcHostServer) GetTaskRelationsExact(ctx context.Context, req *pluginv1.GetTaskRelationsExactRequest) (*pluginv1.GetTaskRelationsExactResponse, error) {
	manager, err := s.exactQueryManager()
	if err != nil {
		return nil, err
	}
	page, err := manager.GetTaskRelations(ctx, ExactTaskRelationsQuery{RequestID: req.GetRequestId(), WorkspaceID: req.GetWorkspaceId(), TaskID: req.GetTaskId(), Page: exactReadPageFromProto(req.GetPage())})
	if err != nil {
		return nil, err
	}
	return &pluginv1.GetTaskRelationsExactResponse{Items: taskRelationsToProto(page.Items), PageInfo: exactReadPageInfoToProto(page.PageInfo)}, nil
}
func (h grpcExactQueryManager) ListTaskRelations(ctx context.Context, q ExactTaskRelationsQuery) (ExactTaskRelationsPage, error) {
	r, err := h.client.ListTaskRelationsExact(ctx, &pluginv1.ListTaskRelationsExactRequest{RequestId: q.RequestID, WorkspaceId: q.WorkspaceID, TaskId: q.TaskID, Page: exactReadPageToProto(q.Page)})
	if err != nil {
		return ExactTaskRelationsPage{}, err
	}
	return ExactTaskRelationsPage{Items: taskRelationsFromProto(r.GetItems()), PageInfo: exactReadPageInfoFromProto(r.GetPageInfo())}, nil
}
func (h grpcExactQueryManager) GetTaskRelations(ctx context.Context, q ExactTaskRelationsQuery) (ExactTaskRelationsPage, error) {
	r, err := h.client.GetTaskRelationsExact(ctx, &pluginv1.GetTaskRelationsExactRequest{RequestId: q.RequestID, WorkspaceId: q.WorkspaceID, TaskId: q.TaskID, Page: exactReadPageToProto(q.Page)})
	if err != nil {
		return ExactTaskRelationsPage{}, err
	}
	return ExactTaskRelationsPage{Items: taskRelationsFromProto(r.GetItems()), PageInfo: exactReadPageInfoFromProto(r.GetPageInfo())}, nil
}

func (s *grpcHostServer) ListTaskInboxExact(ctx context.Context, req *pluginv1.ListTaskInboxExactRequest) (*pluginv1.ListTaskInboxExactResponse, error) {
	manager, err := s.exactQueryManager()
	if err != nil {
		return nil, err
	}
	page, err := manager.ListTaskInbox(ctx, ExactTaskInboxQuery{RequestID: req.GetRequestId(), WorkspaceID: req.GetWorkspaceId(), TaskID: req.GetTaskId(), SessionID: req.GetSessionId(), Page: exactReadPageFromProto(req.GetPage())})
	if err != nil {
		return nil, err
	}
	items := make([]*pluginv1.TaskInboxItem, len(page.Items))
	for i, item := range page.Items {
		items[i] = taskInboxItemToProto(item)
	}
	return &pluginv1.ListTaskInboxExactResponse{Items: items, PageInfo: exactReadPageInfoToProto(page.PageInfo)}, nil
}
func (h grpcExactQueryManager) ListTaskInbox(ctx context.Context, q ExactTaskInboxQuery) (ExactTaskInboxPage, error) {
	r, err := h.client.ListTaskInboxExact(ctx, &pluginv1.ListTaskInboxExactRequest{RequestId: q.RequestID, WorkspaceId: q.WorkspaceID, TaskId: q.TaskID, SessionId: q.SessionID, Page: exactReadPageToProto(q.Page)})
	if err != nil {
		return ExactTaskInboxPage{}, err
	}
	items := make([]TaskInboxItem, len(r.GetItems()))
	for i, item := range r.GetItems() {
		items[i] = taskInboxItemFromProto(item)
	}
	return ExactTaskInboxPage{Items: items, PageInfo: exactReadPageInfoFromProto(r.GetPageInfo())}, nil
}

func (s *grpcHostServer) GetTaskDirectiveExact(ctx context.Context, req *pluginv1.GetTaskDirectiveExactRequest) (*pluginv1.GetTaskDirectiveExactResponse, error) {
	manager, err := s.exactQueryManager()
	if err != nil {
		return nil, err
	}
	item, receipt, err := manager.GetTaskDirective(ctx, ExactTaskDirectiveGetQuery{RequestID: req.GetRequestId(), WorkspaceID: req.GetWorkspaceId(), DirectiveID: req.GetDirectiveId()})
	if err != nil {
		return nil, err
	}
	return &pluginv1.GetTaskDirectiveExactResponse{Item: taskDirectiveToProto(item), Receipt: hostReadReceiptToProto(receipt)}, nil
}
func (h grpcExactQueryManager) GetTaskDirective(ctx context.Context, q ExactTaskDirectiveGetQuery) (TaskDirective, HostReadReceipt, error) {
	r, err := h.client.GetTaskDirectiveExact(ctx, &pluginv1.GetTaskDirectiveExactRequest{RequestId: q.RequestID, WorkspaceId: q.WorkspaceID, DirectiveId: q.DirectiveID})
	if err != nil {
		return TaskDirective{}, HostReadReceipt{}, err
	}
	return taskDirectiveFromProto(r.GetItem()), hostReadReceiptFromProto(r.GetReceipt()), nil
}
func (s *grpcHostServer) ListTaskDirectivesExact(ctx context.Context, req *pluginv1.ListTaskDirectivesExactRequest) (*pluginv1.ListTaskDirectivesExactResponse, error) {
	manager, err := s.exactQueryManager()
	if err != nil {
		return nil, err
	}
	page, err := manager.ListTaskDirectives(ctx, ExactTaskDirectiveQuery{RequestID: req.GetRequestId(), WorkspaceID: req.GetWorkspaceId(), TaskID: req.GetTaskId(), Page: exactReadPageFromProto(req.GetPage())})
	if err != nil {
		return nil, err
	}
	items := make([]*pluginv1.TaskDirective, len(page.Items))
	for i, item := range page.Items {
		items[i] = taskDirectiveToProto(item)
	}
	return &pluginv1.ListTaskDirectivesExactResponse{Items: items, PageInfo: exactReadPageInfoToProto(page.PageInfo)}, nil
}
func (h grpcExactQueryManager) ListTaskDirectives(ctx context.Context, q ExactTaskDirectiveQuery) (ExactTaskDirectivesPage, error) {
	r, err := h.client.ListTaskDirectivesExact(ctx, &pluginv1.ListTaskDirectivesExactRequest{RequestId: q.RequestID, WorkspaceId: q.WorkspaceID, TaskId: q.TaskID, Page: exactReadPageToProto(q.Page)})
	if err != nil {
		return ExactTaskDirectivesPage{}, err
	}
	items := make([]TaskDirective, len(r.GetItems()))
	for i, item := range r.GetItems() {
		items[i] = taskDirectiveFromProto(item)
	}
	return ExactTaskDirectivesPage{Items: items, PageInfo: exactReadPageInfoFromProto(r.GetPageInfo())}, nil
}

func (s *grpcHostServer) ListPendingTaskTransitionsExact(ctx context.Context, req *pluginv1.ListPendingTaskTransitionsExactRequest) (*pluginv1.ListPendingTaskTransitionsExactResponse, error) {
	manager, err := s.exactQueryManager()
	if err != nil {
		return nil, err
	}
	page, err := manager.ListPendingTaskTransitions(ctx, ExactPendingTaskTransitionsQuery{RequestID: req.GetRequestId(), WorkspaceID: req.GetWorkspaceId(), TaskID: req.GetTaskId(), Page: exactReadPageFromProto(req.GetPage())})
	if err != nil {
		return nil, err
	}
	items := make([]*pluginv1.PendingTaskTransition, len(page.Items))
	for i, item := range page.Items {
		items[i] = pendingTaskTransitionToProto(item)
	}
	return &pluginv1.ListPendingTaskTransitionsExactResponse{Items: items, PageInfo: exactReadPageInfoToProto(page.PageInfo)}, nil
}
func (h grpcExactQueryManager) ListPendingTaskTransitions(ctx context.Context, q ExactPendingTaskTransitionsQuery) (ExactPendingTransitionsPage, error) {
	r, err := h.client.ListPendingTaskTransitionsExact(ctx, &pluginv1.ListPendingTaskTransitionsExactRequest{RequestId: q.RequestID, WorkspaceId: q.WorkspaceID, TaskId: q.TaskID, Page: exactReadPageToProto(q.Page)})
	if err != nil {
		return ExactPendingTransitionsPage{}, err
	}
	items := make([]PendingTaskTransition, len(r.GetItems()))
	for i, item := range r.GetItems() {
		items[i] = pendingTaskTransitionFromProto(item)
	}
	return ExactPendingTransitionsPage{Items: items, PageInfo: exactReadPageInfoFromProto(r.GetPageInfo())}, nil
}

func (s *grpcHostServer) ListChangeRequestEvidenceExact(ctx context.Context, req *pluginv1.ListChangeRequestEvidenceExactRequest) (*pluginv1.ListChangeRequestEvidenceExactResponse, error) {
	manager, err := s.exactQueryManager()
	if err != nil {
		return nil, err
	}
	page, err := manager.ListChangeRequestEvidence(ctx, ExactChangeRequestEvidenceQuery{RequestID: req.GetRequestId(), WorkspaceID: req.GetWorkspaceId(), TaskIDs: req.GetTaskIds(), Page: exactReadPageFromProto(req.GetPage())})
	if err != nil {
		return nil, err
	}
	items := make([]*pluginv1.ChangeRequestEvidence, len(page.Items))
	for i, item := range page.Items {
		items[i] = changeRequestEvidenceToProto(item)
	}
	return &pluginv1.ListChangeRequestEvidenceExactResponse{Items: items, PageInfo: exactReadPageInfoToProto(page.PageInfo)}, nil
}
func (h grpcExactQueryManager) ListChangeRequestEvidence(ctx context.Context, q ExactChangeRequestEvidenceQuery) (ExactChangeRequestEvidencePage, error) {
	r, err := h.client.ListChangeRequestEvidenceExact(ctx, &pluginv1.ListChangeRequestEvidenceExactRequest{RequestId: q.RequestID, WorkspaceId: q.WorkspaceID, TaskIds: q.TaskIDs, Page: exactReadPageToProto(q.Page)})
	if err != nil {
		return ExactChangeRequestEvidencePage{}, err
	}
	items := make([]ChangeRequestEvidence, len(r.GetItems()))
	for i, item := range r.GetItems() {
		items[i] = changeRequestEvidenceFromProto(item)
	}
	return ExactChangeRequestEvidencePage{Items: items, PageInfo: exactReadPageInfoFromProto(r.GetPageInfo())}, nil
}

func (s *grpcHostServer) ListTaskUsageExact(ctx context.Context, req *pluginv1.ListTaskUsageExactRequest) (*pluginv1.ListTaskUsageExactResponse, error) {
	manager, err := s.exactQueryManager()
	if err != nil {
		return nil, err
	}
	page, err := manager.ListTaskUsage(ctx, ExactTaskUsageQuery{RequestID: req.GetRequestId(), WorkspaceID: req.GetWorkspaceId(), TaskIDs: req.GetTaskIds(), Page: exactReadPageFromProto(req.GetPage())})
	if err != nil {
		return nil, err
	}
	items := make([]*pluginv1.TaskUsageObservation, len(page.Items))
	for i, item := range page.Items {
		items[i] = exactTaskUsageToProto(item)
	}
	return &pluginv1.ListTaskUsageExactResponse{Items: items, PageInfo: exactReadPageInfoToProto(page.PageInfo)}, nil
}
func (h grpcExactQueryManager) ListTaskUsage(ctx context.Context, q ExactTaskUsageQuery) (ExactTaskUsagePage, error) {
	r, err := h.client.ListTaskUsageExact(ctx, &pluginv1.ListTaskUsageExactRequest{RequestId: q.RequestID, WorkspaceId: q.WorkspaceID, TaskIds: q.TaskIDs, Page: exactReadPageToProto(q.Page)})
	if err != nil {
		return ExactTaskUsagePage{}, err
	}
	items := make([]TaskUsageObservation, len(r.GetItems()))
	for i, item := range r.GetItems() {
		items[i] = exactTaskUsageFromProto(item)
	}
	return ExactTaskUsagePage{Items: items, PageInfo: exactReadPageInfoFromProto(r.GetPageInfo())}, nil
}

func taskRelationsToProto(items []TaskRelation) []*pluginv1.TaskRelation {
	out := make([]*pluginv1.TaskRelation, len(items))
	for i, item := range items {
		out[i] = &pluginv1.TaskRelation{WorkspaceId: item.WorkspaceID, SourceTaskId: item.SourceTaskID, TargetTaskId: item.TargetTaskID, Kind: item.Kind, SourceResourceVersion: item.SourceResourceVersion, TargetResourceVersion: item.TargetResourceVersion, ResourceVersion: item.ResourceVersion}
	}
	return out
}
func taskRelationsFromProto(items []*pluginv1.TaskRelation) []TaskRelation {
	out := make([]TaskRelation, len(items))
	for i, item := range items {
		out[i] = TaskRelation{WorkspaceID: item.GetWorkspaceId(), SourceTaskID: item.GetSourceTaskId(), TargetTaskID: item.GetTargetTaskId(), Kind: item.GetKind(), SourceResourceVersion: item.GetSourceResourceVersion(), TargetResourceVersion: item.GetTargetResourceVersion(), ResourceVersion: item.GetResourceVersion()}
	}
	return out
}
func taskInboxItemToProto(item TaskInboxItem) *pluginv1.TaskInboxItem {
	return &pluginv1.TaskInboxItem{Id: item.ID, WorkspaceId: item.WorkspaceID, TaskId: item.TaskID, SessionId: item.SessionID, Kind: item.Kind, State: item.State, CreatedAt: item.CreatedAt, ResourceVersion: item.ResourceVersion}
}
func taskInboxItemFromProto(item *pluginv1.TaskInboxItem) TaskInboxItem {
	if item == nil {
		return TaskInboxItem{}
	}
	return TaskInboxItem{ID: item.GetId(), WorkspaceID: item.GetWorkspaceId(), TaskID: item.GetTaskId(), SessionID: item.GetSessionId(), Kind: item.GetKind(), State: item.GetState(), CreatedAt: item.GetCreatedAt(), ResourceVersion: item.GetResourceVersion()}
}
func taskDirectiveToProto(item TaskDirective) *pluginv1.TaskDirective {
	return &pluginv1.TaskDirective{Id: item.ID, WorkspaceId: item.WorkspaceID, TaskId: item.TaskID, CapabilityClass: item.CapabilityClass, State: item.State, InstructionDigest: item.InstructionDigest, ExpiresAt: item.ExpiresAt, ResourceVersion: item.ResourceVersion}
}
func taskDirectiveFromProto(item *pluginv1.TaskDirective) TaskDirective {
	if item == nil {
		return TaskDirective{}
	}
	return TaskDirective{ID: item.GetId(), WorkspaceID: item.GetWorkspaceId(), TaskID: item.GetTaskId(), CapabilityClass: item.GetCapabilityClass(), State: item.GetState(), InstructionDigest: item.GetInstructionDigest(), ExpiresAt: item.ExpiresAt, ResourceVersion: item.GetResourceVersion()}
}
func pendingTaskTransitionToProto(item PendingTaskTransition) *pluginv1.PendingTaskTransition {
	return &pluginv1.PendingTaskTransition{Id: item.ID, WorkspaceId: item.WorkspaceID, TaskId: item.TaskID, FromWorkflowId: item.FromWorkflowID, FromStepId: item.FromStepID, ToWorkflowId: item.ToWorkflowID, ToStepId: item.ToStepID, State: item.State, ResourceVersion: item.ResourceVersion, CreatedAt: item.CreatedAt}
}
func pendingTaskTransitionFromProto(item *pluginv1.PendingTaskTransition) PendingTaskTransition {
	if item == nil {
		return PendingTaskTransition{}
	}
	return PendingTaskTransition{ID: item.GetId(), WorkspaceID: item.GetWorkspaceId(), TaskID: item.GetTaskId(), FromWorkflowID: item.GetFromWorkflowId(), FromStepID: item.GetFromStepId(), ToWorkflowID: item.GetToWorkflowId(), ToStepID: item.GetToStepId(), State: item.GetState(), ResourceVersion: item.GetResourceVersion(), CreatedAt: item.GetCreatedAt()}
}
func changeRequestEvidenceToProto(item ChangeRequestEvidence) *pluginv1.ChangeRequestEvidence {
	return &pluginv1.ChangeRequestEvidence{TaskId: item.TaskID, Provider: item.Provider, RepositoryId: item.RepositoryID, RepositoryOwner: item.RepositoryOwner, RepositoryName: item.RepositoryName, Number: item.Number, Url: item.URL, HeadRevision: item.HeadRevision, ReviewState: item.ReviewState, ChecksState: item.ChecksState, MergeableState: item.MergeableState, UnresolvedReviewThreads: item.UnresolvedReviewThreads, ChecksTotal: item.ChecksTotal, ChecksPassing: item.ChecksPassing, ProviderState: item.ProviderState, ProviderError: item.ProviderError, FetchedAt: item.FetchedAt, ResourceVersion: item.ResourceVersion, ObservedAt: item.ObservedAt}
}
func changeRequestEvidenceFromProto(item *pluginv1.ChangeRequestEvidence) ChangeRequestEvidence {
	if item == nil {
		return ChangeRequestEvidence{}
	}
	return ChangeRequestEvidence{TaskID: item.GetTaskId(), Provider: item.GetProvider(), RepositoryID: item.GetRepositoryId(), RepositoryOwner: item.GetRepositoryOwner(), RepositoryName: item.GetRepositoryName(), Number: item.GetNumber(), URL: item.GetUrl(), HeadRevision: item.GetHeadRevision(), ReviewState: item.GetReviewState(), ChecksState: item.GetChecksState(), MergeableState: item.GetMergeableState(), UnresolvedReviewThreads: item.GetUnresolvedReviewThreads(), ChecksTotal: item.GetChecksTotal(), ChecksPassing: item.GetChecksPassing(), ProviderState: item.GetProviderState(), ProviderError: item.ProviderError, FetchedAt: item.FetchedAt, ResourceVersion: item.GetResourceVersion(), ObservedAt: item.GetObservedAt()}
}

var _ ExactQueryManager = grpcExactQueryManager{}
