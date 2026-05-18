package service

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/office/models"
)

// ListAgentsFromConfig returns all agent instances for a workspace.
// An empty workspaceID returns rows across all workspaces.
func (s *Service) ListAgentsFromConfig(ctx context.Context, workspaceID string) ([]*models.AgentInstance, error) {
	return s.repo.ListAgentInstances(ctx, workspaceID)
}

// GetAgentFromConfig looks up an agent by ID or name.
func (s *Service) GetAgentFromConfig(ctx context.Context, idOrName string) (*models.AgentInstance, error) {
	if agent, err := s.repo.GetAgentInstance(ctx, idOrName); err == nil {
		return agent, nil
	}
	agent, err := s.repo.GetAgentInstanceByNameAny(ctx, idOrName)
	if err != nil {
		return nil, fmt.Errorf("agent not found: %s", idOrName)
	}
	return agent, nil
}

// ListSkillsFromConfig returns all skills for a workspace.
// An empty workspaceID returns rows across all workspaces.
func (s *Service) ListSkillsFromConfig(ctx context.Context, workspaceID string) ([]*models.Skill, error) {
	return s.repo.ListSkills(ctx, workspaceID)
}

// GetSkillFromConfig looks up a skill by ID or slug.
func (s *Service) GetSkillFromConfig(ctx context.Context, idOrSlug string) (*models.Skill, error) {
	if skill, err := s.repo.GetSkill(ctx, idOrSlug); err == nil {
		return skill, nil
	}
	skills, err := s.repo.ListSkills(ctx, "")
	if err != nil {
		return nil, err
	}
	for _, sk := range skills {
		if sk.Slug == idOrSlug {
			return sk, nil
		}
	}
	return nil, fmt.Errorf("skill not found: %s", idOrSlug)
}

// ListProjectsFromConfig returns all projects for a workspace.
// An empty workspaceID returns rows across all workspaces.
func (s *Service) ListProjectsFromConfig(ctx context.Context, workspaceID string) ([]*models.Project, error) {
	return s.repo.ListProjects(ctx, workspaceID)
}

// ListProjectsWithCountsFromConfig returns projects with task counts.
func (s *Service) ListProjectsWithCountsFromConfig(
	ctx context.Context, workspaceID string,
) ([]*models.ProjectWithCounts, error) {
	return s.repo.ListProjectsWithCounts(ctx, workspaceID)
}

// GetProjectFromConfig looks up a project by ID or name.
func (s *Service) GetProjectFromConfig(ctx context.Context, idOrName string) (*models.Project, error) {
	if project, err := s.repo.GetProject(ctx, idOrName); err == nil {
		return project, nil
	}
	projects, err := s.repo.ListProjects(ctx, "")
	if err != nil {
		return nil, err
	}
	for _, p := range projects {
		if p.Name == idOrName {
			return p, nil
		}
	}
	return nil, fmt.Errorf("project not found: %s", idOrName)
}

// ListRoutinesFromConfig returns all routines for a workspace.
// An empty workspaceID returns rows across all workspaces.
func (s *Service) ListRoutinesFromConfig(ctx context.Context, workspaceID string) ([]*models.Routine, error) {
	return s.repo.ListRoutines(ctx, workspaceID)
}

// GetRoutineFromConfig looks up a routine by ID or name.
func (s *Service) GetRoutineFromConfig(ctx context.Context, idOrName string) (*models.Routine, error) {
	if routine, err := s.repo.GetRoutine(ctx, idOrName); err == nil {
		return routine, nil
	}
	routines, err := s.repo.ListRoutines(ctx, "")
	if err != nil {
		return nil, err
	}
	for _, r := range routines {
		if r.Name == idOrName {
			return r, nil
		}
	}
	return nil, fmt.Errorf("routine not found: %s", idOrName)
}

// defaultRecoveryLookbackHours is used when no explicit value is configured.
const defaultRecoveryLookbackHours = 24

// GetRecoveryLookbackHours returns the recovery_lookback_hours setting for the
// default workspace. Falls back to defaultRecoveryLookbackHours if not set.
func (s *Service) GetRecoveryLookbackHours() int {
	ws, err := s.cfgLoader.GetWorkspace(defaultWorkspaceName)
	if err != nil || ws == nil {
		return defaultRecoveryLookbackHours
	}
	if ws.Settings.RecoveryLookbackHours > 0 {
		return ws.Settings.RecoveryLookbackHours
	}
	return defaultRecoveryLookbackHours
}
