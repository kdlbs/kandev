package backendapp

import (
	"context"
	"database/sql"
	"errors"

	agentsettingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	workflowservice "github.com/kandev/kandev/internal/workflow/service"
)

type workflowImportProfileCatalog struct {
	repo settingsstore.Repository
}

func newWorkflowImportProfileCatalog(repos *Repositories) workflowImportProfileCatalog {
	return workflowImportProfileCatalog{repo: repos.AgentSettings}
}

func (c workflowImportProfileCatalog) ListEligibleProfiles(ctx context.Context) ([]workflowservice.ImportProfileCandidate, error) {
	if c.repo == nil {
		return nil, workflowservice.ErrImportProfileCatalogUnavailable
	}
	agents, err := c.repo.ListAgents(ctx)
	if err != nil {
		return nil, err
	}
	profiles := make([]workflowservice.ImportProfileCandidate, 0)
	for _, agent := range agents {
		candidates, listErr := c.repo.ListAgentProfiles(ctx, agent.ID)
		if listErr != nil {
			return nil, listErr
		}
		for _, profile := range candidates {
			if !isEligibleWorkflowImportProfile(profile) {
				continue
			}
			profiles = append(profiles, workflowImportProfileCandidate(profile))
		}
	}
	return profiles, nil
}

func (c workflowImportProfileCatalog) GetEligibleProfile(ctx context.Context, id string) (*workflowservice.ImportProfileCandidate, error) {
	if c.repo == nil {
		return nil, workflowservice.ErrImportProfileCatalogUnavailable
	}
	profile, err := c.repo.GetAgentProfile(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, workflowservice.ErrImportProfileNotFound
		}
		return nil, err
	}
	if !isEligibleWorkflowImportProfile(profile) {
		return nil, workflowservice.ErrImportProfileNotFound
	}
	candidate := workflowImportProfileCandidate(profile)
	return &candidate, nil
}

func isEligibleWorkflowImportProfile(profile *agentsettingsmodels.AgentProfile) bool {
	return profile != nil && profile.ID != "" && profile.Enabled && profile.WorkspaceID == ""
}

func workflowImportProfileCandidate(profile *agentsettingsmodels.AgentProfile) workflowservice.ImportProfileCandidate {
	return workflowservice.ImportProfileCandidate{
		ID:        profile.ID,
		Name:      profile.Name,
		AgentName: profile.AgentDisplayName,
		Model:     profile.Model,
		Mode:      profile.Mode,
		UpdatedAt: profile.UpdatedAt,
	}
}
