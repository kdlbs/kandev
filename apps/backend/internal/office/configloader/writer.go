package configloader

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kandev/kandev/internal/office/models"
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

func isValidPathComponent(s string) bool {
	return s != "" && !strings.Contains(s, "/") && !strings.Contains(s, "\\") && !strings.Contains(s, "..")
}

// NewFileWriter creates a writer backed by the given config loader.
func NewFileWriter(basePath string, loader *ConfigLoader) *FileWriter {
	return &FileWriter{basePath: basePath, loader: loader}
}

// WorkspacePath returns the on-disk directory for a workspace.
func (w *FileWriter) WorkspacePath(workspaceName string) string {
	return filepath.Join(w.basePath, "workspaces", workspaceName)
}

// DeleteWorkspace removes a workspace directory from disk and reloads the cache.
func (w *FileWriter) DeleteWorkspace(workspaceName string) error {
	if !isValidPathComponent(workspaceName) {
		return fmt.Errorf("invalid workspace name")
	}
	if err := os.RemoveAll(w.WorkspacePath(workspaceName)); err != nil {
		return fmt.Errorf("delete workspace dir: %w", err)
	}
	return w.loader.Reload(workspaceName)
}

// WriteAgent marshals an agent to YAML and writes it to disk.
func (w *FileWriter) WriteAgent(workspaceName string, agent *models.AgentInstance) error {
	if !isValidPathComponent(workspaceName) || !isValidPathComponent(agent.Name) {
		return fmt.Errorf("invalid workspace or agent name")
	}
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
	if !isValidPathComponent(workspaceName) || !isValidPathComponent(agentName) {
		return fmt.Errorf("invalid workspace or agent name")
	}
	filePath := filepath.Join(w.basePath, "workspaces", workspaceName, "agents", agentName+".yml")
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete agent file: %w", err)
	}
	return w.loader.Reload(workspaceName)
}

// WriteSkill writes a SKILL.md file to the skill directory.
func (w *FileWriter) WriteSkill(workspaceName, slug, content string) error {
	if !isValidPathComponent(workspaceName) || !isValidPathComponent(slug) {
		return fmt.Errorf("invalid workspace name or skill slug")
	}
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
	if !isValidPathComponent(workspaceName) || !isValidPathComponent(slug) {
		return fmt.Errorf("invalid workspace name or skill slug")
	}
	dir := filepath.Join(w.basePath, "workspaces", workspaceName, "skills", slug)
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("delete skill dir: %w", err)
	}
	return w.loader.Reload(workspaceName)
}

// WriteProject marshals a project to YAML and writes it to disk.
func (w *FileWriter) WriteProject(workspaceName string, project *models.Project) error {
	if !isValidPathComponent(workspaceName) || !isValidPathComponent(project.Name) {
		return fmt.Errorf("invalid workspace or project name")
	}
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
	if !isValidPathComponent(workspaceName) || !isValidPathComponent(projectName) {
		return fmt.Errorf("invalid workspace or project name")
	}
	filePath := filepath.Join(w.basePath, "workspaces", workspaceName, "projects", projectName+".yml")
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete project file: %w", err)
	}
	return w.loader.Reload(workspaceName)
}

// WriteRoutine marshals a routine to YAML and writes it to disk.
func (w *FileWriter) WriteRoutine(workspaceName string, routine *models.Routine) error {
	if !isValidPathComponent(workspaceName) || !isValidPathComponent(routine.Name) {
		return fmt.Errorf("invalid workspace or routine name")
	}
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
	if !isValidPathComponent(workspaceName) || !isValidPathComponent(routineName) {
		return fmt.Errorf("invalid workspace or routine name")
	}
	filePath := filepath.Join(w.basePath, "workspaces", workspaceName, "routines", routineName+".yml")
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete routine file: %w", err)
	}
	return w.loader.Reload(workspaceName)
}

// WriteRawSettings writes pre-marshalled YAML bytes to the workspace kandev.yml
// and reloads the config loader.
func (w *FileWriter) WriteRawSettings(workspaceName string, data []byte) error {
	if !isValidPathComponent(workspaceName) {
		return fmt.Errorf("invalid workspace name")
	}
	wsDir := filepath.Join(w.basePath, "workspaces", workspaceName)
	if err := os.MkdirAll(wsDir, 0o755); err != nil {
		return fmt.Errorf("create workspace dir: %w", err)
	}
	settingsPath := filepath.Join(wsDir, "kandev.yml")
	if err := os.WriteFile(settingsPath, data, 0o644); err != nil {
		return fmt.Errorf("write settings: %w", err)
	}
	return w.loader.Reload(workspaceName)
}

// memoryPath computes the filesystem path for a memory entry.
func (w *FileWriter) memoryPath(workspaceName, agentName, layer, key string) (string, error) {
	if !isValidPathComponent(workspaceName) || !isValidPathComponent(agentName) ||
		!isValidPathComponent(layer) || !isValidPathComponent(key) {
		return "", fmt.Errorf("invalid memory path component")
	}
	return filepath.Join(
		w.basePath, "workspaces", workspaceName,
		"agents", agentName, "memory", layer, key+".md",
	), nil
}

// WriteMemoryEntry writes a markdown memory file to disk.
func (w *FileWriter) WriteMemoryEntry(workspaceName, agentName, layer, key, content string) error {
	filePath, err := w.memoryPath(workspaceName, agentName, layer, key)
	if err != nil {
		return err
	}
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
	filePath, err := w.memoryPath(workspaceName, agentName, layer, key)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("read memory file: %w", err)
	}
	return string(data), nil
}

// ListMemoryEntries lists all memory files for an agent within a given layer.
func (w *FileWriter) ListMemoryEntries(workspaceName, agentName, layer string) ([]MemoryEntry, error) {
	if !isValidPathComponent(workspaceName) || !isValidPathComponent(agentName) || !isValidPathComponent(layer) {
		return nil, fmt.Errorf("invalid memory path component")
	}
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
	filePath, err := w.memoryPath(workspaceName, agentName, layer, key)
	if err != nil {
		return err
	}
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete memory file: %w", err)
	}
	return nil
}
