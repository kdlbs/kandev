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
	"github.com/kandev/kandev/internal/task/models"
)

type taskAgentExecutorCompatibilityValidator struct {
	profiles        agentProfileCompatibilityReader
	agentRegistry   *registry.Registry
	dynamicResolver profileExecutionValidator
}

type profileExecutionValidator interface {
	ValidateProfile(context.Context, string) error
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
	if !ok || registeredAgent == nil || !registeredAgent.Enabled() || agents.IsVirtualAgent(registeredAgent) {
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
	if !models.IsRemoteExecutorType(executor.Type) {
		return nil
	}
	if executorProfile == nil {
		return fmt.Errorf("remote executor profile is required")
	}
	return validateRemoteAgentCredentials(registeredAgent, executorProfile)
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
