package backendapp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/agent/remoteauth"
	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	agentsettingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/task/models"
)

type taskAgentExecutorCompatibilityValidator struct {
	profiles        agentProfileCompatibilityReader
	agentRegistry   *registry.Registry
	dynamicResolver *agentruntime.ProfileExecutionResolver
}

type agentProfileCompatibilityReader interface {
	GetAgent(ctx context.Context, id string) (*agentsettingsmodels.Agent, error)
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
	if v.profiles == nil || v.agentRegistry == nil {
		return fmt.Errorf("agent compatibility registry is unavailable")
	}
	settingsAgent, err := v.profiles.GetAgent(ctx, profile.AgentID)
	if err != nil {
		return fmt.Errorf("load agent family: %w", err)
	}
	if settingsAgent == nil {
		return fmt.Errorf("agent family is unavailable")
	}
	registeredAgent, ok := v.agentRegistry.Get(settingsAgent.Name)
	if !ok || registeredAgent == nil || !registeredAgent.Enabled() || agents.IsVirtualAgent(registeredAgent) {
		return fmt.Errorf("agent family %q cannot execute", settingsAgent.Name)
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
