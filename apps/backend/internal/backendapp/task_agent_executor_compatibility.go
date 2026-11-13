package backendapp

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/agent/remoteauth"
	agentsettingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/models"
)

type taskAgentExecutorCompatibilityValidator struct {
	profiles        agentProfileCompatibilityReader
	agentRegistry   *registry.Registry
	dynamicResolver profileExecutionValidator
	secrets         secrets.SecretStore
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
	return validateRemoteAgentCredentials(ctx, registeredAgent, profile, executorProfile, v.secrets)
}

func validateRemoteAgentCredentials(
	ctx context.Context,
	agent agents.Agent,
	profile *agentsettingsmodels.AgentProfile,
	executorProfile *models.ExecutorProfile,
	store secrets.SecretStore,
) error {
	return remoteauth.ValidateCredentialAdmission(ctx, remoteauth.AdmissionInput{
		AgentID:         agent.ID(),
		ProfileEnvVars:  profile.EnvVars,
		ExecutorEnvVars: executorProfile.EnvVars,
		ExecutorConfig:  executorProfile.Config,
		Catalog:         remoteauth.BuildCatalog([]agents.Agent{agent}),
		SecretStore:     store,
	})
}
