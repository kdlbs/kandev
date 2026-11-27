package backendapp

import (
	"context"
	"errors"
	"testing"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/automation"
	"github.com/stretchr/testify/require"
)

type automationCredentialLifecycleFake struct {
	environmentErr error
	personalErr    error
	environment    agentruntime.AutomationCredentialRequest
	personalAgent  string
	personalExec   string
	personalInputs []agentruntime.PersonalCredentialBindingInput
	validated      []agentruntime.ValidatedPersonalCredentialBinding
}

func (f *automationCredentialLifecycleFake) ValidateAutomationCredentialEnvironment(
	_ context.Context,
	request agentruntime.AutomationCredentialRequest,
) error {
	f.environment = request
	return f.environmentErr
}

func (f *automationCredentialLifecycleFake) ValidateAutomationPersonalCredentialBindings(
	_ context.Context,
	agentProfileID, executorProfileID string,
	inputs []agentruntime.PersonalCredentialBindingInput,
) ([]agentruntime.ValidatedPersonalCredentialBinding, error) {
	f.personalAgent = agentProfileID
	f.personalExec = executorProfileID
	f.personalInputs = inputs
	return f.validated, f.personalErr
}

type automationCredentialChoiceResolverFake struct {
	choice automation.AutomationCredentialChoice
	value  string
	err    error
}

func (f automationCredentialChoiceResolverFake) ResolveAutomationCredentialChoice(
	context.Context,
	*automation.Automation,
	string,
	string,
) (automation.AutomationCredentialChoice, string, error) {
	return f.choice, f.value, f.err
}

func TestCredentialValidatorRequiresCreatorPreparation(t *testing.T) {
	a := &automation.Automation{WorkspaceID: "workspace", CreatedByUserID: "creator"}
	manager := &automationCredentialLifecycleFake{}
	validator := lifecycleAutomationCredentialValidator{manager: manager}
	require.ErrorIs(t, validator.ValidateAutomationCredentials(context.Background(), a, nil), automation.ErrCredentialCreatorUnavailable)
	_, err := validator.ValidatePersonalCredentialBindings(context.Background(), a, nil)
	require.ErrorIs(t, err, automation.ErrCredentialCreatorUnavailable)

	validator.prepareCreator = func(context.Context, string, string, string) (context.Context, error) {
		return nil, errors.New("creator is inactive")
	}
	require.ErrorIs(t, validator.ValidateAutomationCredentials(context.Background(), a, nil), automation.ErrCredentialCreatorUnavailable)
}

func TestCredentialValidatorResolvesAndTranslatesWorkspaceBindings(t *testing.T) {
	a := &automation.Automation{
		ID: "automation", WorkspaceID: "workspace", AgentProfileID: "agent", ExecutorProfileID: "executor",
		CredentialChoices: []automation.CredentialChoiceProjection{
			{Kind: automation.CredentialChoiceWorkspace, RepositoryID: "removed", Key: "OLD_KEY"},
			{Kind: automation.CredentialChoiceWorkspace, RepositoryID: "repo", Key: "SERVICE_KEY", SecretID: "selected-secret"},
			{Kind: automation.CredentialChoicePersonal, ProfileID: "agent", Key: "PERSONAL_KEY", SecretID: "personal-secret"},
		},
	}
	manager := &automationCredentialLifecycleFake{}
	validator := lifecycleAutomationCredentialValidator{
		manager: manager,
		prepareCreator: func(ctx context.Context, workspace, creator, profile string) (context.Context, error) {
			require.Equal(t, "workspace", workspace)
			require.Equal(t, "creator", creator)
			require.Equal(t, "executor", profile)
			return ctx, nil
		},
		service: automationCredentialChoiceResolverFake{
			choice: automation.AutomationCredentialChoice{SecretID: "current-secret", SourceSecretID: "source-secret", State: automation.CredentialChoiceGranted},
			value:  "resolved-value",
		},
	}
	a.CreatedByUserID = "creator"
	require.NoError(t, validator.ValidateAutomationCredentials(context.Background(), a, []string{"repo"}))
	require.Len(t, manager.environment.CredentialBindings, 2)
	require.Equal(t, "current-secret", manager.environment.CredentialBindings[0].SecretID)
	require.Equal(t, "resolved-value", manager.environment.CredentialBindings[0].Value)
	require.Equal(t, "personal-secret", manager.environment.CredentialBindings[1].SecretID)
	require.Equal(t, []string{"repo"}, manager.environment.RepositoryIDs)

	manager.environmentErr = &agentruntime.AutomationCredentialEnvironmentError{Field: "SERVICE_KEY", Source: "profile"}
	err := validator.ValidateAutomationCredentials(context.Background(), a, []string{"repo"})
	require.ErrorIs(t, err, automation.ErrCredentialsUnavailable)
	require.Equal(t, map[string]interface{}{
		"credential_error_code": automation.ErrCredentialsUnavailable.Error(),
		"field":                 "SERVICE_KEY", "source": "profile",
	}, automation.CredentialValidationDetails(err))
	manager.environmentErr = agentruntime.ErrAutomationCredentialStorage
	require.ErrorIs(t, validator.ValidateAutomationCredentials(context.Background(), a, []string{"repo"}), automation.ErrCredentialStorageUnavailable)
}

