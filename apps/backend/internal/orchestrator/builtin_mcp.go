package orchestrator

import (
	"context"
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/orchestrator/executor"
)

func preparedBuiltinPackages(ctx context.Context, taskID, sessionID string) *p.BuiltinManifest {
	profile := executor.PreparedMCPProfile(ctx, taskID, sessionID)
	if profile == nil {
		return nil
	}
	return profile.BuiltinPackages
}
