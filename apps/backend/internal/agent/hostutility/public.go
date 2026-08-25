package hostutility

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/settings/cliflags"
	agentctlutil "github.com/kandev/kandev/internal/agentctl/server/utility"
)

// ExecuteProfilePrompt resolves a complete profile snapshot at call start
// and executes it through the host utility instance. A missing or ineligible
// profile fails before dispatch; no agent/model fallback is used.
func (m *Manager) ExecuteProfilePrompt(ctx context.Context, profileID, prompt string) (*PromptResult, error) {
	if m.profileResolver == nil {
		return nil, errors.New("utility profile resolver is not configured")
	}
	profile, err := m.profileResolver.Resolve(ctx, profileID)
	if err != nil {
		return nil, err
	}
	cliFlags, err := cliflags.Resolve(profile.CLIFlags)
	if err != nil {
		return nil, fmt.Errorf("resolve profile cli flags: %w", err)
	}
	var prefix []string
	if profile.CommandPrefix != "" {
		if err := cliflags.ValidateCommandPrefix(profile.CommandPrefix); err != nil {
			return nil, err
		}
		prefix, err = cliflags.Tokenise(profile.CommandPrefix)
		if err != nil {
			return nil, err
		}
	}
	env := make(map[string]string, len(profile.EnvVars))
	for _, value := range profile.EnvVars {
		if value.Key != "" && value.SecretID == "" {
			env[value.Key] = value.Value
		}
	}
	inst, ia, err := m.getInstance(ctx, profile.AgentID)
	if err != nil {
		return nil, err
	}
	cfg := ia.InferenceConfig()
	command, err := m.resolveInferenceCommand(ctx, profile.AgentID, ia, agents.Command{})
	if err != nil {
		return nil, err
	}
	resolved := m.resolveModel(profile.AgentID, profile.Model, ia)
	autoApprove := profile.AutoApprove
	req := &agentctlutil.PromptRequest{
		Prompt:                 prompt,
		AgentID:                profile.AgentID,
		Model:                  resolved,
		Mode:                   profile.Mode,
		AutoApprovePermissions: &autoApprove,
		InferenceConfig: &agentctlutil.InferenceConfigDTO{
			Command: command.Args(), ModelFlag: cfg.ModelFlag.Args(), WorkDir: inst.workDir,
			Env: env, StripEnv: agents.StripEnvFor(ia), CLIFlags: cliFlags, CommandPrefix: prefix,
		},
	}
	resp, err := inst.client.InferencePrompt(ctx, req)
	if err != nil {
		return nil, err
	}
	if !resp.Success {
		return nil, errors.New(resp.Error)
	}
	return &PromptResult{Response: resp.Response, Model: resp.Model, PromptTokens: resp.PromptTokens, ResponseTokens: resp.ResponseTokens, DurationMs: resp.DurationMs}, nil
}

// GetAll returns a snapshot of every probed agent type's capabilities.
func (m *Manager) GetAll() []AgentCapabilities {
	if m == nil || m.cache == nil {
		return nil
	}
	return m.cache.all()
}

// Get returns the cached capabilities for a single agent type.
func (m *Manager) Get(agentType string) (AgentCapabilities, bool) {
	if m == nil || m.cache == nil {
		return AgentCapabilities{}, false
	}
	return m.cache.get(agentType)
}

func (m *Manager) invalidateModelConfigCache(agentType string) uint64 {
	if m == nil {
		return 0
	}
	if m.modelCache != nil {
		m.modelCache.invalidateAgent(agentType)
	}
	m.modelGenerationMu.Lock()
	defer m.modelGenerationMu.Unlock()
	if m.modelGenerations == nil {
		m.modelGenerations = make(map[string]uint64)
	}
	m.modelGenerations[agentType]++
	return m.modelGenerations[agentType]
}

func (m *Manager) modelConfigGeneration(agentType string) uint64 {
	m.modelGenerationMu.Lock()
	defer m.modelGenerationMu.Unlock()
	return m.modelGenerations[agentType]
}