func TestCredentialValidatorFailsClosedWithoutWorkspaceResolver(t *testing.T) {
	a := &automation.Automation{ID: "automation", WorkspaceID: "workspace", CreatedByUserID: "creator"}
	a.CredentialChoices = []automation.CredentialChoiceProjection{{
		Kind: automation.CredentialChoiceWorkspace, RepositoryID: "repo", Key: "SERVICE_KEY",
	}}
	validator := lifecycleAutomationCredentialValidator{
		manager:        &automationCredentialLifecycleFake{},
		prepareCreator: func(ctx context.Context, _, _, _ string) (context.Context, error) { return ctx, nil },
	}
	require.ErrorIs(t, validator.ValidateAutomationCredentials(context.Background(), a, []string{"repo"}), automation.ErrCredentialStorageUnavailable)

	resolverErr := errors.New("credential store unavailable")
	validator.service = automationCredentialChoiceResolverFake{err: resolverErr}
	require.ErrorIs(t, validator.ValidateAutomationCredentials(context.Background(), a, []string{"repo"}), resolverErr)
}

func TestCredentialValidatorConvertsPersonalBindingsInBothDirections(t *testing.T) {
	manager := &automationCredentialLifecycleFake{
		validated: []agentruntime.ValidatedPersonalCredentialBinding{{
			ProfileID: "agent", Key: "SERVICE_KEY", SecretID: "personal-secret",
			SourceSecretID: "source-secret", SecretName: "Creator token",
		}},
	}
	validator := lifecycleAutomationCredentialValidator{
		manager:        manager,
		prepareCreator: func(ctx context.Context, _, _, _ string) (context.Context, error) { return ctx, nil },
	}
	a := &automation.Automation{AgentProfileID: "agent", ExecutorProfileID: "executor", CreatedByUserID: "creator"}
	choices, err := validator.ValidatePersonalCredentialBindings(context.Background(), a, []automation.PersonalCredentialBindingInput{{
		ProfileID: "agent", Key: "SERVICE_KEY", SecretID: "personal-secret",
	}})
	require.NoError(t, err)
	require.Equal(t, "agent", manager.personalAgent)
	require.Equal(t, "executor", manager.personalExec)
	require.Equal(t, []agentruntime.PersonalCredentialBindingInput{{ProfileID: "agent", Key: "SERVICE_KEY", SecretID: "personal-secret"}}, manager.personalInputs)
	require.Equal(t, []automation.AutomationCredentialChoice{{
		Kind: automation.CredentialChoicePersonal, ProfileID: "agent", Key: "SERVICE_KEY",
		SecretID: "personal-secret", SourceSecretID: "source-secret", SecretName: "Creator token",
		State: automation.CredentialChoiceSelected,
	}}, choices)

	manager.personalErr = agentruntime.ErrAutomationCredentialStorage
	_, err = validator.ValidatePersonalCredentialBindings(context.Background(), a, nil)
	require.ErrorIs(t, err, automation.ErrCredentialStorageUnavailable)
	manager.personalErr = &agentruntime.AutomationCredentialEnvironmentError{Field: "SERVICE_KEY", Source: "executor"}
	_, err = validator.ValidatePersonalCredentialBindings(context.Background(), a, nil)
	require.ErrorIs(t, err, automation.ErrCredentialsUnavailable)
}
