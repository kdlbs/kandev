package lifecycle

import (
	"context"
	"fmt"
	"strings"
	"time"

	runtimeenv "github.com/kandev/kandev/internal/agent/runtime/environment"
	"github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/secrets"
)

const (
	automationCredentialBindingWorkspace = "workspace"
	automationCredentialBindingSelected  = "selected"
)

type AutomationCredentialBinding struct {
	Kind            string
	ProfileID       string
	RepositoryID    string
	Key             string
	SecretID        string
	SourceSecretID  string
	SourceUpdatedAt time.Time
	State           string
	Value           string
}

type AutomationCredentialBindingReader interface {
	ResolveAutomationCredentialBindings(context.Context, string, string, string) ([]AutomationCredentialBinding, error)
}

func (m *Manager) applyAutomationRunCredentialBindings(
	ctx context.Context,
	req *LaunchRequest,
	definitions []runtimeenv.Definition,
) ([]runtimeenv.Definition, error) {
	if req == nil || req.Metadata == nil {
		return definitions, nil
	}
	automationID, _ := req.Metadata["automation_id"].(string)
	runID, _ := req.Metadata["automation_run_id"].(string)
	if automationID == "" || runID == "" {
		return definitions, nil
	}
	if m.automationCredentialBindingReader == nil {
		return nil, ErrAutomationCredentialStorage
	}
	bindings, err := m.automationCredentialBindingReader.ResolveAutomationCredentialBindings(ctx, req.TaskID, automationID, runID)
	if err != nil {
		return nil, err
	}
	agentProfileID, _ := req.Metadata["agent_profile_id"].(string)
	executorProfileID, _ := req.Metadata["executor_profile_id"].(string)
	return applyAutomationCredentialBindings(agentProfileID, executorProfileID, bindings, definitions)
}

type PersonalCredentialBindingInput struct {
	ProfileID string
	Key       string
	SecretID  string
}

type ValidatedPersonalCredentialBinding struct {
	ProfileID      string
	Key            string
	SecretID       string
	SourceSecretID string
	SecretName     string
}

func applyAutomationCredentialBindings(
	agentProfileID, executorProfileID string,
	bindings []AutomationCredentialBinding,
	definitions []runtimeenv.Definition,
) ([]runtimeenv.Definition, error) {
	result := append([]runtimeenv.Definition(nil), definitions...)
	for _, binding := range bindings {
		switch binding.Kind {
		case "personal":
			if err := applyPersonalCredentialBinding(agentProfileID, executorProfileID, binding, result); err != nil {
				return nil, err
			}
		case automationCredentialBindingWorkspace:
			if err := applyWorkspaceCredentialBinding(binding, result); err != nil {
				return nil, err
			}
		default:
			return nil, &AutomationCredentialEnvironmentError{Field: binding.Key, Source: "Credential choice", err: ErrAutomationCredentialUnavailable}
		}
	}
	return result, nil
}

func applyPersonalCredentialBinding(agentProfileID, executorProfileID string, binding AutomationCredentialBinding, result []runtimeenv.Definition) error {
	origin := ""
	switch binding.ProfileID {
	case agentProfileID:
		origin = runtimeenv.OriginAgentProfile
	case executorProfileID:
		origin = runtimeenv.OriginExecutorProfile
	}
	if binding.State != automationCredentialBindingSelected || origin == "" || binding.SecretID == "" || binding.SourceSecretID == "" {
		return &AutomationCredentialEnvironmentError{Field: binding.Key, Source: "Selected profile", err: ErrAutomationCredentialUnavailable}
	}
	matched := false
	for index := range result {
		definition := &result[index]
		if definition.Origin != origin || definition.Key != binding.Key {
			continue
		}
		if definition.SecretID != binding.SourceSecretID {
			return &AutomationCredentialEnvironmentError{Field: binding.Key, Source: origin, err: ErrAutomationCredentialUnavailable}
		}
		definition.SecretID = binding.SecretID
		matched = true
	}
	if !matched {
		return &AutomationCredentialEnvironmentError{Field: binding.Key, Source: origin, err: ErrAutomationCredentialUnavailable}
	}
	return nil
}

