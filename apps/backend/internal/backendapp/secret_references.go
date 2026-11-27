package backendapp

import (
	"context"
	"errors"
	"fmt"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	automationpkg "github.com/kandev/kandev/internal/automation"
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

type secretAutomationReferenceReader interface {
	ListAutomations(context.Context, string) ([]*automationpkg.Automation, error)
	GetAutomation(context.Context, string) (*automationpkg.Automation, error)
}

type secretReferenceChecker struct {
	agents             secretAgentReferenceReader
	tasks              secretTaskReferenceReader
	automations        secretAutomationReferenceReader
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
	refs = append(refs, repositories...)
	automationRefs, err := c.automationReferences(ctx, id)
	if err != nil {
		return nil, err
	}
	return append(refs, automationRefs...), nil
}

func (c secretReferenceChecker) agentReferences(ctx context.Context, id string) ([]secrets.Reference, error) {
	agents, err := c.agents.ListAgents(ctx)
	if err != nil {
		return nil, fmt.Errorf("list agents: %w", err)
	}
	var refs []secrets.Reference
	workspaceAccess := make(map[string]error)
	for _, agent := range agents {
		profiles, err := c.agents.ListAgentProfiles(ctx, agent.ID)
		if err != nil {
			return nil, fmt.Errorf("list agent profiles: %w", err)
		}
		refs, err = c.appendAgentProfileReferences(ctx, refs, id, profiles, workspaceAccess)
		if err != nil {
			return nil, err
		}
	}
	return refs, nil
}

func (c secretReferenceChecker) appendAgentProfileReferences(
	ctx context.Context,
	refs []secrets.Reference,
	secretID string,
	profiles []*settingsmodels.AgentProfile,
	workspaceAccess map[string]error,
) ([]secrets.Reference, error) {
	for _, profile := range profiles {
		if profile == nil || !hasSecretReference(profile.EnvVars, secretID) {
			continue
		}
		redacted, err := c.workspaceReferenceRedacted(ctx, profile.WorkspaceID, workspaceAccess)
		if err != nil {
			return nil, err
		}
		if redacted {
			refs = appendRedactedEnvironmentReferences(refs, secretID, "agent_profile", profile.EnvVars)
			continue
		}
		refs = appendEnvironmentReferences(refs, secretID, "agent_profile", profile.ID, profile.Name, profile.EnvVars)
	}
	return refs, nil
}

func (c secretReferenceChecker) workspaceReferenceRedacted(
	ctx context.Context,
	workspaceID string,
	workspaceAccess map[string]error,
) (bool, error) {
	if workspaceID == "" {
		return false, nil
	}
	if c.authorizeWorkspace == nil {
		return false, errors.New("workspace reference authorization is unavailable")
	}
	accessErr, cached := workspaceAccess[workspaceID]
	if !cached {
		accessErr = c.authorizeWorkspace(ctx, workspaceID)
		workspaceAccess[workspaceID] = accessErr
	}
	if accessErr != nil && !errors.Is(accessErr, repoerrors.ErrWorkspaceNotFound) {
		return false, accessErr
	}
	return accessErr != nil, nil
}

func (c secretReferenceChecker) automationReferences(ctx context.Context, secretID string) ([]secrets.Reference, error) {
	if c.automations == nil {
		return nil, errors.New("automation credential reference reader is unavailable")
	}
	workspaces, err := c.tasks.ListWorkspaces(ctx)
	if err != nil {
		return nil, fmt.Errorf("list automation workspaces: %w", err)
	}
	workspaceAccess := make(map[string]error)
	var refs []secrets.Reference
	for _, workspace := range workspaces {
		if workspace == nil {
			continue
		}
		workspaceRefs, err := c.automationWorkspaceReferences(ctx, workspace.ID, secretID, workspaceAccess)
		if err != nil {
			return nil, err
		}
		refs = append(refs, workspaceRefs...)
	}
	return refs, nil
}

func (c secretReferenceChecker) automationWorkspaceReferences(
	ctx context.Context,
	workspaceID, secretID string,
	workspaceAccess map[string]error,
) ([]secrets.Reference, error) {
	automations, err := c.automations.ListAutomations(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list automation credential references: %w", err)
	}
	var refs []secrets.Reference
	for _, summary := range automations {
		if summary == nil || summary.ID == "" {
			continue
		}
		automationRefs, err := c.automationCredentialReferences(ctx, workspaceID, summary.ID, secretID, workspaceAccess)
		if err != nil {
			return nil, err
		}
		refs = append(refs, automationRefs...)
	}
	return refs, nil
}

func (c secretReferenceChecker) automationCredentialReferences(
	ctx context.Context,
	workspaceID, automationID, secretID string,
	workspaceAccess map[string]error,
) ([]secrets.Reference, error) {
	definition, err := c.automations.GetAutomation(ctx, automationID)
	if err != nil {
		return nil, fmt.Errorf("load automation credential references: %w", err)
	}
	if definition == nil {
		return nil, nil
	}
	var refs []secrets.Reference
	for _, choice := range definition.CredentialChoices {
		if choice.Kind != automationpkg.CredentialChoicePersonal || choice.SecretID != secretID {
			continue
		}
		redacted, err := c.workspaceReferenceRedacted(ctx, workspaceID, workspaceAccess)
		if err != nil {
			return nil, err
		}
		ref := secrets.Reference{Kind: "automation"}
		if !redacted {
			ref.ID, ref.Name, ref.Key = definition.ID, definition.Name, choice.Key
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

func appendRedactedEnvironmentReferences(refs []secrets.Reference, secretID, kind string, env []models.ProfileEnvVar) []secrets.Reference {
	for _, entry := range env {
		if entry.SecretID == secretID {
			refs = append(refs, secrets.Reference{Kind: kind})
		}
	}
	return refs
}

func hasSecretReference(env []models.ProfileEnvVar, secretID string) bool {
	for _, entry := range env {
		if entry.SecretID == secretID {
			return true
		}
	}
	return false
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
