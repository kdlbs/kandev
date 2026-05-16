package config

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/office/models"
)

// PreviewImport diffs a bundle against the current workspace state.
func (s *ConfigService) PreviewImport(
	ctx context.Context, workspaceID string, bundle *ConfigBundle,
) (*ImportPreview, error) {
	preview := &ImportPreview{}
	if err := s.previewAgents(ctx, workspaceID, bundle.Agents, &preview.Agents); err != nil {
		return nil, err
	}
	if err := s.previewSkills(ctx, workspaceID, bundle.Skills, &preview.Skills); err != nil {
		return nil, err
	}
	if err := s.previewRoutines(ctx, workspaceID, bundle.Routines, &preview.Routines); err != nil {
		return nil, err
	}
	if err := s.previewProjects(ctx, workspaceID, bundle.Projects, &preview.Projects); err != nil {
		return nil, err
	}
	return preview, nil
}

func (s *ConfigService) previewAgents(
	ctx context.Context, wsID string, incoming []AgentConfig, diff *ImportDiff,
) error {
	existing, err := s.repo.ListAgentInstances(ctx, wsID)
	if err != nil {
		return err
	}
	byName := make(map[string]bool, len(existing))
	for _, a := range existing {
		byName[a.Name] = true
	}
	for _, a := range incoming {
		if byName[a.Name] {
			diff.Updated = append(diff.Updated, a.Name)
		} else {
			diff.Created = append(diff.Created, a.Name)
		}
	}
	return nil
}

func (s *ConfigService) previewSkills(
	ctx context.Context, wsID string, incoming []SkillConfig, diff *ImportDiff,
) error {
	existing, err := s.repo.ListSkills(ctx, wsID)
	if err != nil {
		return err
	}
	bySlug := make(map[string]bool, len(existing))
	for _, sk := range existing {
		bySlug[sk.Slug] = true
	}
	for _, sk := range incoming {
		if bySlug[sk.Slug] {
			diff.Updated = append(diff.Updated, sk.Slug)
		} else {
			diff.Created = append(diff.Created, sk.Slug)
		}
	}
	return nil
}

func (s *ConfigService) previewRoutines(
	ctx context.Context, wsID string, incoming []RoutineConfig, diff *ImportDiff,
) error {
	existing, err := s.repo.ListRoutines(ctx, wsID)
	if err != nil {
		return err
	}
	byName := make(map[string]bool, len(existing))
	for _, r := range existing {
		byName[r.Name] = true
	}
	for _, r := range incoming {
		if byName[r.Name] {
			diff.Updated = append(diff.Updated, r.Name)
		} else {
			diff.Created = append(diff.Created, r.Name)
		}
	}
	return nil
}

func (s *ConfigService) previewProjects(
	ctx context.Context, wsID string, incoming []ProjectConfig, diff *ImportDiff,
) error {
	existing, err := s.repo.ListProjects(ctx, wsID)
	if err != nil {
		return err
	}
	byName := make(map[string]bool, len(existing))
	for _, p := range existing {
		byName[p.Name] = true
	}
	for _, p := range incoming {
		if byName[p.Name] {
			diff.Updated = append(diff.Updated, p.Name)
		} else {
			diff.Created = append(diff.Created, p.Name)
		}
	}
	return nil
}

// ApplyImport applies a config bundle to the workspace, deduplicating by name.
func (s *ConfigService) ApplyImport(
	ctx context.Context, workspaceID string, bundle *ConfigBundle,
) (*ImportResult, error) {
	result := &ImportResult{}

	if err := s.applyAgents(ctx, workspaceID, bundle.Agents, result); err != nil {
		return nil, fmt.Errorf("apply agents: %w", err)
	}
	if err := s.applySkills(ctx, workspaceID, bundle.Skills, result); err != nil {
		return nil, fmt.Errorf("apply skills: %w", err)
	}
	if err := s.applyRoutines(ctx, workspaceID, bundle.Routines, result); err != nil {
		return nil, fmt.Errorf("apply routines: %w", err)
	}
	if err := s.applyProjects(ctx, workspaceID, bundle.Projects, result); err != nil {
		return nil, fmt.Errorf("apply projects: %w", err)
	}

	s.activity.LogActivity(ctx, workspaceID, "user", "",
		"config_imported", "workspace", workspaceID,
		fmt.Sprintf("created=%d updated=%d", result.CreatedCount, result.UpdatedCount))

	return result, nil
}

