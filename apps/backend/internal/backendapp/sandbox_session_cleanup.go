package backendapp

import (
	"context"
	policy "github.com/kandev/kandev/internal/agent/sandbox"
)

type sandboxSessionCleanup struct {
	policies *policy.Service
	remove   func(context.Context, string, string) error
}

func (c sandboxSessionCleanup) cleanup(ctx context.Context, taskID, sessionID string) error {
	if taskID == "" || sessionID == "" {
		return policy.ErrConflict
	}
	if c.remove != nil {
		return c.remove(ctx, taskID, sessionID)
	}
	if c.policies == nil {
		return errSandboxRuntimeUnavailable
	}
	found, err := c.policies.HasSessionAttempts(ctx, sessionID)
	if err != nil {
		return err
	}
	if found {
		return errSandboxRuntimeUnavailable
	}
	return nil
}
