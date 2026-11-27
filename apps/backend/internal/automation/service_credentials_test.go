package automation

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type credentialValidatorFunc func(context.Context, *Automation, []string) error

func (f credentialValidatorFunc) ValidateAutomationCredentials(
	ctx context.Context,
	automation *Automation,
	repositoryIDs []string,
) error {
	return f(ctx, automation, repositoryIDs)
}

func TestAutomationCredentialSave(t *testing.T) {
	ctx := context.Background()
	credentialErr := errors.New("credential unavailable")

	t.Run("enabled create checks credentials before storing", func(t *testing.T) {
		svc := newTestService(t)
		var validated *Automation
		svc.SetCredentialValidator(credentialValidatorFunc(func(_ context.Context, a *Automation, _ []string) error {
			validated = a
			return credentialErr
		}))

		_, err := svc.CreateAutomation(ctx, &CreateAutomationRequest{
			WorkspaceID: "ws-credential-create", Name: "credential create", Enabled: boolPtr(true),
		})

		require.ErrorIs(t, err, credentialErr)
		require.NotNil(t, validated)
		automations, listErr := svc.store.ListAutomations(ctx, "ws-credential-create")
		require.NoError(t, listErr)
		require.Empty(t, automations)
	})

	t.Run("disabled create keeps an incomplete draft", func(t *testing.T) {
		svc := newTestService(t)
		calls := 0
		svc.SetCredentialValidator(credentialValidatorFunc(func(context.Context, *Automation, []string) error {
			calls++
			return credentialErr
		}))

		a, err := svc.CreateAutomation(ctx, &CreateAutomationRequest{
			WorkspaceID: "ws-credential-draft", Name: "credential draft", Enabled: boolPtr(false),
		})

		require.NoError(t, err)
		require.False(t, a.Enabled)
		require.Zero(t, calls)
	})

	t.Run("enabled executable update preserves saved definition on failure", func(t *testing.T) {
		svc := newTestService(t)
		a := &Automation{WorkspaceID: "ws-credential-update", Name: "before", Prompt: "old", Enabled: true}
		require.NoError(t, svc.store.CreateAutomation(ctx, a))
		svc.SetCredentialValidator(credentialValidatorFunc(func(context.Context, *Automation, []string) error {
			return credentialErr
		}))
		prompt := "new"

		_, err := svc.UpdateAutomation(ctx, a.ID, &UpdateAutomationRequest{Prompt: &prompt})

		require.ErrorIs(t, err, credentialErr)
		saved, readErr := svc.store.GetAutomation(ctx, a.ID)
		require.NoError(t, readErr)
		require.Equal(t, "old", saved.Prompt)
	})

	t.Run("enable checks credentials before changing state", func(t *testing.T) {
		svc := newTestService(t)
		a := &Automation{WorkspaceID: "ws-credential-enable", Name: "draft", Enabled: false}
		require.NoError(t, svc.store.CreateAutomation(ctx, a))
		svc.SetCredentialValidator(credentialValidatorFunc(func(context.Context, *Automation, []string) error {
			return credentialErr
		}))

		err := svc.EnableAutomation(ctx, a.ID)

		require.ErrorIs(t, err, credentialErr)
		saved, readErr := svc.store.GetAutomation(ctx, a.ID)
		require.NoError(t, readErr)
		require.False(t, saved.Enabled)
	})

	t.Run("enabled trigger changes check credentials before mutation", func(t *testing.T) {
		svc := newTestService(t)
		a := &Automation{WorkspaceID: "ws-credential-trigger", Name: "trigger", Enabled: true}
		require.NoError(t, svc.store.CreateAutomation(ctx, a))
		trigger := &AutomationTrigger{AutomationID: a.ID, Type: TriggerTypeManual, Enabled: false}
		require.NoError(t, svc.store.CreateTrigger(ctx, trigger))
		svc.SetCredentialValidator(credentialValidatorFunc(func(context.Context, *Automation, []string) error {
			return credentialErr
		}))
		enabled := true

		err := svc.UpdateTrigger(ctx, trigger.ID, &UpdateTriggerRequest{Enabled: &enabled})

		require.ErrorIs(t, err, credentialErr)
		saved, readErr := svc.store.GetTrigger(ctx, trigger.ID)
		require.NoError(t, readErr)
		require.False(t, saved.Enabled)
	})

	t.Run("credential choices reject an incompatible continuation update", func(t *testing.T) {
		svc, a := credentialChoiceService(t)
		input := []PersonalCredentialBindingInput{{ProfileID: a.AgentProfileID, Key: "SERVICE_KEY", SecretID: "creator-secret"}}
		svc.SetPersonalCredentialBindingValidator(personalBindingValidatorFunc(func(_ context.Context, _ *Automation, inputs []PersonalCredentialBindingInput) ([]AutomationCredentialChoice, error) {
			return []AutomationCredentialChoice{{
				Kind: CredentialChoicePersonal, ProfileID: inputs[0].ProfileID, Key: inputs[0].Key,
				SecretID: inputs[0].SecretID, State: CredentialChoiceSelected,
			}}, nil
		}))
		_, err := svc.UpdateAutomation(credentialIdentity("creator-1"), a.ID, &UpdateAutomationRequest{PersonalCredentialBindings: &input})
		require.NoError(t, err)

		policy := ContinuationPolicyReuseThread
		_, err = svc.UpdateAutomation(credentialIdentity("creator-1"), a.ID, &UpdateAutomationRequest{ContinuationPolicy: &policy})

		require.ErrorIs(t, err, ErrCredentialSettingsUnsupported)
	})

	t.Run("removing a personal binding from an enabled automation revalidates fallback", func(t *testing.T) {
		svc, a := credentialChoiceService(t)
		input := []PersonalCredentialBindingInput{{ProfileID: a.AgentProfileID, Key: "SERVICE_KEY", SecretID: "creator-secret"}}
		svc.SetPersonalCredentialBindingValidator(personalBindingValidatorFunc(func(_ context.Context, _ *Automation, inputs []PersonalCredentialBindingInput) ([]AutomationCredentialChoice, error) {
			return []AutomationCredentialChoice{{
				Kind: CredentialChoicePersonal, ProfileID: inputs[0].ProfileID, Key: inputs[0].Key,
				SecretID: inputs[0].SecretID, State: CredentialChoiceSelected,
			}}, nil
		}))
		_, err := svc.UpdateAutomation(credentialIdentity("creator-1"), a.ID, &UpdateAutomationRequest{PersonalCredentialBindings: &input})
		require.NoError(t, err)
		valid := credentialValidatorFunc(func(context.Context, *Automation, []string) error { return nil })
		svc.SetCredentialValidator(valid)
		enabled := true
		_, err = svc.UpdateAutomation(credentialIdentity("creator-1"), a.ID, &UpdateAutomationRequest{Enabled: &enabled})
		require.NoError(t, err)

		fallbackErr := &CredentialValidationError{Field: "SERVICE_KEY", Source: "Shared profile"}
		var validatedChoices int
		svc.SetCredentialValidator(credentialValidatorFunc(func(_ context.Context, candidate *Automation, _ []string) error {
			validatedChoices = len(candidate.CredentialChoices)
			return fallbackErr
		}))
		remove := []PersonalCredentialBindingInput{}
		_, err = svc.UpdateAutomation(credentialIdentity("creator-1"), a.ID, &UpdateAutomationRequest{PersonalCredentialBindings: &remove})

		require.ErrorIs(t, err, fallbackErr)
		require.Zero(t, validatedChoices)
	})
}

func boolPtr(value bool) *bool { return &value }