func applyWorkspaceCredentialBinding(binding AutomationCredentialBinding, result []runtimeenv.Definition) error {
	if binding.State != "granted" || binding.SecretID == "" || binding.SecretID != binding.SourceSecretID || binding.Value == "" {
		return &AutomationCredentialEnvironmentError{Field: binding.Key, Source: "Selected repository", err: ErrAutomationCredentialUnavailable}
	}
	matched := false
	for index := range result {
		definition := &result[index]
		if !runtimeenv.IsRepositoryOrigin(definition.Origin) || definition.RepositoryID != binding.RepositoryID ||
			definition.Key != binding.Key || definition.SecretID != binding.SourceSecretID ||
			!definition.BindingUpdatedAt.Equal(binding.SourceUpdatedAt) {
			continue
		}
		definition.Literal = binding.Value
		definition.SecretID = ""
		definition.WorkspaceID = ""
		matched = true
	}
	if !matched {
		return &AutomationCredentialEnvironmentError{Field: binding.Key, Source: "Selected repository", err: ErrAutomationCredentialUnavailable}
	}
	return nil
}

func (m *Manager) ValidateAutomationPersonalCredentialBindings(
	ctx context.Context,
	agentProfileID, executorProfileID string,
	inputs []PersonalCredentialBindingInput,
) ([]ValidatedPersonalCredentialBinding, error) {
	if len(inputs) == 0 {
		return []ValidatedPersonalCredentialBinding{}, nil
	}
	if m.secretStore == nil {
		return nil, ErrAutomationCredentialStorage
	}
	seen := make(map[string]struct{}, len(inputs))
	validated := make([]ValidatedPersonalCredentialBinding, 0, len(inputs))
	for _, input := range inputs {
		key := strings.TrimSpace(input.Key)
		if key == "" || input.SecretID == "" {
			return nil, ErrAutomationCredentialUnavailable
		}
		identity := input.ProfileID + "\x00" + key
		if _, duplicate := seen[identity]; duplicate {
			return nil, fmt.Errorf("duplicate personal credential field %s", key)
		}
		seen[identity] = struct{}{}
		available, origin, err := m.personalCredentialProfileFields(ctx, input.ProfileID, agentProfileID, executorProfileID)
		if err != nil {
			return nil, err
		}
		source, ok := available[key]
		if !ok {
			return nil, &AutomationCredentialEnvironmentError{Field: key, Source: origin, err: ErrAutomationCredentialUnavailable}
		}
		if err := secrets.ValidateGlobalReference(ctx, m.secretStore, input.SecretID); err != nil {
			return nil, automationCredentialSecretError(key, origin, err)
		}
		secret, err := m.secretStore.Get(ctx, input.SecretID)
		if err != nil || secret == nil {
			return nil, automationCredentialSecretError(key, origin, err)
		}
		validated = append(validated, ValidatedPersonalCredentialBinding{
			ProfileID: input.ProfileID, Key: key, SecretID: input.SecretID,
			SourceSecretID: source.SecretID, SecretName: secret.Name,
		})
	}
	return validated, nil
}

func (m *Manager) personalCredentialProfileFields(
	ctx context.Context,
	profileID, agentProfileID, executorProfileID string,
) (map[string]models.ProfileEnvVar, string, error) {
	var (
		fields []models.ProfileEnvVar
		origin string
	)
	switch profileID {
	case agentProfileID:
		origin = runtimeenv.OriginAgentProfile
		if profileID == "" || m.agentProfileReader == nil {
			return nil, origin, ErrAutomationCredentialStorage
		}
		profile, err := m.agentProfileReader.GetAgentProfile(ctx, profileID)
		if err != nil {
			return nil, origin, ErrAutomationCredentialStorage
		}
		if profile == nil {
			return nil, origin, &AutomationCredentialEnvironmentError{Field: "profile secret", Source: origin, err: ErrAutomationCredentialUnavailable}
		}
		fields = profile.EnvVars
	case executorProfileID:
		origin = runtimeenv.OriginExecutorProfile
		if profileID == "" || m.executorProfileReader == nil {
			return nil, origin, ErrAutomationCredentialStorage
		}
		profile, err := m.executorProfileReader.GetExecutorProfile(ctx, profileID)
		if err != nil {
			return nil, origin, ErrAutomationCredentialStorage
		}
		if profile == nil {
			return nil, origin, &AutomationCredentialEnvironmentError{Field: "profile secret", Source: origin, err: ErrAutomationCredentialUnavailable}
		}
		fields = profile.EnvVars
	default:
		return nil, "Selected profile", ErrAutomationCredentialUnavailable
	}
	available := make(map[string]models.ProfileEnvVar, len(fields))
	for _, field := range fields {
		if key := strings.TrimSpace(field.Key); key != "" && field.SecretID != "" {
			available[key] = field
		}
	}
	return available, origin, nil
}
