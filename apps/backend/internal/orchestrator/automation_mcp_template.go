package orchestrator

import (
	"context"
	"errors"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/automation"
)

type automationMCPTemplateReader interface {
	AutomationMCPTaskTemplate(context.Context, string) (*mcpconfig.BuiltinCreationPreferences, []string, error)
}

func (s *Service) automationMCPTemplate(ctx context.Context, a *automation.Automation) (*mcpconfig.BuiltinCreationPreferences, []string, error) {
	if a.TaskMode != automation.TaskModeNormalTask {
		return nil, nil, nil
	}
	reader, ok := s.automationService.(automationMCPTemplateReader)
	if !ok {
		if a.MCPSelection == nil {
			return nil, nil, nil
		}
		return nil, nil, errors.New("automation MCP template reader unavailable")
	}
	return reader.AutomationMCPTaskTemplate(ctx, a.ID)
}
