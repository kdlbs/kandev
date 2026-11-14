package pluginsdk

import (
	"context"

	pluginv1 "github.com/kdlbs/kandev/pluginsdk/proto/kandev/plugin/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *grpcHostServer) automationHistoryReader() (AutomationHistoryReader, error) {
	if s.automationHistory == nil || s.automationHistory.AutomationHistory() == nil {
		return nil, status.Error(codes.Unimplemented, "automation history reads are unavailable")
	}
	return s.automationHistory.AutomationHistory(), nil
}

func (s *grpcHostServer) ListAutomationDefinitionsExact(ctx context.Context, request *pluginv1.ListAutomationDefinitionsExactRequest) (*pluginv1.ListAutomationDefinitionsExactResponse, error) {
	reader, err := s.automationHistoryReader()
	if err != nil {
		return nil, err
	}
	page, err := reader.ListAutomations(ctx, AutomationDefinitionQuery{
		RequestID: request.GetRequestId(), WorkspaceID: request.GetWorkspaceId(),
		Filter: automationDefinitionFilterFromProto(request.GetFilter()),
		Page:   exactReadPageFromProto(request.GetPage()),
	})
	if err != nil {
		return nil, err
	}
	items := make([]*pluginv1.AutomationDefinitionObservation, len(page.Items))
	for i, item := range page.Items {
		items[i] = automationDefinitionObservationToProto(item)
	}
	return &pluginv1.ListAutomationDefinitionsExactResponse{
		Items: items, PageInfo: exactReadPageInfoToProto(page.PageInfo),
	}, nil
}

func (s *grpcHostServer) ListAutomationRunsExact(ctx context.Context, request *pluginv1.ListAutomationRunsExactRequest) (*pluginv1.ListAutomationRunsExactResponse, error) {
	reader, err := s.automationHistoryReader()
	if err != nil {
		return nil, err
	}
	page, err := reader.ListRuns(ctx, AutomationRunQuery{
		RequestID: request.GetRequestId(), WorkspaceID: request.GetWorkspaceId(),
		Filter: automationRunFilterFromProto(request.GetFilter()),
		Page:   exactReadPageFromProto(request.GetPage()),
	})
	if err != nil {
		return nil, err
	}
	items := make([]*pluginv1.AutomationRunObservation, len(page.Items))
	for i, item := range page.Items {
		items[i] = automationRunObservationToProto(item)
	}
	return &pluginv1.ListAutomationRunsExactResponse{
		Items: items, PageInfo: exactReadPageInfoToProto(page.PageInfo),
	}, nil
}

func (s *grpcHostServer) GetAutomationRunExact(ctx context.Context, request *pluginv1.GetAutomationRunExactRequest) (*pluginv1.GetAutomationRunExactResponse, error) {
	reader, err := s.automationHistoryReader()
	if err != nil {
		return nil, err
	}
	item, receipt, err := reader.GetRun(ctx, AutomationRunGetQuery{
		RequestID: request.GetRequestId(), WorkspaceID: request.GetWorkspaceId(), RunID: request.GetRunId(),
	})
	if err != nil {
		return nil, err
	}
	return &pluginv1.GetAutomationRunExactResponse{
		Item: automationRunObservationToProto(item), Receipt: hostReadReceiptToProto(receipt),
	}, nil
}
