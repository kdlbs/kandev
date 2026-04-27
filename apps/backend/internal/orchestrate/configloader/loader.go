package configloader

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/orchestrate/models"
)

// WorkspaceConfig holds the parsed configuration for a single workspace.
type WorkspaceConfig struct {
	Name     string
	DirPath  string
	Settings WorkspaceSettings
	Agents   map[string]*models.AgentInstance
	Skills   map[string]*SkillInfo
	Projects map[string]*models.Project
	Routines map[string]*models.Routine
}

// SkillInfo wraps a skill model with its on-disk directory path.
type SkillInfo struct {
	models.Skill
	DirPath string
}

// ConfigError records a parse/read error for a workspace file.
type ConfigError struct {
	WorkspaceID string
	FilePath    string
	Error       string
	Timestamp   time.Time
}

// ConfigLoader reads and caches workspace configurations from the filesystem.
type ConfigLoader struct {
	basePath   string
	mu         sync.RWMutex
	workspaces map[string]*WorkspaceConfig
	errors     map[string]*ConfigError
}

// NewConfigLoader creates a loader rooted at the given base path (e.g. ~/.kandev).
func NewConfigLoader(basePath string) *ConfigLoader {
	return &ConfigLoader{
		basePath:   basePath,
		workspaces: make(map[string]*WorkspaceConfig),
		errors:     make(map[string]*ConfigError),
	}
}

// BasePath returns the root configuration directory.
func (cl *ConfigLoader) BasePath() string {
	return cl.basePath
}

// Load scans all workspace directories and reads their configuration files.
func (cl *ConfigLoader) Load() error {
	wsDir := filepath.Join(cl.basePath, "workspaces")
	entries, err := os.ReadDir(wsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read workspaces dir: %w", err)
	}
	cl.mu.Lock()
	defer cl.mu.Unlock()
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		cl.loadWorkspaceLocked(name)
	}
	return nil
}

// loadWorkspaceLocked reads a single workspace. Caller must hold cl.mu.
func (cl *ConfigLoader) loadWorkspaceLocked(name string) {
	wsPath := filepath.Join(cl.basePath, "workspaces", name)
	cfg := &WorkspaceConfig{
		Name:     name,
		DirPath:  wsPath,
		Agents:   make(map[string]*models.AgentInstance),
		Skills:   make(map[string]*SkillInfo),
		Projects: make(map[string]*models.Project),
		Routines: make(map[string]*models.Routine),
	}

	// Read kandev.yml settings.
	settingsPath := filepath.Join(wsPath, "kandev.yml")
	if data, err := os.ReadFile(settingsPath); err == nil {
		settings, parseErr := UnmarshalSettings(data)
		if parseErr != nil {
			cl.recordErrorLocked(name, settingsPath, parseErr)
			return
		}
		cfg.Settings = settings
	}

	// Track whether new errors were recorded during this load.
	prevErr := cl.errors[name]
	delete(cl.errors, name)

	cl.loadAgentsLocked(cfg, wsPath, name)
	cl.loadSkillsLocked(cfg, wsPath, name)
	cl.loadProjectsLocked(cfg, wsPath, name)
	cl.loadRoutinesLocked(cfg, wsPath, name)

	// If no new error was recorded but we had a previous one, it is cleared.
	// If a new error was recorded during sub-loads, it persists.
	_ = prevErr
	cl.workspaces[name] = cfg
}

// loadAgentsLocked reads agents/*.yml into the workspace config.
func (cl *ConfigLoader) loadAgentsLocked(cfg *WorkspaceConfig, wsPath, wsName string) {
	agentsDir := filepath.Join(wsPath, "agents")
	entries, err := os.ReadDir(agentsDir)
	if err != nil {
		return // agents dir may not exist
	}
	for _, entry := range entries {
		if entry.IsDir() || !isYAMLFile(entry.Name()) {
			continue
		}
		filePath := filepath.Join(agentsDir, entry.Name())
		data, readErr := os.ReadFile(filePath)
		if readErr != nil {
			cl.recordErrorLocked(wsName, filePath, readErr)
			continue
		}
		agent, parseErr := UnmarshalAgent(data, wsName)
		if parseErr != nil {
			cl.recordErrorLocked(wsName, filePath, parseErr)
			continue
		}
		cfg.Agents[agent.Name] = agent
	}
}