// ResolveModelConfig applies a model in a sessionless ACP probe and returns the
// provider's complete configuration-option snapshot. Successful resolutions
// are cached for a short time and identical concurrent requests share one
// probe.
func (m *Manager) ResolveModelConfig(
	ctx context.Context,
	agentType string,
	req ModelConfigResolutionRequest,
) (ModelConfigResolution, error) {
	if m == nil || m.modelCache == nil {
		return ModelConfigResolution{}, errors.New("host utility manager not configured")
	}
	if req.Model == "" {
		return ModelConfigResolution{}, errors.New("model is required")
	}

	key := modelConfigCacheKey(agentType, req)
	generation := m.modelConfigGeneration(agentType)
	if req.Refresh {
		generation = m.invalidateModelConfigCache(agentType)
	}
	if !req.Refresh {
		if cached, ok := m.modelCache.get(key, time.Now()); ok {
			return cached, nil
		}
	}

	flightKey := fmt.Sprintf("%d:%s", generation, key)
	value, err, _ := m.modelGroup.Do(flightKey, func() (interface{}, error) {
		return m.resolveModelConfigFlight(ctx, agentType, req, key, generation)
	})
	if err != nil {
		return ModelConfigResolution{}, err
	}
	return value.(ModelConfigResolution), nil
}

func (m *Manager) resolveModelConfigFlight(
	ctx context.Context,
	agentType string,
	req ModelConfigResolutionRequest,
	key string,
	generation uint64,
) (interface{}, error) {
	if !req.Refresh {
		if cached, ok := m.modelCache.get(key, time.Now()); ok {
			return cached, nil
		}
	}

	// All work inside the shared flight uses a detached request context.
	// A caller disconnecting must not cancel the probe for other waiters;
	// the bounded timeout remains the lifetime limit for the shared work.
	probeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), modelConfigResolveTimeout)
	defer cancel()
	inst, ia, err := m.getInstance(probeCtx, agentType)
	if err != nil {
		return nil, err
	}
	cfg := ia.InferenceConfig()
	if cfg == nil || !cfg.Supported {
		return nil, errors.New("inference config not available")
	}
	command, err := m.resolveInferenceCommand(probeCtx, agentType, ia, agents.Command{})
	if err != nil {
		return nil, err
	}

	probeReq := buildProbeRequest(inst, ia, req.Refresh, command)
	probeReq.Model = req.Model
	probeReq.Mode = req.Mode
	probeReq.ConfigOptions = cloneStringMap(req.ConfigOptions)
	resp, err := inst.client.Probe(probeCtx, probeReq)
	if err != nil {
		return nil, err
	}

	resolution := ModelConfigResolution{
		AgentType: agentType,
		Model:     req.Model,
		Status:    StatusFailed,
	}
	if !resp.Success {
		resolution.Error = resp.Error
		if isAuthError(resp.Error) {
			resolution.Status = StatusAuthRequired
		}
		m.log.Warn("model config resolution failed",
			zap.String("agent_type", agentType),
			zap.String("model", req.Model),
			zap.String("status", string(resolution.Status)),
			zap.String("error", resp.Error))
		return resolution, nil
	}

	resolution.Status = StatusOK
	resolution.ConfigOptions = configOptionsFromProbe(resp.ConfigOptions)
	if m.modelConfigGeneration(agentType) == generation {
		m.modelCache.set(key, resolution, time.Now())
	}
	return resolution, nil
}

// Refresh re-probes the given agent type, refreshes the cache, and returns the
// new capabilities. If the warm instance is missing (never bootstrapped or
// crashed), it is lazily recreated.
func (m *Manager) Refresh(ctx context.Context, agentType string) (AgentCapabilities, error) {
	m.invalidateModelConfigCache(agentType)
	// Publish "probing" so callers polling the cache see in-flight state.
	m.cache.set(AgentCapabilities{
		AgentType:     agentType,
		Status:        StatusProbing,
		LastCheckedAt: time.Now(),
	})
	inst, ia, err := m.getInstance(ctx, agentType)
	if err != nil {
		status := StatusFailed
		if errors.Is(err, errAgentNotInstalled) {
			status = StatusNotInstalled
		}
		m.cache.set(AgentCapabilities{
			AgentType:     agentType,
			Status:        status,
			Error:         err.Error(),
			LastCheckedAt: time.Now(),
		})
		return AgentCapabilities{}, err
	}
	caps := m.probe(ctx, inst, ia, true)
	m.cache.set(caps)
	return caps, nil
}

// RefreshWithCommand probes an agent with a trusted command override. Unlike
// Refresh, it preserves the last successful capability record unless the
// override probe succeeds. Runtime-update callers use this after priming npm's
// cache so a failed new runtime cannot erase the model list currently shown.
func (m *Manager) RefreshWithCommand(
	ctx context.Context,
	agentType string,
	command agents.Command,
) (AgentCapabilities, error) {
	caps, err := m.ProbeWithCommand(ctx, agentType, command)
	if err != nil {
		return AgentCapabilities{}, err
	}
	if caps.Status == StatusOK {
		m.PublishCapabilities(agentType, caps)
	}
	return caps, nil
}

