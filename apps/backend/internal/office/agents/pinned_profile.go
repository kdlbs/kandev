package agents

import (
	"context"
	"fmt"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/routing"
)

func (s *AgentService) ConfigurePinnedProfile(ctx context.Context, agent *models.AgentInstance, id string) error {
	if err := s.ApplyProfileConfiguration(ctx, agent, id); err != nil {
		return err
	}
	profile, err := s.profileStore.GetAgentProfile(ctx, id)
	if err != nil {
		return err
	}
	if profile.Role != "" || profile.Model == "" {
		return fmt.Errorf("select an execution profile with a model, not an Office persona")
	}
	agent.Settings, err = routing.WriteAgentOverrides(agent.Settings, routing.AgentOverrides{ExecutionProfileID: id})
	return err
}
