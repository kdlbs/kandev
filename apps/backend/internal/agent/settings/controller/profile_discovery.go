package controller

import (
	"context"
	"database/sql"
	"errors"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/hostutility"
	runtimeenv "github.com/kandev/kandev/internal/agent/runtime/environment"
	"github.com/kandev/kandev/internal/agent/settings/cliflags"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/secrets"
)

var (
	ErrInvalidProfileDiscovery = errors.New("invalid profile discovery context")
	ErrProfileDiscoveryMissing = errors.New("profile discovery target not found")
)

const agentAuthenticationRequiredMessage = "agent authentication is required"

func (c *Controller) FetchProfileDynamicModels(
	ctx context.Context,
	agentName string,
	req dto.ProfileCapabilityRequest,
) (*dto.DynamicModelsResponse, error) {
	if c.agentRegistry == nil {
		return nil, ErrAgentNotFound
	}
	if _, ok := c.agentRegistry.Get(agentName); !ok {
		return nil, ErrAgentNotFound
	}
	profileContext, err := c.resolveProfileProbeContext(ctx, agentName, req.AuthorizationScope, req.ProfileID, req.LaunchSettings)
	if err != nil {
		return nil, err
	}
	resp := &dto.DynamicModelsResponse{
		AgentName: agentName, Status: string(hostutility.StatusNotConfigured),
		Models: []dto.ModelEntryDTO{}, Modes: []dto.ModeEntryDTO{},
	}
	if c.hostUtility == nil {
		return resp, nil
	}
	provider, ok := c.hostUtility.(profileHostUtilityProvider)
	if !ok {
		resp.Status = string(hostutility.StatusUnsupported)
		message := "profile discovery is not supported by this host utility"
		resp.Error = &message
		return resp, nil
	}
	result, err := provider.ProbeProfileCapabilities(ctx, agentName, hostutility.ProfileCapabilityRequest{
		Context: profileContext, Refresh: req.Refresh,
	})
	if err != nil {
		return nil, err
	}
	caps := result.Capabilities
	resp.Status = string(caps.Status)
	resp.ContextRevision = result.ContextRevision
	if caps.Error != "" {
		message := profileFailureMessage(caps.Status)
		resp.Error = &message
	}
	resp.CurrentModelID = caps.CurrentModelID
	resp.CurrentModeID = caps.CurrentModeID
	for _, model := range caps.Models {
		resp.Models = append(resp.Models, dto.ModelEntryDTO{
			ID: model.ID, Name: model.Name, Description: model.Description,
			IsDefault: model.ID == caps.CurrentModelID, Meta: model.Meta,
		})
	}
	for _, mode := range caps.Modes {
		resp.Modes = append(resp.Modes, dto.ModeEntryDTO{ID: mode.ID, Name: mode.Name, Description: mode.Description, Meta: mode.Meta})
	}
	for _, command := range caps.Commands {
		resp.Commands = append(resp.Commands, dto.CommandEntryDTO{Name: command.Name, Description: command.Description})
	}
	return resp, nil
}

func (c *Controller) resolveProfileProbeContext(
	ctx context.Context,
	agentName, authorizationScope, profileID string,
	launchSettings *dto.ProfileLaunchSettingsRequest,
) (hostutility.ProfileProbeContext, error) {
	if c.repo == nil || c.agentRegistry == nil || authorizationScope == "" {
		return hostutility.ProfileProbeContext{}, ErrInvalidProfileDiscovery
	}
	agentID, registered, inference, err := c.resolveProfileProbeAgent(ctx, agentName)
	if err != nil {
		return hostutility.ProfileProbeContext{}, err
	}
	profile, err := c.resolveProfileDiscoveryProfile(ctx, profileID, agentID)
	if err != nil {
		return hostutility.ProfileProbeContext{}, err
	}
	launch, err := c.resolveProfileProbeLaunchSettings(ctx, registered, profile, launchSettings)
	if err != nil {
		return hostutility.ProfileProbeContext{}, err
	}
	definitions := profileProbeEnvironmentDefinitions(inference, launch.envVars)
	resolvedEnv, _, err := runtimeenv.Resolve(ctx, definitions, c.revealProfileDiscoverySecret)
	if err != nil {
		return hostutility.ProfileProbeContext{}, ErrInvalidProfileDiscovery
	}
	scope := profileProbeScope(authorizationScope, agentID, profile)
	return hostutility.ProfileProbeContext{
		Scope: scope, Env: resolvedEnv, CLIFlags: launch.cliTokens, CommandPrefix: launch.prefixTokens,
	}, nil
}

func (c *Controller) resolveProfileProbeAgent(
	ctx context.Context,
	agentName string,
) (string, agents.Agent, agents.InferenceAgent, error) {
	agent, err := c.repo.GetAgentByName(ctx, agentName)
	if errors.Is(err, sql.ErrNoRows) || agent == nil {
		return "", nil, nil, ErrAgentNotFound
	}
	if err != nil {
		return "", nil, nil, err
	}
	registered, ok := c.agentRegistry.Get(agentName)
	if !ok || isDisabledOptionalAgent(registered) {
		return "", nil, nil, ErrAgentNotFound
	}
	inference, ok := c.agentRegistry.GetInferenceAgent(agentName)
	if !ok {
		return "", nil, nil, ErrInvalidProfileDiscovery
	}
	return agent.ID, registered, inference, nil
}

func (c *Controller) resolveProfileDiscoveryProfile(
	ctx context.Context,
	profileID, agentID string,
) (*models.AgentProfile, error) {
	if profileID == "" {
		return nil, nil
	}
	profile, err := c.repo.GetAgentProfile(ctx, profileID)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && profile == nil) {
		return nil, ErrProfileDiscoveryMissing
	}
	if err != nil {
		return nil, err
	}
	if profile.AgentID != agentID || profile.WorkspaceID != "" || profile.Role != "" || profileKind(profile) != "concrete" {
		return nil, ErrProfileDiscoveryMissing
	}
	return profile, nil
}