// loadSkillsLocked reads skills/*/SKILL.md into the workspace config.
func (cl *ConfigLoader) loadSkillsLocked(cfg *WorkspaceConfig, wsPath, wsName string) {
	skillsDir := filepath.Join(wsPath, "skills")
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		slug := entry.Name()
		dirPath := filepath.Join(skillsDir, slug)
		skillFile := filepath.Join(dirPath, "SKILL.md")
		content, readErr := os.ReadFile(skillFile)
		if readErr != nil {
			continue // skip dirs without SKILL.md
		}
		cfg.Skills[slug] = &SkillInfo{
			Skill: models.Skill{
				ID:          slug,
				WorkspaceID: wsName,
				Name:        slug,
				Slug:        slug,
				SourceType:  "filesystem",
				Content:     string(content),
			},
			DirPath: dirPath,
		}
	}
}

// loadProjectsLocked reads projects/*.yml into the workspace config.
func (cl *ConfigLoader) loadProjectsLocked(cfg *WorkspaceConfig, wsPath, wsName string) {
	projectsDir := filepath.Join(wsPath, "projects")
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || !isYAMLFile(entry.Name()) {
			continue
		}
		filePath := filepath.Join(projectsDir, entry.Name())
		data, readErr := os.ReadFile(filePath)
		if readErr != nil {
			cl.recordErrorLocked(wsName, filePath, readErr)
			continue
		}
		project, parseErr := UnmarshalProject(data, wsName)
		if parseErr != nil {
			cl.recordErrorLocked(wsName, filePath, parseErr)
			continue
		}
		cfg.Projects[project.Name] = project
	}
}

// loadRoutinesLocked reads routines/*.yml into the workspace config.
func (cl *ConfigLoader) loadRoutinesLocked(cfg *WorkspaceConfig, wsPath, wsName string) {
	routinesDir := filepath.Join(wsPath, "routines")
	entries, err := os.ReadDir(routinesDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || !isYAMLFile(entry.Name()) {
			continue
		}
		filePath := filepath.Join(routinesDir, entry.Name())
		data, readErr := os.ReadFile(filePath)
		if readErr != nil {
			cl.recordErrorLocked(wsName, filePath, readErr)
			continue
		}
		routine, parseErr := UnmarshalRoutine(data, wsName)
		if parseErr != nil {
			cl.recordErrorLocked(wsName, filePath, parseErr)
			continue
		}
		cfg.Routines[routine.Name] = routine
	}
}

func (cl *ConfigLoader) recordErrorLocked(wsName, filePath string, err error) {
	cl.errors[wsName] = &ConfigError{
		WorkspaceID: wsName,
		FilePath:    filePath,
		Error:       err.Error(),
		Timestamp:   time.Now(),
	}
}

// GetWorkspaces returns all loaded workspace configs.
func (cl *ConfigLoader) GetWorkspaces() []WorkspaceConfig {
	cl.mu.RLock()
	defer cl.mu.RUnlock()
	result := make([]WorkspaceConfig, 0, len(cl.workspaces))
	for _, ws := range cl.workspaces {
		result = append(result, *ws)
	}
	return result
}

// GetWorkspace returns a single workspace config by name.
func (cl *ConfigLoader) GetWorkspace(name string) (*WorkspaceConfig, error) {
	cl.mu.RLock()
	defer cl.mu.RUnlock()
	ws, ok := cl.workspaces[name]
	if !ok {
		return nil, fmt.Errorf("workspace not found: %s", name)
	}
	return ws, nil
}

