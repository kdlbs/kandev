package backendapp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/agent/remoteauth"
	agentsettingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/cursorcloud"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/models"
)

type taskAgentExecutorCompatibilityValidator struct {
	profiles           agentProfileCompatibilityReader
	agentRegistry      *registry.Registry
	dynamicResolver    profileExecutionValidator
	secretStore        secrets.SecretStore
	cursorCloudEnabled bool
	cursorCloudModels  cursorCloudModelValidator
}

type profileExecutionValidator interface {
	ValidateProfile(context.Context, string) error
}

type cursorCloudModelValidator interface {
	ValidateModel(context.Context, string, string) error
}

type cursorCloudModelValidatorFunc func(context.Context, string, string) error

func (f cursorCloudModelValidatorFunc) ValidateModel(ctx context.Context, secretID, model string) error {
	return f(ctx, secretID, model)
}

type agentProfileCompatibilityReader interface {
	GetAgent(ctx context.Context, id string) (*agentsettingsmodels.Agent, error)
}

func (v taskAgentExecutorCompatibilityValidator) validateAgentFamily(
	ctx context.Context,
	profile *agentsettingsmodels.AgentProfile,
) (agents.Agent, error) {
	if v.profiles == nil || v.agentRegistry == nil {
		return nil, fmt.Errorf("agent compatibility registry is unavailable")
	}
	settingsAgent, err := v.profiles.GetAgent(ctx, profile.AgentID)
	if err != nil {
		return nil, fmt.Errorf("load agent family: %w", err)
	}
	if settingsAgent == nil {
		return nil, fmt.Errorf("agent family is unavailable")
	}
	if settingsAgent.Name == agents.DynamicAgentID {
		if v.dynamicResolver == nil {
			return nil, fmt.Errorf("dynamic agent routing validator is unavailable")
		}
		return nil, nil
	}
	registeredAgent, ok := v.agentRegistry.Get(settingsAgent.Name)
	if !ok || registeredAgent == nil || !registeredAgent.Enabled() {
		return nil, fmt.Errorf("agent family %q cannot execute", settingsAgent.Name)
	}
	if _, managed := registeredAgent.(agents.ManagedRemoteAgent); !managed && agents.IsVirtualAgent(registeredAgent) {
		return nil, fmt.Errorf("agent family %q cannot execute", settingsAgent.Name)
	}
	return registeredAgent, nil
}

func (v taskAgentExecutorCompatibilityValidator) ValidateAgentProfileForExecutor(
	ctx context.Context,
	profile *agentsettingsmodels.AgentProfile,
	executor *models.Executor,
	executorProfile *models.ExecutorProfile,
) error {
	if profile == nil {
		return fmt.Errorf("agent profile is unavailable")
	}
	if executor == nil {
		return fmt.Errorf("executor is unavailable")
	}
	if v.dynamicResolver != nil {
		if err := v.dynamicResolver.ValidateProfile(ctx, profile.ID); err != nil {
			return err
		}
	}
	registeredAgent, err := v.validateAgentFamily(ctx, profile)
	if err != nil {
		return err
	}
	if registeredAgent == nil {
		// Dynamic profiles have already passed the runtime validator and resolve
		// to a concrete candidate at launch.
		return nil
	}
	if _, managed := registeredAgent.(agents.ManagedRemoteAgent); managed {
		return v.validateManagedAgentProfileForExecutor(ctx, registeredAgent, profile, executor, executorProfile)
	}
	if executor.Type == models.ExecutorTypeCursorCloud {
		return fmt.Errorf("executor type cursor_cloud only supports the cursor_cloud agent")
	}
	if !models.IsRemoteExecutorType(executor.Type) {
		return nil
	}
	if executorProfile == nil {
		return fmt.Errorf("remote executor profile is required")
	}
	return validateRemoteAgentCredentials(registeredAgent, executorProfile)
}

