// Package configloader provides filesystem-first configuration loading for orchestrate workspaces.
package configloader

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/orchestrate/models"

	"gopkg.in/yaml.v3"
)

// WorkspaceSettings represents the kandev.yml workspace configuration.
type WorkspaceSettings struct {
	Name                string `yaml:"name"`
	Slug                string `yaml:"slug,omitempty"`
	Description         string `yaml:"description,omitempty"`
	ApprovalDefault     string `yaml:"approval_default,omitempty"`
	BudgetDefault       int    `yaml:"budget_default,omitempty"`
	TaskPrefix          string `yaml:"task_prefix,omitempty"`
	DefaultExecutor     string `yaml:"default_executor,omitempty"`
	DefaultAgentProfile string `yaml:"default_agent_profile,omitempty"`
}

// agentYAML is the YAML representation of an agent instance.
type agentYAML struct {
	ID                    string `yaml:"id,omitempty"`
	Name                  string `yaml:"name"`
	Role                  string `yaml:"role"`
	Icon                  string `yaml:"icon,omitempty"`
	ReportsTo             string `yaml:"reports_to,omitempty"`
	AgentProfileID        string `yaml:"agent_profile_id,omitempty"`
	Permissions           string `yaml:"permissions,omitempty"`
	BudgetMonthlyCents    int    `yaml:"budget_monthly_cents,omitempty"`
	MaxConcurrentSessions int    `yaml:"max_concurrent_sessions,omitempty"`
	DesiredSkills         string `yaml:"desired_skills,omitempty"`
	ExecutorPreference    string `yaml:"executor_preference,omitempty"`
}

// projectYAML is the YAML representation of a project.
type projectYAML struct {
	ID             string `yaml:"id,omitempty"`
	Name           string `yaml:"name"`
	Description    string `yaml:"description,omitempty"`
	Status         string `yaml:"status,omitempty"`
	Color          string `yaml:"color,omitempty"`
	BudgetCents    int    `yaml:"budget_cents,omitempty"`
	Repositories   string `yaml:"repositories,omitempty"`
	ExecutorConfig string `yaml:"executor_config,omitempty"`
	LeadAgentName  string `yaml:"lead_agent_name,omitempty"`
}

// routineYAML is the YAML representation of a routine.
type routineYAML struct {
	ID                string        `yaml:"id,omitempty"`
	Name              string        `yaml:"name"`
	Description       string        `yaml:"description,omitempty"`
	TaskTemplate      string        `yaml:"task_template,omitempty"`
	AssigneeName      string        `yaml:"assignee_name,omitempty"`
	Status            string        `yaml:"status,omitempty"`
	ConcurrencyPolicy string        `yaml:"concurrency_policy,omitempty"`
	Variables         string        `yaml:"variables,omitempty"`
	Triggers          []triggerYAML `yaml:"triggers,omitempty"`
}

// triggerYAML is the YAML representation of a routine trigger.
type triggerYAML struct {
	Kind           string `yaml:"kind"`
	CronExpression string `yaml:"cron_expression,omitempty"`
	Timezone       string `yaml:"timezone,omitempty"`
	Enabled        bool   `yaml:"enabled"`
}

// MarshalSettings serializes workspace settings to YAML.
func MarshalSettings(settings WorkspaceSettings) ([]byte, error) {
	return yaml.Marshal(settings)
}

// UnmarshalSettings deserializes workspace settings from YAML.
func UnmarshalSettings(data []byte) (WorkspaceSettings, error) {
	var settings WorkspaceSettings
	if err := yaml.Unmarshal(data, &settings); err != nil {
		return WorkspaceSettings{}, fmt.Errorf("unmarshal settings: %w", err)
	}
	return settings, nil
}

// MarshalAgent serializes an agent instance to YAML.
func MarshalAgent(agent *models.AgentInstance) ([]byte, error) {
	y := agentYAML{
		ID:                    agent.ID,
		Name:                  agent.Name,
		Role:                  string(agent.Role),
		Icon:                  agent.Icon,
		ReportsTo:             agent.ReportsTo,
		AgentProfileID:        agent.AgentProfileID,
		Permissions:           agent.Permissions,
		BudgetMonthlyCents:    agent.BudgetMonthlyCents,
		MaxConcurrentSessions: agent.MaxConcurrentSessions,
		DesiredSkills:         agent.DesiredSkills,
		ExecutorPreference:    agent.ExecutorPreference,
	}
	return yaml.Marshal(y)
}