// GetAgents returns all agents for a workspace.
func (cl *ConfigLoader) GetAgents(workspaceName string) []*models.AgentInstance {
	cl.mu.RLock()
	defer cl.mu.RUnlock()
	ws, ok := cl.workspaces[workspaceName]
	if !ok {
		return nil
	}
	result := make([]*models.AgentInstance, 0, len(ws.Agents))
	for _, a := range ws.Agents {
		result = append(result, a)
	}
	return result
}

// GetSkills returns all skills for a workspace.
func (cl *ConfigLoader) GetSkills(workspaceName string) []*SkillInfo {
	cl.mu.RLock()
	defer cl.mu.RUnlock()
	ws, ok := cl.workspaces[workspaceName]
	if !ok {
		return nil
	}
	result := make([]*SkillInfo, 0, len(ws.Skills))
	for _, s := range ws.Skills {
		result = append(result, s)
	}
	return result
}

// GetProjects returns all projects for a workspace.
func (cl *ConfigLoader) GetProjects(workspaceName string) []*models.Project {
	cl.mu.RLock()
	defer cl.mu.RUnlock()
	ws, ok := cl.workspaces[workspaceName]
	if !ok {
		return nil
	}
	result := make([]*models.Project, 0, len(ws.Projects))
	for _, p := range ws.Projects {
		result = append(result, p)
	}
	return result
}

// GetRoutines returns all routines for a workspace.
func (cl *ConfigLoader) GetRoutines(workspaceName string) []*models.Routine {
	cl.mu.RLock()
	defer cl.mu.RUnlock()
	ws, ok := cl.workspaces[workspaceName]
	if !ok {
		return nil
	}
	result := make([]*models.Routine, 0, len(ws.Routines))
	for _, r := range ws.Routines {
		result = append(result, r)
	}
	return result
}

// GetErrors returns all current config errors.
func (cl *ConfigLoader) GetErrors() []*ConfigError {
	cl.mu.RLock()
	defer cl.mu.RUnlock()
	result := make([]*ConfigError, 0, len(cl.errors))
	for _, e := range cl.errors {
		result = append(result, e)
	}
	return result
}

// Reload re-reads a single workspace from disk.
func (cl *ConfigLoader) Reload(workspaceName string) error {
	wsPath := filepath.Join(cl.basePath, "workspaces", workspaceName)
	if _, err := os.Stat(wsPath); err != nil {
		if os.IsNotExist(err) {
			cl.mu.Lock()
			delete(cl.workspaces, workspaceName)
			delete(cl.errors, workspaceName)
			cl.mu.Unlock()
			return nil
		}
		return err
	}
	cl.mu.Lock()
	defer cl.mu.Unlock()
	cl.loadWorkspaceLocked(workspaceName)
	return nil
}

// WorkspaceNameFromPath extracts the workspace name from an absolute path
// under basePath/workspaces/<name>/...
func (cl *ConfigLoader) WorkspaceNameFromPath(path string) string {
	wsDir := filepath.Join(cl.basePath, "workspaces") + string(os.PathSeparator)
	if !strings.HasPrefix(path, wsDir) {
		return ""
	}
	rel := path[len(wsDir):]
	parts := strings.SplitN(rel, string(os.PathSeparator), 2)
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}

// EnsureDefaultWorkspace creates the default workspace directory and kandev.yml if missing.
func (cl *ConfigLoader) EnsureDefaultWorkspace() error {
	wsDir := filepath.Join(cl.basePath, "workspaces", "default")
	if err := os.MkdirAll(wsDir, 0o755); err != nil {
		return fmt.Errorf("create default workspace dir: %w", err)
	}
	settingsPath := filepath.Join(wsDir, "kandev.yml")
	if _, err := os.Stat(settingsPath); err == nil {
		return nil // already exists
	}
	defaults := WorkspaceSettings{
		Name:        "default",
		Slug:        "default",
		Description: "Default workspace",
	}
	data, err := MarshalSettings(defaults)
	if err != nil {
		return fmt.Errorf("marshal default settings: %w", err)
	}
	return os.WriteFile(settingsPath, data, 0o644)
}
