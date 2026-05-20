package agents

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/office/configloader"
	"github.com/kandev/kandev/internal/office/models"
)

// ListInstructions returns all instruction files for an agent.
func (s *AgentService) ListInstructions(ctx context.Context, agentInstanceID string) ([]*models.InstructionFile, error) {
	return s.repo.ListInstructions(ctx, agentInstanceID)
}

// GetInstruction returns a single instruction file by agent and filename.
func (s *AgentService) GetInstruction(ctx context.Context, agentInstanceID, filename string) (*models.InstructionFile, error) {
	return s.repo.GetInstruction(ctx, agentInstanceID, filename)
}

// UpsertInstruction creates or updates an instruction file.
func (s *AgentService) UpsertInstruction(
	ctx context.Context, agentInstanceID, filename, content string, isEntry bool,
) error {
	return s.repo.UpsertInstruction(ctx, agentInstanceID, filename, content, isEntry)
}

// DeleteInstruction deletes an instruction file.
func (s *AgentService) DeleteInstruction(ctx context.Context, agentInstanceID, filename string) error {
	return s.repo.DeleteInstruction(ctx, agentInstanceID, filename)
}

// CreateDefaultInstructions creates default instruction files for an agent based on its role.
// Templates are loaded from embedded files in configloader/instructions/<role>/.
func (s *AgentService) CreateDefaultInstructions(ctx context.Context, agentInstanceID, role string) error {
	templates, err := configloader.LoadRoleTemplates(role)
	if err != nil {
		return fmt.Errorf("load role templates for %s: %w", role, err)
	}
	for _, t := range templates {
		isEntry := t.Filename == "AGENTS.md"
		if upsertErr := s.repo.UpsertInstruction(ctx, agentInstanceID, t.Filename, t.Content, isEntry); upsertErr != nil {
			return fmt.Errorf("upsert instruction %s: %w", t.Filename, upsertErr)
		}
	}
	return nil
}
