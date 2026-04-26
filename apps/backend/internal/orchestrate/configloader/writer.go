package configloader

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kandev/kandev/internal/orchestrate/models"
)

// MemoryEntry represents a single memory file on disk.
type MemoryEntry struct {
	Layer   string
	Key     string
	Content string
}

// FileWriter handles CRUD operations that write config files to disk.
type FileWriter struct {
	basePath string
	loader   *ConfigLoader
}

// NewFileWriter creates a writer backed by the given config loader.
func NewFileWriter(basePath string, loader *ConfigLoader) *FileWriter {
	return &FileWriter{basePath: basePath, loader: loader}
}

// WriteAgent marshals an agent to YAML and writes it to disk.
func (w *FileWriter) WriteAgent(workspaceName string, agent *models.AgentInstance) error {
	dir := filepath.Join(w.basePath, "workspaces", workspaceName, "agents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create agents dir: %w", err)
	}
	data, err := MarshalAgent(agent)
	if err != nil {
		return err
	}
	filePath := filepath.Join(dir, agent.Name+".yml")
	if err := os.WriteFile(filePath, data, 0o644); err != nil {
		return fmt.Errorf("write agent file: %w", err)
	}
	return w.loader.Reload(workspaceName)
}

// DeleteAgent removes an agent YAML file from disk.
func (w *FileWriter) DeleteAgent(workspaceName, agentName string) error {
	filePath := filepath.Join(w.basePath, "workspaces", workspaceName, "agents", agentName+".yml")
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete agent file: %w", err)
	}
	return w.loader.Reload(workspaceName)
}

// WriteSkill writes a SKILL.md file to the skill directory.
func (w *FileWriter) WriteSkill(workspaceName, slug, content string) error {
	dir := filepath.Join(w.basePath, "workspaces", workspaceName, "skills", slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create skill dir: %w", err)
	}
	filePath := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write skill file: %w", err)
	}
	return w.loader.Reload(workspaceName)
}

// DeleteSkill removes a skill directory from disk.
func (w *FileWriter) DeleteSkill(workspaceName, slug string) error {
	dir := filepath.Join(w.basePath, "workspaces", workspaceName, "skills", slug)
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("delete skill dir: %w", err)
	}
	return w.loader.Reload(workspaceName)
}

// WriteProject marshals a project to YAML and writes it to disk.
func (w *FileWriter) WriteProject(workspaceName string, project *models.Project) error {
	dir := filepath.Join(w.basePath, "workspaces", workspaceName, "projects")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create projects dir: %w", err)
	}
	data, err := MarshalProject(project)
	if err != nil {
		return err
	}
	filePath := filepath.Join(dir, project.Name+".yml")
	if err := os.WriteFile(filePath, data, 0o644); err != nil {
		return fmt.Errorf("write project file: %w", err)
	}
	return w.loader.Reload(workspaceName)
}

// DeleteProject removes a project YAML file from disk.
func (w *FileWriter) DeleteProject(workspaceName, projectName string) error {
	filePath := filepath.Join(w.basePath, "workspaces", workspaceName, "projects", projectName+".yml")
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete project file: %w", err)
	}
	return w.loader.Reload(workspaceName)
}

// WriteRoutine marshals a routine to YAML and writes it to disk.
func (w *FileWriter) WriteRoutine(workspaceName string, routine *models.Routine) error {
	dir := filepath.Join(w.basePath, "workspaces", workspaceName, "routines")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create routines dir: %w", err)
	}
	data, err := MarshalRoutine(routine)
	if err != nil {
		return err
	}
	filePath := filepath.Join(dir, routine.Name+".yml")
	if err := os.WriteFile(filePath, data, 0o644); err != nil {
		return fmt.Errorf("write routine file: %w", err)
	}
	return w.loader.Reload(workspaceName)
}

// DeleteRoutine removes a routine YAML file from disk.
func (w *FileWriter) DeleteRoutine(workspaceName, routineName string) error {
	filePath := filepath.Join(w.basePath, "workspaces", workspaceName, "routines", routineName+".yml")
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete routine file: %w", err)
	}
	return w.loader.Reload(workspaceName)
}

// memoryPath computes the filesystem path for a memory entry.
func (w *FileWriter) memoryPath(workspaceName, agentName, layer, key string) string {
	return filepath.Join(
		w.basePath, "workspaces", workspaceName,
		"agents", agentName, "memory", layer, key+".md",
	)
}

// WriteMemoryEntry writes a markdown memory file to disk.
func (w *FileWriter) WriteMemoryEntry(workspaceName, agentName, layer, key, content string) error {
	filePath := w.memoryPath(workspaceName, agentName, layer, key)
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create memory dir: %w", err)
	}
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write memory file: %w", err)
	}
	return nil
}

// ReadMemoryEntry reads a markdown memory file from disk.
func (w *FileWriter) ReadMemoryEntry(workspaceName, agentName, layer, key string) (string, error) {
	filePath := w.memoryPath(workspaceName, agentName, layer, key)
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("read memory file: %w", err)
	}
	return string(data), nil
}

// ListMemoryEntries lists all memory files for an agent within a given layer.
func (w *FileWriter) ListMemoryEntries(workspaceName, agentName, layer string) ([]MemoryEntry, error) {
	dir := filepath.Join(
		w.basePath, "workspaces", workspaceName,
		"agents", agentName, "memory", layer,
	)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read memory dir: %w", err)
	}
	var result []MemoryEntry
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		key := strings.TrimSuffix(entry.Name(), ".md")
		content, readErr := os.ReadFile(filepath.Join(dir, entry.Name()))
		if readErr != nil {
			continue
		}
		result = append(result, MemoryEntry{
			Layer:   layer,
			Key:     key,
			Content: string(content),
		})
	}
	return result, nil
}

// DeleteMemoryEntry removes a markdown memory file from disk.
func (w *FileWriter) DeleteMemoryEntry(workspaceName, agentName, layer, key string) error {
	filePath := w.memoryPath(workspaceName, agentName, layer, key)
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete memory file: %w", err)
	}
	return nil
}
