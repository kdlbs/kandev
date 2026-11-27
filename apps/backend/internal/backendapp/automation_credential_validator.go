package backendapp

import (
	"context"
	"errors"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/automation"
)

type lifecycleAutomationCredentialValidator struct {
	manager        automationCredentialLifecycle
	service        automationCredentialChoiceResolver
	prepareCreator func(context.Context, string, string, string) (context.Context, error)
}

type automationCredentialChoiceResolver interface {
	ResolveAutomationCredentialChoice(
		context.Context,
		*automation.Automation,
		string,
		string,
	) (automation.AutomationCredentialChoice, string, error)
}

type automationCredentialLifecycle interface {
	ValidateAutomationCredentialEnvironment(context.Context, agentruntime.AutomationCredentialRequest) error
	ValidateAutomationPersonalCredentialBindings(
		context.Context,
		string,
		string,
		[]agentruntime.PersonalCredentialBindingInput,
	) ([]agentruntime.ValidatedPersonalCredentialBinding, error)
}

func (v lifecycleAutomationCredentialValidator) credentialActorContext(
	ctx context.Context,
	a *automation.Automation,
) (context.Context, error) {
	if a == nil || v.prepareCreator == nil {
		return ctx, automation.ErrCredentialCreatorUnavailable
	}
	prepared, err := v.prepareCreator(ctx, a.WorkspaceID, a.CreatedByUserID, a.ExecutorProfileID)
	if err != nil {
		return ctx, automation.ErrCredentialCreatorUnavailable
	}
	return prepared, nil
}

func (v lifecycleAutomationCredentialValidator) ValidateAutomationCredentials(
	ctx context.Context,
	a *automation.Automation,
	repositoryIDs []string,
) error {
	if a == nil || v.manager == nil {
		return nil
	}
	ctx, err := v.credentialActorContext(ctx, a)
	if err != nil {
		return err
	}
	choices := automationCredentialChoicesForRepositories(a.CredentialChoices, repositoryIDs)
	bindings := make([]agentruntime.AutomationCredentialBinding, 0, len(choices))
	for _, choice := range choices {
		binding := agentruntime.AutomationCredentialBinding{
			Kind: string(choice.Kind), ProfileID: choice.ProfileID, RepositoryID: choice.RepositoryID,
			Key: choice.Key, SecretID: choice.SecretID, SourceSecretID: choice.SourceSecretID,
			SourceUpdatedAt: choice.SourceUpdatedAt, State: string(choice.State),
		}
		if choice.Kind == automation.CredentialChoiceWorkspace {
			if v.service == nil {
				return automation.ErrCredentialStorageUnavailable
			}
			current, value, err := v.service.ResolveAutomationCredentialChoice(ctx, a, choice.RepositoryID, choice.Key)
			if err != nil {
				return err
			}
			binding.SecretID = current.SecretID
			binding.SourceSecretID = current.SourceSecretID
			binding.SourceUpdatedAt = current.SourceUpdatedAt
			binding.State = string(current.State)
			binding.Value = value
		}
		bindings = append(bindings, binding)
	}
	err = v.manager.ValidateAutomationCredentialEnvironment(ctx, agentruntime.AutomationCredentialRequest{
		AutomationID: a.ID, WorkspaceID: a.WorkspaceID, AgentProfileID: a.AgentProfileID,
		ExecutorProfileID: a.ExecutorProfileID, RepositoryIDs: repositoryIDs, CredentialBindings: bindings,
	})
	if err == nil {
		return nil
	}
	var sourceErr *agentruntime.AutomationCredentialEnvironmentError
	if errors.As(err, &sourceErr) {
		return &automation.CredentialValidationError{Field: sourceErr.Field, Source: sourceErr.Source}
	}
	if errors.Is(err, agentruntime.ErrAutomationCredentialStorage) {
		return automation.ErrCredentialStorageUnavailable
	}
	return err
}

func automationCredentialChoicesForRepositories(
	choices []automation.CredentialChoiceProjection,
	repositoryIDs []string,
) []automation.CredentialChoiceProjection {
	selected := make([]automation.CredentialChoiceProjection, 0, len(choices))
	for _, choice := range choices {
		if choice.Kind == automation.CredentialChoiceWorkspace && !automationCredentialRepositorySelected(repositoryIDs, choice.RepositoryID) {
			continue
		}
		selected = append(selected, choice)
	}
	return selected
}

func automationCredentialRepositorySelected(repositoryIDs []string, repositoryID string) bool {
	if repositoryID == "" {
		return false
	}
	for _, selectedID := range repositoryIDs {
		if selectedID == repositoryID {
			return true
		}
	}
	return false
}

func (v lifecycleAutomationCredentialValidator) ValidatePersonalCredentialBindings(
	ctx context.Context,
	a *automation.Automation,
	inputs []automation.PersonalCredentialBindingInput,
) ([]automation.AutomationCredentialChoice, error) {
	if a == nil || v.manager == nil {
		return nil, automation.ErrCredentialChoiceUnavailable
	}
	ctx, err := v.credentialActorContext(ctx, a)
	if err != nil {
		return nil, err
	}
	bindings := make([]agentruntime.PersonalCredentialBindingInput, 0, len(inputs))
	for _, input := range inputs {
		bindings = append(bindings, agentruntime.PersonalCredentialBindingInput{
			ProfileID: input.ProfileID, Key: input.Key, SecretID: input.SecretID,
		})
	}
	validated, err := v.manager.ValidateAutomationPersonalCredentialBindings(ctx, a.AgentProfileID, a.ExecutorProfileID, bindings)
	if err != nil {
		var sourceErr *agentruntime.AutomationCredentialEnvironmentError
		if errors.As(err, &sourceErr) {
			return nil, &automation.CredentialValidationError{Field: sourceErr.Field, Source: sourceErr.Source}
		}
		if errors.Is(err, agentruntime.ErrAutomationCredentialStorage) {
			return nil, automation.ErrCredentialStorageUnavailable
		}
		return nil, err
	}
	choices := make([]automation.AutomationCredentialChoice, 0, len(validated))
	for _, binding := range validated {
		choices = append(choices, automation.AutomationCredentialChoice{
			Kind: automation.CredentialChoicePersonal, ProfileID: binding.ProfileID,
			Key: binding.Key, SecretID: binding.SecretID, SourceSecretID: binding.SourceSecretID,
			State: automation.CredentialChoiceSelected, SecretName: binding.SecretName,
		})
	}
	return choices, nil
}
