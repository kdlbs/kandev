package service

import (
	"context"

	"github.com/kandev/kandev/internal/orchestrate/models"
)

// GetMemory returns a single memory entry by agent, layer, and key.
func (s *Service) GetMemory(ctx context.Context, agentInstanceID, layer, key string) (*models.AgentMemory, error) {
	return s.repo.GetAgentMemory(ctx, agentInstanceID, layer, key)
}

// ListMemory returns all memory entries for an agent, optionally filtered by layer.
func (s *Service) ListMemory(ctx context.Context, agentInstanceID, layer string) ([]*models.AgentMemory, error) {
	if layer != "" {
		return s.repo.ListAgentMemoryByLayer(ctx, agentInstanceID, layer)
	}
	return s.repo.ListAgentMemory(ctx, agentInstanceID)
}

// DeleteAllMemory deletes all memory entries for an agent.
func (s *Service) DeleteAllMemory(ctx context.Context, agentInstanceID string) error {
	return s.repo.DeleteAllAgentMemory(ctx, agentInstanceID)
}

// GetMemorySummary returns operating entries and recent knowledge entries.
func (s *Service) GetMemorySummary(
	ctx context.Context, agentInstanceID string,
) ([]*models.AgentMemory, error) {
	operating, err := s.repo.ListAgentMemoryByLayer(ctx, agentInstanceID, "operating")
	if err != nil {
		return nil, err
	}
	knowledge, err := s.repo.ListAgentMemoryByLayer(ctx, agentInstanceID, "knowledge")
	if err != nil {
		return nil, err
	}
	// Combine operating + knowledge (limited to most recent 20).
	result := make([]*models.AgentMemory, 0, len(operating)+len(knowledge))
	result = append(result, operating...)
	if len(knowledge) > 20 {
		knowledge = knowledge[len(knowledge)-20:]
	}
	result = append(result, knowledge...)
	return result, nil
}

// ExportMemory returns all memory entries for an agent.
func (s *Service) ExportMemory(ctx context.Context, agentInstanceID string) ([]*models.AgentMemory, error) {
	return s.repo.ListAgentMemory(ctx, agentInstanceID)
}
