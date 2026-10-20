package backendapp

import (
	"context"
	"fmt"

	officemodels "github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/routing"
)

type officeLaunchProfileAgentReader interface {
	GetAgentInstance(ctx context.Context, id string) (*officemodels.AgentInstance, error)
}

type officeLaunchProfileResolver interface {
	Resolve(
		ctx context.Context, workspaceID string, agent officemodels.AgentInstance, opts routing.ResolveOptions,
	) (*routing.Resolution, error)
}

// officeLaunchProfilePreview answers which execution profile Office dispatch
// would launch an agent with, using the dispatcher's read-only resolver.
type officeLaunchProfilePreview struct {
	agents   officeLaunchProfileAgentReader
	resolver officeLaunchProfileResolver
}

// Preview returns the first routed candidate's execution profile, or "" when
// routing selects no candidate and dispatch would launch the agent directly.
func (p *officeLaunchProfilePreview) Preview(ctx context.Context, workspaceID, agentID string) (string, error) {
	agent, err := p.agents.GetAgentInstance(ctx, agentID)
	if err != nil {
		return "", fmt.Errorf("load office agent %q: %w", agentID, err)
	}
	if agent == nil {
		return "", fmt.Errorf("load office agent %q: not found", agentID)
	}
	res, err := p.resolver.Resolve(ctx, workspaceID, *agent, routing.ResolveOptions{})
	if err != nil {
		return "", fmt.Errorf("resolve office launch profile: %w", err)
	}
	if res == nil || len(res.Candidates) == 0 {
		return "", nil
	}
	return res.Candidates[0].ExecutionProfileID, nil
}