type profileProbeLaunchSettings struct {
	envVars      []dto.ProfileEnvVarDTO
	cliTokens    []string
	prefixTokens []string
}

func (c *Controller) resolveProfileProbeLaunchSettings(
	ctx context.Context,
	registered agents.Agent,
	profile *models.AgentProfile,
	launchSettings *dto.ProfileLaunchSettingsRequest,
) (profileProbeLaunchSettings, error) {
	var envVars []dto.ProfileEnvVarDTO
	var cliFlags []dto.CLIFlagDTO
	var commandPrefix string
	cliPassthrough := false
	if launchSettings == nil {
		if profile == nil {
			return profileProbeLaunchSettings{}, ErrInvalidProfileDiscovery
		}
		envVars = envVarsToDTO(profile.EnvVars)
		cliFlags = cliFlagsToDTO(profile.CLIFlags)
		commandPrefix = profile.CommandPrefix
		cliPassthrough = profile.CLIPassthrough
	} else {
		if launchSettings.EnvVars == nil || launchSettings.CLIFlags == nil || launchSettings.CommandPrefix == nil {
			return profileProbeLaunchSettings{}, ErrInvalidProfileDiscovery
		}
		envVars = *launchSettings.EnvVars
		cliFlags = *launchSettings.CLIFlags
		commandPrefix = *launchSettings.CommandPrefix
		if profile != nil {
			cliPassthrough = profile.CLIPassthrough
		}
	}
	if validateProfileEnvVarDTOs(envVars) != nil || validateCLIFlagDTOs(cliFlags) != nil || validateCommandPrefix(commandPrefix) != nil {
		return profileProbeLaunchSettings{}, ErrInvalidProfileDiscovery
	}
	if err := validatePassthroughOnlyCLIFlags(registered, cliFlags, cliPassthrough); err != nil {
		return profileProbeLaunchSettings{}, ErrInvalidProfileDiscovery
	}
	if err := c.validateProfileDiscoverySecrets(ctx, envVars); err != nil {
		return profileProbeLaunchSettings{}, ErrInvalidProfileDiscovery
	}
	cliTokens, err := cliflags.Resolve(cliFlagsFromDTO(cliFlags))
	if err != nil {
		return profileProbeLaunchSettings{}, ErrInvalidProfileDiscovery
	}
	prefixTokens, err := cliflags.Tokenise(commandPrefix)
	if err != nil {
		return profileProbeLaunchSettings{}, ErrInvalidProfileDiscovery
	}
	return profileProbeLaunchSettings{envVars: envVars, cliTokens: cliTokens, prefixTokens: prefixTokens}, nil
}

func profileProbeEnvironmentDefinitions(
	inference agents.InferenceAgent,
	envVars []dto.ProfileEnvVarDTO,
) []runtimeenv.Definition {
	defaults := agents.RuntimeEnvFor(inference)
	profileKeys := make(map[string]struct{}, len(envVars))
	for _, envVar := range envVars {
		if envVar.Value == "" && envVar.SecretID == "" {
			continue
		}
		profileKeys[envVar.Key] = struct{}{}
	}
	definitions := make([]runtimeenv.Definition, 0, len(defaults)+len(envVars))
	for key, value := range defaults {
		if _, overridden := profileKeys[key]; overridden {
			continue
		}
		definitions = append(definitions, runtimeenv.Definition{
			Key: key, Literal: value, Origin: runtimeenv.OriginManagedAgentDefaults,
		})
	}
	for _, envVar := range envVars {
		if envVar.Value == "" && envVar.SecretID == "" {
			continue
		}
		definitions = append(definitions, runtimeenv.Definition{
			Key: envVar.Key, Literal: envVar.Value, SecretID: envVar.SecretID,
			Origin: runtimeenv.OriginAgentProfile,
		})
	}
	return definitions
}

func profileProbeScope(scope, agentID string, profile *models.AgentProfile) string {
	if profile != nil {
		return scope + ":" + profile.ID
	}
	return scope + ":draft:" + agentID
}

func (c *Controller) validateProfileDiscoverySecrets(ctx context.Context, envVars []dto.ProfileEnvVarDTO) error {
	for _, envVar := range envVars {
		if envVar.SecretID == "" {
			continue
		}
		if c.secretStore == nil {
			return errors.New("secret store unavailable")
		}
		if err := secrets.ValidateGlobalReference(ctx, c.secretStore, envVar.SecretID); err != nil {
			return errors.New("secret reference unavailable")
		}
	}
	return nil
}

func (c *Controller) revealProfileDiscoverySecret(ctx context.Context, definition runtimeenv.Definition) (string, error) {
	if c.secretStore == nil || definition.SecretID == "" {
		return "", errors.New("secret resolver unavailable")
	}
	if scoped, ok := c.secretStore.(secrets.ScopedSecretStore); ok {
		return scoped.RevealGlobal(ctx, definition.SecretID)
	}
	if err := secrets.ValidateGlobalReference(ctx, c.secretStore, definition.SecretID); err != nil {
		return "", err
	}
	return c.secretStore.Reveal(ctx, definition.SecretID)
}

func profileFailureMessage(status hostutility.Status) string {
	switch status {
	case hostutility.StatusUnsupported:
		return "profile launch settings are unsupported by this agent"
	case hostutility.StatusAuthRequired:
		return agentAuthenticationRequiredMessage
	default:
		return "profile discovery failed"
	}
}