func (s *ConfigService) applyAgents(
	ctx context.Context, wsID string, incoming []AgentConfig, result *ImportResult,
) error {
	existing, err := s.repo.ListAgentInstances(ctx, wsID)
	if err != nil {
		return err
	}
	byName := make(map[string]*models.AgentInstance, len(existing))
	for _, a := range existing {
		byName[a.Name] = a
	}
	for _, cfg := range incoming {
		if agent, ok := byName[cfg.Name]; ok {
			agent.Role = models.AgentRole(cfg.Role)
			agent.Icon = cfg.Icon
			agent.BudgetMonthlyCents = cfg.BudgetMonthlyCents
			agent.MaxConcurrentSessions = cfg.MaxConcurrentSessions
			agent.DesiredSkills = cfg.DesiredSkills
			agent.ExecutorPreference = cfg.ExecutorPreference
			if err := s.repo.UpdateAgentInstance(ctx, agent); err != nil {
				return err
			}
			result.UpdatedCount++
		} else {
			agent := &models.AgentInstance{
				WorkspaceID:           wsID,
				Name:                  cfg.Name,
				Role:                  models.AgentRole(cfg.Role),
				Icon:                  cfg.Icon,
				Status:                models.AgentStatusIdle,
				BudgetMonthlyCents:    cfg.BudgetMonthlyCents,
				MaxConcurrentSessions: cfg.MaxConcurrentSessions,
				DesiredSkills:         cfg.DesiredSkills,
				ExecutorPreference:    cfg.ExecutorPreference,
			}
			if err := s.repo.CreateAgentInstance(ctx, agent); err != nil {
				return err
			}
			result.CreatedCount++
		}
	}
	return nil
}

func (s *ConfigService) applySkills(
	ctx context.Context, wsID string, incoming []SkillConfig, result *ImportResult,
) error {
	existing, err := s.repo.ListSkills(ctx, wsID)
	if err != nil {
		return err
	}
	bySlug := make(map[string]*models.Skill, len(existing))
	for _, sk := range existing {
		bySlug[sk.Slug] = sk
	}
	for _, cfg := range incoming {
		if skill, ok := bySlug[cfg.Slug]; ok {
			skill.Name = cfg.Name
			skill.Description = cfg.Description
			skill.SourceType = cfg.SourceType
			skill.Content = cfg.Content
			if err := s.repo.UpdateSkill(ctx, skill); err != nil {
				return err
			}
			result.UpdatedCount++
		} else {
			skill := &models.Skill{
				WorkspaceID: wsID,
				Name:        cfg.Name,
				Slug:        cfg.Slug,
				Description: cfg.Description,
				SourceType:  cfg.SourceType,
				Content:     cfg.Content,
			}
			if err := s.repo.CreateSkill(ctx, skill); err != nil {
				return err
			}
			result.CreatedCount++
		}
	}
	return nil
}

func (s *ConfigService) applyRoutines(
	ctx context.Context, wsID string, incoming []RoutineConfig, result *ImportResult,
) error {
	existing, err := s.repo.ListRoutines(ctx, wsID)
	if err != nil {
		return err
	}
	byName := make(map[string]*models.Routine, len(existing))
	for _, r := range existing {
		byName[r.Name] = r
	}
	for _, cfg := range incoming {
		if routine, ok := byName[cfg.Name]; ok {
			routine.Description = cfg.Description
			routine.TaskTemplate = cfg.TaskTemplate
			routine.ConcurrencyPolicy = cfg.ConcurrencyPolicy
			if err := s.repo.UpdateRoutine(ctx, routine); err != nil {
				return err
			}
			result.UpdatedCount++
		} else {
			routine := &models.Routine{
				WorkspaceID:       wsID,
				Name:              cfg.Name,
				Description:       cfg.Description,
				TaskTemplate:      cfg.TaskTemplate,
				Status:            "active",
				ConcurrencyPolicy: cfg.ConcurrencyPolicy,
			}
			if err := s.repo.CreateRoutine(ctx, routine); err != nil {
				return err
			}
			result.CreatedCount++
		}
	}
	return nil
}

func (s *ConfigService) applyProjects(
	ctx context.Context, wsID string, incoming []ProjectConfig, result *ImportResult,
) error {
	existing, err := s.repo.ListProjects(ctx, wsID)
	if err != nil {
		return err
	}
	byName := make(map[string]*models.Project, len(existing))
	for _, p := range existing {
		byName[p.Name] = p
	}
	for _, cfg := range incoming {
		if project, ok := byName[cfg.Name]; ok {
			project.Description = cfg.Description
			project.Color = cfg.Color
			project.BudgetCents = cfg.BudgetCents
			project.Repositories = cfg.Repositories
			project.ExecutorConfig = cfg.ExecutorConfig
			if err := s.repo.UpdateProject(ctx, project); err != nil {
				return err
			}
			result.UpdatedCount++
		} else {
			project := &models.Project{
				WorkspaceID:    wsID,
				Name:           cfg.Name,
				Description:    cfg.Description,
				Status:         models.ProjectStatusActive,
				Color:          cfg.Color,
				BudgetCents:    cfg.BudgetCents,
				Repositories:   cfg.Repositories,
				ExecutorConfig: cfg.ExecutorConfig,
			}
			if err := s.repo.CreateProject(ctx, project); err != nil {
				return err
			}
			result.CreatedCount++
		}
	}
	return nil
}
