package service

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/orchestrate/configloader"
	"github.com/kandev/kandev/internal/orchestrate/models"
)

// ListAgentsFromConfig returns agents from the filesystem ConfigLoader.
func (s *Service) ListAgentsFromConfig(_ context.Context, _ string) ([]*models.AgentInstance, error) {
	if s.cfgLoader == nil {
		return nil, fmt.Errorf("config loader not initialized")
	}
	return s.cfgLoader.GetAgents(defaultWorkspaceName), nil
}

// GetAgentFromConfig looks up an agent by ID or name from the ConfigLoader.
func (s *Service) GetAgentFromConfig(_ context.Context, id string) (*models.AgentInstance, error) {
	if s.cfgLoader == nil {
		return nil, fmt.Errorf("config loader not initialized")
	}
	for _, a := range s.cfgLoader.GetAgents(defaultWorkspaceName) {
		if a.ID == id || a.Name == id {
			return a, nil
		}
	}
	return nil, fmt.Errorf("agent not found: %s", id)
}

// ListSkillsFromConfig returns skills from the filesystem ConfigLoader.
func (s *Service) ListSkillsFromConfig(_ context.Context, _ string) ([]*models.Skill, error) {
	if s.cfgLoader == nil {
		return nil, fmt.Errorf("config loader not initialized")
	}
	return skillInfosToModels(s.cfgLoader.GetSkills(defaultWorkspaceName)), nil
}

// GetSkillFromConfig looks up a skill by ID or slug from the ConfigLoader.
func (s *Service) GetSkillFromConfig(_ context.Context, id string) (*models.Skill, error) {
	if s.cfgLoader == nil {
		return nil, fmt.Errorf("config loader not initialized")
	}
	for _, si := range s.cfgLoader.GetSkills(defaultWorkspaceName) {
		if si.ID == id || si.Slug == id {
			sk := si.Skill
			return &sk, nil
		}
	}
	return nil, fmt.Errorf("skill not found: %s", id)
}

// ListProjectsFromConfig returns projects from the filesystem ConfigLoader.
func (s *Service) ListProjectsFromConfig(_ context.Context, _ string) ([]*models.Project, error) {
	if s.cfgLoader == nil {
		return nil, fmt.Errorf("config loader not initialized")
	}
	return s.cfgLoader.GetProjects(defaultWorkspaceName), nil
}

// ListProjectsWithCountsFromConfig returns projects with task counts (counts still from DB).
func (s *Service) ListProjectsWithCountsFromConfig(
	ctx context.Context, _ string,
) ([]*models.ProjectWithCounts, error) {
	if s.cfgLoader == nil {
		return nil, fmt.Errorf("config loader not initialized")
	}
	projects := s.cfgLoader.GetProjects(defaultWorkspaceName)
	result := make([]*models.ProjectWithCounts, len(projects))
	for i, p := range projects {
		pc := &models.ProjectWithCounts{Project: *p}
		if counts, err := s.repo.GetTaskCounts(ctx, p.ID); err == nil && counts != nil {
			pc.TaskCounts = *counts
		}
		result[i] = pc
	}
	return result, nil
}

// GetProjectFromConfig looks up a project by ID or name from the ConfigLoader.
func (s *Service) GetProjectFromConfig(_ context.Context, id string) (*models.Project, error) {
	if s.cfgLoader == nil {
		return nil, fmt.Errorf("config loader not initialized")
	}
	for _, p := range s.cfgLoader.GetProjects(defaultWorkspaceName) {
		if p.ID == id || p.Name == id {
			return p, nil
		}
	}
	return nil, fmt.Errorf("project not found: %s", id)
}

// ListRoutinesFromConfig returns routines from the filesystem ConfigLoader.
func (s *Service) ListRoutinesFromConfig(_ context.Context, _ string) ([]*models.Routine, error) {
	if s.cfgLoader == nil {
		return nil, fmt.Errorf("config loader not initialized")
	}
	return s.cfgLoader.GetRoutines(defaultWorkspaceName), nil
}

// GetRoutineFromConfig looks up a routine by ID or name from the ConfigLoader.
func (s *Service) GetRoutineFromConfig(_ context.Context, id string) (*models.Routine, error) {
	if s.cfgLoader == nil {
		return nil, fmt.Errorf("config loader not initialized")
	}
	for _, r := range s.cfgLoader.GetRoutines(defaultWorkspaceName) {
		if r.ID == id || r.Name == id {
			return r, nil
		}
	}
	return nil, fmt.Errorf("routine not found: %s", id)
}

// skillInfosToModels extracts models.Skill from configloader.SkillInfo slices.
func skillInfosToModels(infos []*configloader.SkillInfo) []*models.Skill {
	result := make([]*models.Skill, len(infos))
	for i, si := range infos {
		sk := si.Skill
		result[i] = &sk
	}
	return result
}
