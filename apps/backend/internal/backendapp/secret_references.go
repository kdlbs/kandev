package backendapp

import (
	"context"
	"errors"
	"fmt"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

type secretAgentReferenceReader interface {
	ListAgents(context.Context) ([]*settingsmodels.Agent, error)
	ListAgentProfiles(context.Context, string) ([]*settingsmodels.AgentProfile, error)
}

type secretTaskReferenceReader interface {
	ListAllExecutorProfiles(context.Context) ([]*models.ExecutorProfile, error)
	ListWorkspaces(context.Context) ([]*models.Workspace, error)
	ListRepositories(context.Context, string) ([]*models.Repository, error)
}

type secretReferenceChecker struct {
	agents             secretAgentReferenceReader
	tasks              secretTaskReferenceReader
	authorizeWorkspace func(context.Context, string) error
}

func (c secretReferenceChecker) list(ctx context.Context, id string) ([]secrets.Reference, error) {
	refs, err := c.agentReferences(ctx, id)
	if err != nil {
		return nil, err
	}
	profiles, err := c.tasks.ListAllExecutorProfiles(ctx)
	if err != nil {
		return nil, fmt.Errorf("list executor profiles: %w", err)
	}
	for _, profile := range profiles {
		refs = appendEnvironmentReferences(refs, id, "executor_profile", profile.ID, profile.Name, profile.EnvVars)
	}
	repositories, err := c.repositoryReferences(ctx, id)
	if err != nil {
		return nil, err
	}
	return append(refs, repositories...), nil
}

func (c secretReferenceChecker) agentReferences(ctx context.Context, id string) ([]secrets.Reference, error) {
	agents, err := c.agents.ListAgents(ctx)
	if err != nil {
		return nil, fmt.Errorf("list agents: %w", err)
	}
	var refs []secrets.Reference
	for _, agent := range agents {
		profiles, err := c.agents.ListAgentProfiles(ctx, agent.ID)
		if err != nil {
			return nil, fmt.Errorf("list agent profiles: %w", err)
		}
		for _, profile := range profiles {
			refs = appendEnvironmentReferences(refs, id, "agent_profile", profile.ID, profile.Name, profile.EnvVars)
		}
	}
	return refs, nil
}

func appendEnvironmentReferences(refs []secrets.Reference, secretID, kind, id, name string, env []models.ProfileEnvVar) []secrets.Reference {
	for _, entry := range env {
		if entry.SecretID == secretID {
			refs = append(refs, secrets.Reference{Kind: kind, ID: id, Name: name, Key: entry.Key})
		}
	}
	return refs
}

func (c secretReferenceChecker) repositoryReferences(ctx context.Context, id string) ([]secrets.Reference, error) {
	workspaces, err := c.tasks.ListWorkspaces(ctx)
	if err != nil {
		return nil, fmt.Errorf("list workspaces: %w", err)
	}
	var refs []secrets.Reference
	for _, workspace := range workspaces {
		if c.authorizeWorkspace == nil {
			return nil, errors.New("workspace reference authorization is unavailable")
		}
		accessErr := c.authorizeWorkspace(ctx, workspace.ID)
		if accessErr != nil && !errors.Is(accessErr, repoerrors.ErrWorkspaceNotFound) {
			return nil, accessErr
		}
		// Read every workspace so a hidden reference still prevents deletion.
		repositories, err := c.tasks.ListRepositories(ctx, workspace.ID)
		if err != nil {
			return nil, fmt.Errorf("list repository references: %w", err)
		}
		for _, repository := range repositories {
			for _, binding := range repository.SecretBindings {
				if binding.SecretID != id {
					continue
				}
				ref := secrets.Reference{Kind: "repository"}
				if accessErr == nil {
					ref.ID, ref.Name, ref.Key = repository.ID, repository.Name, binding.Key
				}
				refs = append(refs, ref)
			}
		}
	}
	return refs, nil
}