// ProbeWithCommand runs a trusted command override without changing the live
// capability cache. Runtime activation uses this boundary before persistence.
func (m *Manager) ProbeWithCommand(
	ctx context.Context,
	agentType string,
	command agents.Command,
) (AgentCapabilities, error) {
	m.invalidateModelConfigCache(agentType)
	inst, ia, err := m.getInstance(ctx, agentType)
	if err != nil {
		return AgentCapabilities{}, err
	}
	caps := m.probeWithCommand(ctx, inst, ia, true, command)
	return caps, nil
}

// PublishCapabilities makes a successful candidate the live catalogue. The
// caller is responsible for persisting the active selection first.
func (m *Manager) PublishCapabilities(agentType string, caps AgentCapabilities) {
	if caps.Status != StatusOK {
		return
	}
	m.invalidateModelConfigCache(agentType)
	m.cache.set(caps)
}

// ExecutePrompt runs a sessionless utility prompt against the warm instance
// for the given agent type. Convenience wrapper over ExecutePromptWithMCP
// that opts the call out of MCP tool access — most callers (PR title, commit
// message, etc.) want pure text-in/text-out.
func (m *Manager) ExecutePrompt(
	ctx context.Context,
	agentType, model, mode, prompt string,
) (*PromptResult, error) {
	return m.ExecutePromptWithMCP(ctx, agentType, model, mode, prompt, nil)
}

// ExecutePromptWithMCP runs a sessionless utility prompt with the given MCP
// servers wired into the agent's session. The agent (Claude Code, codex,
// etc.) can call MCP tools mid-prompt and incorporate the results into its
// final reply. Pass nil for mcpServers to disable MCP for this call.
func (m *Manager) ExecutePromptWithMCP(
	ctx context.Context,
	agentType, model, mode, prompt string,
	mcpServers []agentctlutil.MCPServerDTO,
) (*PromptResult, error) {
	if prompt == "" {
		return nil, fmt.Errorf("prompt is required")
	}
	inst, ia, err := m.getInstance(ctx, agentType)
	if err != nil {
		return nil, err
	}
	cfg := ia.InferenceConfig()
	command, err := m.resolveInferenceCommand(ctx, agentType, ia, agents.Command{})
	if err != nil {
		return nil, err
	}

	resolved := m.resolveModel(agentType, model, ia)

	req := &agentctlutil.PromptRequest{
		Prompt:  prompt,
		AgentID: agentType,
		Model:   resolved,
		Mode:    mode,
		InferenceConfig: &agentctlutil.InferenceConfigDTO{
			Command:   command.Args(),
			ModelFlag: cfg.ModelFlag.Args(),
			WorkDir:   inst.workDir,
			Env:       agents.RuntimeEnvFor(ia),
			StripEnv:  agents.StripEnvFor(ia),
		},
		MCPServers: mcpServers,
	}
	resp, err := inst.client.InferencePrompt(ctx, req)
	if err != nil {
		return nil, err
	}
	if !resp.Success {
		return nil, fmt.Errorf("%s", resp.Error)
	}
	return &PromptResult{
		Response:       resp.Response,
		Model:          resp.Model,
		PromptTokens:   resp.PromptTokens,
		ResponseTokens: resp.ResponseTokens,
		DurationMs:     resp.DurationMs,
	}, nil
}

// resolveModel picks the model to use for an ExecutePrompt call.
// Precedence: explicit argument > cached probe currentModelID.
// Static per-agent default models no longer exist; probes are the source of truth.
func (m *Manager) resolveModel(agentType, explicit string, _ agents.InferenceAgent) string {
	if explicit != "" {
		return explicit
	}
	if caps, ok := m.cache.get(agentType); ok && caps.CurrentModelID != "" {
		return caps.CurrentModelID
	}
	return ""
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func configOptionsFromProbe(options []agentctlutil.ProbeConfigOption) []ConfigOption {
	out := make([]ConfigOption, 0, len(options))
	for _, option := range options {
		choices := make([]ConfigOptionChoice, 0, len(option.Options))
		for _, choice := range option.Options {
			choices = append(choices, ConfigOptionChoice{
				Value:       choice.Value,
				Name:        choice.Name,
				Description: choice.Description,
			})
		}
		out = append(out, ConfigOption{
			Type:         option.Type,
			ID:           option.ID,
			Name:         option.Name,
			Description:  option.Description,
			CurrentValue: option.CurrentValue,
			Category:     option.Category,
			Options:      choices,
		})
	}
	return out
}
