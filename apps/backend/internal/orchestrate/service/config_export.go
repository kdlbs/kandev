package service

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"

	"gopkg.in/yaml.v3"
)

// ConfigBundle represents a portable workspace configuration.
type ConfigBundle struct {
	Settings SettingsConfig  `json:"settings" yaml:"settings"`
	Agents   []AgentConfig   `json:"agents" yaml:"agents"`
	Skills   []SkillConfig   `json:"skills" yaml:"skills"`
	Routines []RoutineConfig `json:"routines" yaml:"routines"`
	Projects []ProjectConfig `json:"projects" yaml:"projects"`
}

// SettingsConfig represents workspace-level settings.
type SettingsConfig struct {
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
}

// AgentConfig represents a portable agent instance configuration.
type AgentConfig struct {
	Name                  string `json:"name" yaml:"name"`
	Role                  string `json:"role" yaml:"role"`
	Icon                  string `json:"icon,omitempty" yaml:"icon,omitempty"`
	ReportsTo             string `json:"reports_to,omitempty" yaml:"reports_to,omitempty"`
	BudgetMonthlyCents    int    `json:"budget_monthly_cents" yaml:"budget_monthly_cents"`
	MaxConcurrentSessions int    `json:"max_concurrent_sessions" yaml:"max_concurrent_sessions"`
	DesiredSkills         string `json:"desired_skills,omitempty" yaml:"desired_skills,omitempty"`
	ExecutorPreference    string `json:"executor_preference,omitempty" yaml:"executor_preference,omitempty"`
}