// UnmarshalAgent deserializes an agent instance from YAML.
func UnmarshalAgent(data []byte, workspaceID string) (*models.AgentInstance, error) {
	var y agentYAML
	if err := yaml.Unmarshal(data, &y); err != nil {
		return nil, fmt.Errorf("unmarshal agent: %w", err)
	}
	now := time.Now()
	return &models.AgentInstance{
		ID:                    y.ID,
		WorkspaceID:           workspaceID,
		Name:                  y.Name,
		Role:                  models.AgentRole(y.Role),
		Icon:                  y.Icon,
		ReportsTo:             y.ReportsTo,
		AgentProfileID:        y.AgentProfileID,
		Permissions:           y.Permissions,
		Status:                models.AgentStatusIdle,
		BudgetMonthlyCents:    y.BudgetMonthlyCents,
		MaxConcurrentSessions: y.MaxConcurrentSessions,
		DesiredSkills:         y.DesiredSkills,
		ExecutorPreference:    y.ExecutorPreference,
		CreatedAt:             now,
		UpdatedAt:             now,
	}, nil
}

// MarshalProject serializes a project to YAML.
func MarshalProject(project *models.Project) ([]byte, error) {
	y := projectYAML{
		ID:             project.ID,
		Name:           project.Name,
		Description:    project.Description,
		Status:         string(project.Status),
		Color:          project.Color,
		BudgetCents:    project.BudgetCents,
		Repositories:   project.Repositories,
		ExecutorConfig: project.ExecutorConfig,
	}
	return yaml.Marshal(y)
}

// UnmarshalProject deserializes a project from YAML.
func UnmarshalProject(data []byte, workspaceID string) (*models.Project, error) {
	var y projectYAML
	if err := yaml.Unmarshal(data, &y); err != nil {
		return nil, fmt.Errorf("unmarshal project: %w", err)
	}
	status := models.ProjectStatus(y.Status)
	if status == "" {
		status = models.ProjectStatusActive
	}
	now := time.Now()
	return &models.Project{
		ID:             y.ID,
		WorkspaceID:    workspaceID,
		Name:           y.Name,
		Description:    y.Description,
		Status:         status,
		Color:          y.Color,
		BudgetCents:    y.BudgetCents,
		Repositories:   y.Repositories,
		ExecutorConfig: y.ExecutorConfig,
		CreatedAt:      now,
		UpdatedAt:      now,
	}, nil
}

// MarshalRoutine serializes a routine to YAML.
func MarshalRoutine(routine *models.Routine) ([]byte, error) {
	y := routineYAML{
		ID:                routine.ID,
		Name:              routine.Name,
		Description:       routine.Description,
		TaskTemplate:      routine.TaskTemplate,
		Status:            routine.Status,
		ConcurrencyPolicy: routine.ConcurrencyPolicy,
		Variables:         routine.Variables,
	}
	return yaml.Marshal(y)
}

// UnmarshalRoutine deserializes a routine from YAML.
func UnmarshalRoutine(data []byte, workspaceID string) (*models.Routine, error) {
	var y routineYAML
	if err := yaml.Unmarshal(data, &y); err != nil {
		return nil, fmt.Errorf("unmarshal routine: %w", err)
	}
	now := time.Now()
	return &models.Routine{
		ID:                y.ID,
		WorkspaceID:       workspaceID,
		Name:              y.Name,
		Description:       y.Description,
		TaskTemplate:      y.TaskTemplate,
		Status:            y.Status,
		ConcurrencyPolicy: y.ConcurrencyPolicy,
		Variables:         y.Variables,
		CreatedAt:         now,
		UpdatedAt:         now,
	}, nil
}

// isYAMLFile checks if a filename has a YAML extension.
func isYAMLFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return ext == ".yml" || ext == ".yaml"
}
