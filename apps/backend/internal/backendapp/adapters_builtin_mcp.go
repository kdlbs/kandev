package backendapp

import (
	"context"
	p "github.com/kandev/kandev/internal/mcp/profile"
)

func (w *orchestratorWrapper) PrepareBuiltinIntent(ctx context.Context, taskID, sessionID, intent string) (context.Context, error) {
	return w.svc.PrepareBuiltinIntent(ctx, taskID, sessionID, intent)
}

func (w *orchestratorWrapper) PrepareTaskSessionMCPPrompt(ctx context.Context, taskID, sessionID string) (context.Context, error) {
	return w.svc.PrepareTaskSessionMCPPrompt(ctx, taskID, sessionID)
}

func (a *lifecycleAdapter) PrepareBuiltinWorkspace(ctx context.Context, executionID string, profile p.Context) error {
	return a.mgr.PrepareBuiltinWorkspace(ctx, executionID, profile)
}
