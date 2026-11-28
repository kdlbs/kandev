package backendapp

import (
	"context"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/automation"
)

type lifecycleAutomationCredentialBindingReader struct {
	service *automation.Service
}

func (r lifecycleAutomationCredentialBindingReader) ResolveAutomationCredentialBindings(
	ctx context.Context,
	taskID, automationID, runID string,
) ([]agentruntime.AutomationCredentialBinding, error) {
	if r.service == nil {
		return nil, automation.ErrCredentialChoiceUnavailable
	}
	a, run, choices, err := r.service.CredentialChoicesForRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	if a == nil || run == nil || a.ID != automationID || run.TaskID != taskID {
		return nil, automation.ErrCredentialChoiceUnavailable
	}
	if run.ProtectedCreatorUserID == "" {
		return nil, nil
	}
	if run.ProtectedCreatorUserID != a.CreatedByUserID || len(choices) == 0 {
		return nil, automation.ErrCredentialChoiceUnavailable
	}
	if err := automation.ValidateCredentialChoiceSettingsForRuntime(a, run.TriggerType); err != nil {
		return nil, err
	}
	bindings := make([]agentruntime.AutomationCredentialBinding, 0, len(choices))
	for _, choice := range choices {
		binding := agentruntime.AutomationCredentialBinding{
			Kind: string(choice.Kind), ProfileID: choice.ProfileID, RepositoryID: choice.RepositoryID,
			Key: choice.Key, SecretID: choice.SecretID, SourceSecretID: choice.SourceSecretID,
			SourceUpdatedAt: choice.SourceUpdatedAt, State: string(choice.State),
		}
		if choice.Kind == automation.CredentialChoiceWorkspace {
			value, err := r.service.ResolveAutomationCredentialBindingValue(
				ctx, taskID, automationID, runID, choice.RepositoryID, choice.Key,
			)
			if err != nil {
				return nil, err
			}
			binding.Value = value
		} else if choice.Kind != automation.CredentialChoicePersonal {
			return nil, automation.ErrCredentialChoiceUnavailable
		}
		bindings = append(bindings, binding)
	}
	return bindings, nil
}
