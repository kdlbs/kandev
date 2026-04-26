package service

import (
	"fmt"

	"github.com/kandev/kandev/internal/orchestrate/configloader"
	"github.com/kandev/kandev/internal/orchestrate/models"
)

// ListMemoryFromConfig returns memory entries from the filesystem.
func (s *Service) ListMemoryFromConfig(agentName, layer string) ([]*models.AgentMemory, error) {
	if s.cfgWriter == nil {
		return nil, fmt.Errorf("config writer not initialized")
	}
	entries, err := s.cfgWriter.ListMemoryEntries(defaultWorkspaceName, agentName, layer)
	if err != nil {
		return nil, err
	}
	return memoryEntriesToModels(agentName, entries), nil
}

// UpsertMemoryFromConfig writes a memory entry to the filesystem.
func (s *Service) UpsertMemoryFromConfig(agentName, layer, key, content string) error {
	if s.cfgWriter == nil {
		return fmt.Errorf("config writer not initialized")
	}
	return s.cfgWriter.WriteMemoryEntry(defaultWorkspaceName, agentName, layer, key, content)
}

// DeleteMemoryFromConfig deletes a memory entry from the filesystem.
func (s *Service) DeleteMemoryFromConfig(agentName, layer, key string) error {
	if s.cfgWriter == nil {
		return fmt.Errorf("config writer not initialized")
	}
	return s.cfgWriter.DeleteMemoryEntry(defaultWorkspaceName, agentName, layer, key)
}

// GetMemoryFromConfig reads a single memory entry from the filesystem.
func (s *Service) GetMemoryFromConfig(agentName, layer, key string) (string, error) {
	if s.cfgWriter == nil {
		return "", fmt.Errorf("config writer not initialized")
	}
	return s.cfgWriter.ReadMemoryEntry(defaultWorkspaceName, agentName, layer, key)
}

// memoryEntriesToModels converts configloader.MemoryEntry slices to AgentMemory models.
func memoryEntriesToModels(agentName string, entries []configloader.MemoryEntry) []*models.AgentMemory {
	result := make([]*models.AgentMemory, len(entries))
	for i, e := range entries {
		result[i] = &models.AgentMemory{
			AgentInstanceID: agentName,
			Layer:           e.Layer,
			Key:             e.Key,
			Content:         e.Content,
		}
	}
	return result
}
