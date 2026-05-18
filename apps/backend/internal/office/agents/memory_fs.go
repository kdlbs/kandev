package agents

import (
	"fmt"

	"github.com/kandev/kandev/internal/office/configloader"
	"github.com/kandev/kandev/internal/office/models"
)

// defaultWorkspaceName is used for ConfigLoader lookups when we only have a
// DB workspace ID. Most single-user installs have one workspace named "default".
const defaultWorkspaceName = "default"

// SetConfigWriter sets the filesystem config writer for memory FS operations.
func (s *AgentService) SetConfigWriter(w *configloader.FileWriter) {
	s.cfgWriter = w
}

// ListMemoryFromConfig returns memory entries from the filesystem.
func (s *AgentService) ListMemoryFromConfig(agentName, layer string) ([]*models.AgentMemory, error) {
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
func (s *AgentService) UpsertMemoryFromConfig(agentName, layer, key, content string) error {
	if s.cfgWriter == nil {
		return fmt.Errorf("config writer not initialized")
	}
	return s.cfgWriter.WriteMemoryEntry(defaultWorkspaceName, agentName, layer, key, content)
}

// DeleteMemoryFromConfig deletes a memory entry from the filesystem.
func (s *AgentService) DeleteMemoryFromConfig(agentName, layer, key string) error {
	if s.cfgWriter == nil {
		return fmt.Errorf("config writer not initialized")
	}
	return s.cfgWriter.DeleteMemoryEntry(defaultWorkspaceName, agentName, layer, key)
}

// GetMemoryFromConfig reads a single memory entry from the filesystem.
func (s *AgentService) GetMemoryFromConfig(agentName, layer, key string) (string, error) {
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
			AgentProfileID: agentName,
			Layer:          e.Layer,
			Key:            e.Key,
			Content:        e.Content,
		}
	}
	return result
}
