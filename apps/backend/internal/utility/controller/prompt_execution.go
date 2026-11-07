package controller

import (
	"context"

	agentctlutil "github.com/kandev/kandev/internal/agentctl/server/utility"
	"github.com/kandev/kandev/internal/utility/service"
)

func (c *Controller) CallInferenceContext(ctx context.Context, callID string) (context.Context, error) {
	return c.svc.CallInferenceContext(ctx, callID)
}

func (c *Controller) ExecuteProfilePrompt(ctx context.Context, route *service.PromptRequest,
	execute func(context.Context, string) (*agentctlutil.PromptResponse, error),
) (*agentctlutil.PromptResponse, error) {
	return service.ExecuteProfilePrompt(ctx, c.svc, route, execute)
}

func (c *Controller) SetCallExecutionAttribution(ctx context.Context, callID, profileID, model string) error {
	return c.svc.SetCallExecutionAttribution(ctx, callID, profileID, model)
}

// ReleasePromptCapacity removes only an unused early native reservation.
func (c *Controller) ReleasePromptCapacity(route *service.PromptRequest) {
	if route != nil {
		c.svc.ReleaseNativeRoute(route.RouteSessionID)
	}
}
