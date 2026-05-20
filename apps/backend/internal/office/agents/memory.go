package agents

import (
	"context"

	"github.com/kandev/kandev/internal/office/models"
)

// GetMemory returns a single memory entry by agent, layer, and key.
func (s *AgentService) GetMemory(ctx context.Context, agentInstanceID, layer, key string) (*models.AgentMemory, error) {
	return s.repo.GetAgentMemory(ctx, agentInstanceID, layer, key)
}

// ListMemory returns all memory entries for an agent, optionally filtered by layer.
func (s *AgentService) ListMemory(ctx context.Context, agentInstanceID, layer string) ([]*models.AgentMemory, error) {
	if layer != "" {
		return s.repo.ListAgentMemoryByLayer(ctx, agentInstanceID, layer)
	}
	return s.repo.ListAgentMemory(ctx, agentInstanceID)
}

// UpsertAgentMemory creates or updates an agent memory entry.
func (s *AgentService) UpsertAgentMemory(ctx context.Context, mem *models.AgentMemory) error {
	return s.repo.UpsertAgentMemory(ctx, mem)
}

// DeleteAgentMemory deletes a memory entry by ID.
func (s *AgentService) DeleteAgentMemory(ctx context.Context, id string) error {
	return s.repo.DeleteAgentMemory(ctx, id)
}

// DeleteAgentMemoryOwned deletes a memory entry by ID, verifying it belongs to agentInstanceID.
func (s *AgentService) DeleteAgentMemoryOwned(ctx context.Context, agentInstanceID, id string) error {
	return s.repo.DeleteAgentMemoryOwned(ctx, agentInstanceID, id)
}

// DeleteAllMemory deletes all memory entries for an agent.
func (s *AgentService) DeleteAllMemory(ctx context.Context, agentInstanceID string) error {
	return s.repo.DeleteAllAgentMemory(ctx, agentInstanceID)
}

// GetMemorySummary returns operating entries and recent knowledge entries.
func (s *AgentService) GetMemorySummary(
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
	result := make([]*models.AgentMemory, 0, len(operating)+len(knowledge))
	result = append(result, operating...)
	if len(knowledge) > 20 {
		knowledge = knowledge[len(knowledge)-20:]
	}
	result = append(result, knowledge...)
	return result, nil
}

// ExportMemory returns all memory entries for an agent.
func (s *AgentService) ExportMemory(ctx context.Context, agentInstanceID string) ([]*models.AgentMemory, error) {
	return s.repo.ListAgentMemory(ctx, agentInstanceID)
}
