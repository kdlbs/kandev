package pluginsdk

import (
	"context"
	pluginv1 "github.com/kandev/kandev/proto/kandev/plugin/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// AutomationAdapter is an optional provider-owned verification capability.
// A response cannot select an automation destination: the host binds that authority.
type AutomationAdapter interface {
	DescribeAutomationCondition(context.Context, *AutomationConditionRequest) (*AutomationConditionResponse, error)
	VerifyAutomationWebhook(context.Context, *AutomationWebhookRequest) (*AutomationWebhookResponse, error)
}

type AutomationConditionRequest = pluginv1.AutomationConditionRequest
type AutomationConditionResponse = pluginv1.AutomationConditionResponse
type AutomationWebhookRequest = pluginv1.AutomationWebhookRequest
type AutomationWebhookResponse = pluginv1.AutomationWebhookResponse

func (r *RemotePlugin) DescribeAutomationCondition(ctx context.Context, req *AutomationConditionRequest) (*AutomationConditionResponse, error) {
	return r.client.DescribeAutomationCondition(ctx, req)
}
func (r *RemotePlugin) VerifyAutomationWebhook(ctx context.Context, req *AutomationWebhookRequest) (*AutomationWebhookResponse, error) {
	return r.client.VerifyAutomationWebhook(ctx, req)
}
func (s *grpcPluginServer) DescribeAutomationCondition(ctx context.Context, req *AutomationConditionRequest) (*AutomationConditionResponse, error) {
	adapter, ok := s.impl.(AutomationAdapter)
	if !ok {
		return nil, status.Error(codes.Unimplemented, "automation adapters are not supported")
	}
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request required")
	}
	result, err := adapter.DescribeAutomationCondition(ctx, req)
	if err == nil && result == nil {
		return nil, status.Error(codes.Internal, "empty condition response")
	}
	return result, err
}
func (s *grpcPluginServer) VerifyAutomationWebhook(ctx context.Context, req *AutomationWebhookRequest) (*AutomationWebhookResponse, error) {
	adapter, ok := s.impl.(AutomationAdapter)
	if !ok {
		return nil, status.Error(codes.Unimplemented, "automation adapters are not supported")
	}
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request required")
	}
	result, err := adapter.VerifyAutomationWebhook(ctx, req)
	if err == nil && result == nil {
		return nil, status.Error(codes.Internal, "empty verification response")
	}
	return result, err
}