// SkillConfig represents a portable skill configuration.
type SkillConfig struct {
	Name        string `json:"name" yaml:"name"`
	Slug        string `json:"slug" yaml:"slug"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	SourceType  string `json:"source_type" yaml:"source_type"`
	Content     string `json:"content,omitempty" yaml:"content,omitempty"`
}

// RoutineConfig represents a portable routine configuration.
type RoutineConfig struct {
	Name              string `json:"name" yaml:"name"`
	Description       string `json:"description,omitempty" yaml:"description,omitempty"`
	TaskTemplate      string `json:"task_template,omitempty" yaml:"task_template,omitempty"`
	AssigneeName      string `json:"assignee_name,omitempty" yaml:"assignee_name,omitempty"`
	ConcurrencyPolicy string `json:"concurrency_policy,omitempty" yaml:"concurrency_policy,omitempty"`
}

// ProjectConfig represents a portable project configuration.
type ProjectConfig struct {
	Name           string `json:"name" yaml:"name"`
	Description    string `json:"description,omitempty" yaml:"description,omitempty"`
	Status         string `json:"status,omitempty" yaml:"status,omitempty"`
	Color          string `json:"color,omitempty" yaml:"color,omitempty"`
	BudgetCents    int    `json:"budget_cents,omitempty" yaml:"budget_cents,omitempty"`
	Repositories   string `json:"repositories,omitempty" yaml:"repositories,omitempty"`
	ExecutorConfig string `json:"executor_config,omitempty" yaml:"executor_config,omitempty"`
	LeadAgentName  string `json:"lead_agent_name,omitempty" yaml:"lead_agent_name,omitempty"`
}

// ExportBundle exports the full workspace configuration as a ConfigBundle.
func (s *Service) ExportBundle(ctx context.Context, workspaceID string) (*ConfigBundle, error) {
	bundle := &ConfigBundle{
		Settings: SettingsConfig{Name: workspaceID},
	}

	if err := s.exportAgents(ctx, workspaceID, bundle); err != nil {
		return nil, err
	}
	if err := s.exportSkills(ctx, workspaceID, bundle); err != nil {
		return nil, err
	}
	if err := s.exportRoutines(ctx, workspaceID, bundle); err != nil {
		return nil, err
	}
	if err := s.exportProjects(ctx, workspaceID, bundle); err != nil {
		return nil, err
	}
	return bundle, nil
}

func (s *Service) exportAgents(ctx context.Context, wsID string, bundle *ConfigBundle) error {
	agents, err := s.repo.ListAgentInstances(ctx, wsID)
	if err != nil {
		return fmt.Errorf("list agents: %w", err)
	}
	// Build ID->name map for reports_to resolution.
	nameByID := make(map[string]string, len(agents))
	for _, a := range agents {
		nameByID[a.ID] = a.Name
	}
	for _, a := range agents {
		cfg := AgentConfig{
			Name:                  a.Name,
			Role:                  string(a.Role),
			Icon:                  a.Icon,
			ReportsTo:             nameByID[a.ReportsTo],
			BudgetMonthlyCents:    a.BudgetMonthlyCents,
			MaxConcurrentSessions: a.MaxConcurrentSessions,
			DesiredSkills:         a.DesiredSkills,
			ExecutorPreference:    a.ExecutorPreference,
		}
		bundle.Agents = append(bundle.Agents, cfg)
	}
	return nil
}

func (s *Service) exportSkills(ctx context.Context, wsID string, bundle *ConfigBundle) error {
	skills, err := s.repo.ListSkills(ctx, wsID)
	if err != nil {
		return fmt.Errorf("list skills: %w", err)
	}
	for _, sk := range skills {
		cfg := SkillConfig{
			Name:        sk.Name,
			Slug:        sk.Slug,
			Description: sk.Description,
			SourceType:  sk.SourceType,
			Content:     sk.Content,
		}
		bundle.Skills = append(bundle.Skills, cfg)
	}
	return nil
}

func (s *Service) exportRoutines(ctx context.Context, wsID string, bundle *ConfigBundle) error {
	routines, err := s.repo.ListRoutines(ctx, wsID)
	if err != nil {
		return fmt.Errorf("list routines: %w", err)
	}
	agents, err := s.repo.ListAgentInstances(ctx, wsID)
	if err != nil {
		return fmt.Errorf("list agents for routine resolution: %w", err)
	}
	nameByID := make(map[string]string, len(agents))
	for _, a := range agents {
		nameByID[a.ID] = a.Name
	}
	for _, r := range routines {
		cfg := RoutineConfig{
			Name:              r.Name,
			Description:       r.Description,
			TaskTemplate:      r.TaskTemplate,
			AssigneeName:      nameByID[r.AssigneeAgentInstanceID],
			ConcurrencyPolicy: r.ConcurrencyPolicy,
		}
		bundle.Routines = append(bundle.Routines, cfg)
	}
	return nil
}

func (s *Service) exportProjects(ctx context.Context, wsID string, bundle *ConfigBundle) error {
	projects, err := s.repo.ListProjects(ctx, wsID)
	if err != nil {
		return fmt.Errorf("list projects: %w", err)
	}
	agents, err := s.repo.ListAgentInstances(ctx, wsID)
	if err != nil {
		return fmt.Errorf("list agents for project resolution: %w", err)
	}
	nameByID := make(map[string]string, len(agents))
	for _, a := range agents {
		nameByID[a.ID] = a.Name
	}
	for _, p := range projects {
		cfg := ProjectConfig{
			Name:           p.Name,
			Description:    p.Description,
			Status:         string(p.Status),
			Color:          p.Color,
			BudgetCents:    p.BudgetCents,
			Repositories:   p.Repositories,
			ExecutorConfig: p.ExecutorConfig,
			LeadAgentName:  nameByID[p.LeadAgentInstanceID],
		}
		bundle.Projects = append(bundle.Projects, cfg)
	}
	return nil
}

// ExportZip exports the workspace configuration as a zip archive.
func (s *Service) ExportZip(ctx context.Context, workspaceID string) (io.Reader, error) {
	bundle, err := s.ExportBundle(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return bundleToZip(bundle)
}

// bundleToZip converts a ConfigBundle to a zip archive.
func bundleToZip(bundle *ConfigBundle) (io.Reader, error) {
	buf := new(bytes.Buffer)
	w := zip.NewWriter(buf)

	if err := writeYAMLFile(w, ".kandev/kandev.yml", bundle.Settings); err != nil {
		return nil, err
	}
	for _, a := range bundle.Agents {
		if err := writeYAMLFile(w, ".kandev/agents/"+a.Name+".yml", a); err != nil {
			return nil, err
		}
	}
	for _, sk := range bundle.Skills {
		if err := writeYAMLFile(w, ".kandev/skills/"+sk.Slug+".yml", sk); err != nil {
			return nil, err
		}
	}
	for _, r := range bundle.Routines {
		if err := writeYAMLFile(w, ".kandev/routines/"+r.Name+".yml", r); err != nil {
			return nil, err
		}
	}
	for _, p := range bundle.Projects {
		if err := writeYAMLFile(w, ".kandev/projects/"+p.Name+".yml", p); err != nil {
			return nil, err
		}
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("close zip: %w", err)
	}
	return buf, nil
}

func writeYAMLFile(w *zip.Writer, name string, data interface{}) error {
	f, err := w.Create(name)
	if err != nil {
		return fmt.Errorf("create zip entry %s: %w", name, err)
	}
	b, err := yaml.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal %s: %w", name, err)
	}
	_, err = f.Write(b)
	return err
}