func (v taskAgentExecutorCompatibilityValidator) validateManagedAgentProfileForExecutor(
	ctx context.Context,
	registeredAgent agents.Agent,
	profile *agentsettingsmodels.AgentProfile,
	executor *models.Executor,
	executorProfile *models.ExecutorProfile,
) error {
	if registeredAgent.ID() != agents.CursorCloudAgentID || !v.cursorCloudEnabled {
		return fmt.Errorf("managed agent runtime is unavailable")
	}
	if executor.Type != models.ExecutorTypeCursorCloud {
		return fmt.Errorf("executor type cursor_cloud only supports its managed agent")
	}
	if executorProfile == nil {
		return fmt.Errorf("executor profile is required for cursor_cloud")
	}
	secretID := strings.TrimSpace(executorProfile.Config[cursorcloud.ExecutorConfigSecretID])
	callbackURL := strings.TrimSpace(executorProfile.Config[cursorcloud.ExecutorConfigCallbackURL])
	if secretID == "" || v.secretStore == nil ||
		secrets.ValidateGlobalReference(ctx, v.secretStore, secretID) != nil {
		return fmt.Errorf("cursor cloud API key secret reference is unavailable")
	}
	if cursorcloud.ValidateCallbackURL(callbackURL) != nil {
		return fmt.Errorf("cursor cloud callback URL is invalid")
	}
	modelValidator := v.cursorCloudModels
	if modelValidator == nil {
		modelValidator = cursorCloudModelValidatorFunc(func(ctx context.Context, secretID, model string) error {
			return validateCursorCloudModel(ctx, v.secretStore, secretID, model)
		})
	}
	if err := modelValidator.ValidateModel(ctx, secretID, profile.Model); err != nil {
		return fmt.Errorf("cursor cloud model is unavailable")
	}
	return nil
}

func validateCursorCloudModel(ctx context.Context, store secrets.SecretStore, secretID, model string) error {
	if err := secrets.ValidateGlobalReference(ctx, store, secretID); err != nil {
		return fmt.Errorf("cursor cloud API key is unavailable")
	}
	apiKey, err := store.Reveal(ctx, secretID)
	if err != nil || strings.TrimSpace(apiKey) == "" {
		return fmt.Errorf("cursor cloud API key is unavailable")
	}
	client, err := cursorcloud.NewRuntimeClient(apiKey, true)
	if err != nil {
		return fmt.Errorf("cursor cloud is unavailable")
	}
	return cursorcloud.ValidateModel(ctx, client, model)
}

func validateRemoteAgentCredentials(agent agents.Agent, executorProfile *models.ExecutorProfile) error {
	catalog := remoteauth.BuildCatalog([]agents.Agent{agent})
	var spec *remoteauth.Spec
	for i := range catalog.Specs {
		if catalog.Specs[i].ID == agent.ID() {
			spec = &catalog.Specs[i]
			break
		}
	}
	if spec == nil {
		return fmt.Errorf("agent family %q has no remote credential specification", agent.ID())
	}
	if len(spec.Methods) == 0 {
		return nil
	}
	configuredFiles, err := decodeRemoteCredentialIDs(executorProfile.Config["remote_credentials"])
	if err != nil {
		return fmt.Errorf("invalid remote_credentials: %w", err)
	}
	configuredFileSet := make(map[string]struct{}, len(configuredFiles))
	for _, methodID := range configuredFiles {
		configuredFileSet[methodID] = struct{}{}
	}
	configuredSecrets, err := decodeRemoteCredentialSecrets(executorProfile.Config["remote_auth_secrets"])
	if err != nil {
		return fmt.Errorf("invalid remote_auth_secrets: %w", err)
	}
	for _, method := range spec.Methods {
		if method.Type != "env" {
			if _, ok := configuredFileSet[method.MethodID]; ok {
				return nil
			}
			continue
		}
		if secret, ok := configuredSecrets[method.MethodID]; ok && strings.TrimSpace(secret) != "" {
			return nil
		}
	}
	return fmt.Errorf("no credentials are configured for agent family %q", agent.ID())
}

func decodeRemoteCredentialIDs(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil, err
	}
	return values, nil
}

func decodeRemoteCredentialSecrets(raw string) (map[string]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var values map[string]*string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil, err
	}
	result := make(map[string]string, len(values))
	for key, value := range values {
		if value != nil {
			result[key] = *value
		}
	}
	return result, nil
}
