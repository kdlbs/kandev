package hostutility

import (
	"container/list"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/managedruntime"
	agentctlutil "github.com/kandev/kandev/internal/agentctl/server/utility"
	"github.com/kandev/kandev/internal/common/launchwrapper"
)

var errProfileContextChanged = errors.New("profile discovery context changed during probe; retry")

type profileDiscoveryFlight struct {
	agentType string
	scopeKey  string
	signature string
	cancel    context.CancelCauseFunc
}

func (m *Manager) beginProfileDiscoveryFlight(
	parent context.Context,
	agentType, scope, baseRevision string,
	runtimeGeneration, contextGeneration uint64,
) (context.Context, func()) {
	flightCtx, cancel := context.WithCancelCause(parent)
	scopeKey, signature := m.profileDiscoveryFlightIdentity(agentType, scope, baseRevision, runtimeGeneration, contextGeneration)
	m.modelGenerationMu.Lock()
	defer m.modelGenerationMu.Unlock()
	if m.profileGenerations[agentType] != runtimeGeneration || m.profileContextGenerations[baseRevision] != contextGeneration {
		cancel(errProfileContextChanged)
		return flightCtx, func() {}
	}
	m.profileFlightMu.Lock()
	if m.profileDiscoveryFlights == nil {
		m.profileDiscoveryFlights = make(map[uint64]profileDiscoveryFlight)
	}
	var superseded []context.CancelCauseFunc
	for id, flight := range m.profileDiscoveryFlights {
		if flight.scopeKey == scopeKey && flight.signature != signature {
			superseded = append(superseded, flight.cancel)
			delete(m.profileDiscoveryFlights, id)
		}
	}
	m.nextProfileDiscoveryFlight++
	flightID := m.nextProfileDiscoveryFlight
	m.profileDiscoveryFlights[flightID] = profileDiscoveryFlight{
		agentType: agentType, scopeKey: scopeKey, signature: signature, cancel: cancel,
	}
	m.profileFlightMu.Unlock()
	for _, supersede := range superseded {
		supersede(errProfileContextChanged)
	}
	return flightCtx, func() {
		cancel(context.Canceled)
		m.profileFlightMu.Lock()
		delete(m.profileDiscoveryFlights, flightID)
		m.profileFlightMu.Unlock()
	}
}

func (m *Manager) profileDiscoveryFlightIdentity(
	agentType, scope, baseRevision string,
	runtimeGeneration, contextGeneration uint64,
) (string, string) {
	scopeKey := profileHMAC(m.profileKey, []byte("profile-scope-v1:"+agentType+"\x00"+scope))
	signature := fmt.Sprintf("%s:%d:%d", baseRevision, runtimeGeneration, contextGeneration)
	return scopeKey, signature
}

func (m *Manager) profileDiscoveryFlightIsCurrentLocked(
	agentType, scope, baseRevision string,
	runtimeGeneration, contextGeneration uint64,
) bool {
	scopeKey, signature := m.profileDiscoveryFlightIdentity(agentType, scope, baseRevision, runtimeGeneration, contextGeneration)
	for _, flight := range m.profileDiscoveryFlights {
		if flight.agentType == agentType && flight.scopeKey == scopeKey && flight.signature == signature {
			return true
		}
	}
	return false
}

func (m *Manager) cancelProfileDiscoveryFlights(agentType string) {
	m.profileFlightMu.Lock()
	var cancellations []context.CancelCauseFunc
	for id, flight := range m.profileDiscoveryFlights {
		if agentType == "" || flight.agentType == agentType {
			cancellations = append(cancellations, flight.cancel)
			delete(m.profileDiscoveryFlights, id)
		}
	}
	m.profileFlightMu.Unlock()
	for _, cancel := range cancellations {
		cancel(errProfileContextChanged)
	}
}

// ProbeProfileCapabilities runs a one-shot profile-context discovery probe.
// Verified incomplete managed npm roots may be prepared privately once; the
// resulting capability snapshot remains scoped to this profile context.
func (m *Manager) ProbeProfileCapabilities(
	ctx context.Context,
	agentType string,
	req ProfileCapabilityRequest,
) (ProfileCapabilityResult, error) {
	if m == nil || m.profileCache == nil {
		return ProfileCapabilityResult{}, errors.New("host utility manager not configured")
	}
	if strings.TrimSpace(req.Context.Scope) == "" {
		return ProfileCapabilityResult{}, errors.New("profile scope is required")
	}
	probeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), modelConfigResolveTimeout)
	defer cancel()
	inst, ia, cfg, command, localEnv, runtimeGeneration, err := m.profileProbeInputs(probeCtx, agentType)
	if err != nil {
		return ProfileCapabilityResult{}, err
	}
	baseRevision := profileContextRevision(m.profileKey, req.Context, agentType, string(cfg.Protocol), command.Args(), cfg.OperatorDefined, agents.StripEnvFor(ia), runtimeGeneration, 0)
	contextGeneration, current := m.profileContextGeneration(agentType, baseRevision, runtimeGeneration, req.Refresh)
	if !current {
		return ProfileCapabilityResult{}, errProfileContextChanged
	}
	revision := profileContextRevision(m.profileKey, req.Context, agentType, string(cfg.Protocol), command.Args(), cfg.OperatorDefined, agents.StripEnvFor(ia), runtimeGeneration, contextGeneration)
	key := profileCapabilityKey(m.profileKey, revision)
	if !req.Refresh {
		if cached, ok, current := m.getProfileCapabilityIfCurrent(agentType, baseRevision, runtimeGeneration, contextGeneration, key); !current {
			return ProfileCapabilityResult{}, errProfileContextChanged
		} else if ok {
			return cached, nil
		}
	}
	flightKey := key
	value, err, _ := m.profileGroup.Do(flightKey, func() (interface{}, error) {
		if !req.Refresh {
			if cached, ok, current := m.getProfileCapabilityIfCurrent(agentType, baseRevision, runtimeGeneration, contextGeneration, key); !current {
				return nil, errProfileContextChanged
			} else if ok {
				return cached, nil
			}
		}
		if !m.profileContextIsCurrent(agentType, baseRevision, runtimeGeneration, contextGeneration) {
			return nil, errProfileContextChanged
		}
		flightCtx, finishFlight := m.beginProfileDiscoveryFlight(
			probeCtx, agentType, req.Context.Scope, baseRevision, runtimeGeneration, contextGeneration,
		)
		defer finishFlight()
		return m.probeProfileCapabilitiesFlight(flightCtx, inst, ia, cfg, command, localEnv, req, key, baseRevision, revision, runtimeGeneration, contextGeneration)
	})
	if err != nil {
		return ProfileCapabilityResult{}, err
	}
	if !m.profileContextIsCurrent(agentType, baseRevision, runtimeGeneration, contextGeneration) {
		return ProfileCapabilityResult{}, errProfileContextChanged
	}
	return value.(ProfileCapabilityResult), nil
}

func (m *Manager) probeProfileCapabilitiesFlight(
	ctx context.Context,
	inst *instance,
	ia agents.InferenceAgent,
	cfg *agents.InferenceConfig,
	command agents.Command,
	localEnv map[string]string,
	req ProfileCapabilityRequest,
	key, baseRevision, revision string,
	runtimeGeneration, contextGeneration uint64,
) (ProfileCapabilityResult, error) {
	if !m.profileContextIsCurrent(inst.agentType, baseRevision, runtimeGeneration, contextGeneration) {
		return ProfileCapabilityResult{}, errProfileContextChanged
	}
	pluginFlags, err := m.profileCopilotPluginFlags(ctx, inst.agentType, req.Context, inst.workDir, command.Args())
	if err != nil {
		status := profileCopilotPluginStatus(err)
		caps := AgentCapabilities{
			AgentType: inst.agentType, Status: status,
			Error: err.Error(), LastCheckedAt: time.Now(),
		}
		return ProfileCapabilityResult{
			Capabilities: caps, ContextRevision: revision,
			ReasonCode: profileCopilotPluginReasonCode(inst.agentType, status, err),
		}, nil
	}
	probeReq := buildProbeRequest(inst, ia, req.Refresh, command)
	probeReq.ProfileContext = true
	probeReq.InferenceConfig.Env = mergeInferenceEnv(cloneStringMap(req.Context.Env), localEnv)
	probeReq.InstallContext = managedRuntimeProbeInstallContext(
		probeReq.InferenceConfig,
		managedruntime.RepairOwnerScopeProfile(req.Context.Scope),
	)
	cliFlags := req.Context.CLIFlags
	if pluginFlags != nil {
		cliFlags = pluginFlags
	}
	probeReq.InferenceConfig.CLIFlags = append([]string(nil), cliFlags...)
	probeReq.InferenceConfig.LaunchWrapper = launchwrapper.Profile(req.Context.CommandPrefix)
	probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	release, err := inst.acquireOperation(probeCtx, false)
	if err != nil {
		if errors.Is(context.Cause(probeCtx), errProfileContextChanged) {
			return ProfileCapabilityResult{}, errProfileContextChanged
		}
		return ProfileCapabilityResult{}, err
	}
	resp, err := inst.client.Probe(probeCtx, probeReq)
	release()
	if err != nil {
		if errors.Is(context.Cause(probeCtx), errProfileContextChanged) {
			return ProfileCapabilityResult{}, errProfileContextChanged
		}
		return ProfileCapabilityResult{}, err
	}
	resp = m.retryManagedRuntimeProbe(
		probeCtx, inst, ia, agents.Command{}, command, probeReq, resp,
		func() bool {
			return !errors.Is(context.Cause(probeCtx), errProfileContextChanged) &&
				m.profileContextIsCurrent(inst.agentType, baseRevision, runtimeGeneration, contextGeneration)
		},
		true,
	)
	if errors.Is(context.Cause(probeCtx), errProfileContextChanged) ||
		!m.profileContextIsCurrent(inst.agentType, baseRevision, runtimeGeneration, contextGeneration) {
		return ProfileCapabilityResult{}, errProfileContextChanged
	}
	caps := capabilitiesFromProbe(inst.agentType, resp, time.Now())
	stampConfiguredRuntimeVersion(&caps, ia, command)
	if !resp.Success {
		caps.Status = profileProbeStatus(resp)
		caps.Error = profileProbeError(resp)
	}
	result := ProfileCapabilityResult{Capabilities: caps, ContextRevision: revision}
	if !m.setProfileCapabilityIfCurrent(inst.agentType, req.Context.Scope, baseRevision, runtimeGeneration, contextGeneration, key, result) {
		return ProfileCapabilityResult{}, errProfileContextChanged
	}
	m.log.Debug("profile capability probe completed",
		zap.String("agent_type", inst.agentType),
		zap.String("status", string(caps.Status)),
		zap.String("context_revision", revision),
		zap.Int("duration_ms", caps.DurationMs))
	if errors.Is(context.Cause(ctx), errProfileContextChanged) {
		return ProfileCapabilityResult{}, errProfileContextChanged
	}
	return result, nil
}

func (m *Manager) resolveProfileModelConfig(
	ctx context.Context,
	agentType string,
	req ModelConfigResolutionRequest,
) (ModelConfigResolution, error) {
	if m == nil || m.profileCache == nil {
		return ModelConfigResolution{}, errors.New("host utility manager not configured")
	}
	if req.Model == "" || req.ProfileContext == nil || strings.TrimSpace(req.ProfileContext.Scope) == "" {
		return ModelConfigResolution{}, errors.New("profile model context is incomplete")
	}
	probeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), modelConfigResolveTimeout)
	defer cancel()
	inst, ia, cfg, command, localEnv, runtimeGeneration, err := m.profileProbeInputs(probeCtx, agentType)
	if err != nil {
		return ModelConfigResolution{}, err
	}
	baseRevision := profileContextRevision(m.profileKey, *req.ProfileContext, agentType, string(cfg.Protocol), command.Args(), cfg.OperatorDefined, agents.StripEnvFor(ia), runtimeGeneration, 0)
	contextGeneration, current := m.profileContextGeneration(agentType, baseRevision, runtimeGeneration, req.Refresh)
	if !current {
		return ModelConfigResolution{}, errProfileContextChanged
	}
	revision := profileContextRevision(m.profileKey, *req.ProfileContext, agentType, string(cfg.Protocol), command.Args(), cfg.OperatorDefined, agents.StripEnvFor(ia), runtimeGeneration, contextGeneration)
	key := profileModelConfigKey(m.profileKey, revision, req)
	if !req.Refresh {
		cached, ok, current := m.getProfileResolutionIfCurrent(agentType, baseRevision, runtimeGeneration, contextGeneration, key)
		if !current {
			return ModelConfigResolution{}, errProfileContextChanged
		}
		if ok {
			return cached, nil
		}
	}
	value, err, _ := m.profileGroup.Do(key, func() (interface{}, error) {
		if !req.Refresh {
			cached, ok, current := m.getProfileResolutionIfCurrent(agentType, baseRevision, runtimeGeneration, contextGeneration, key)
			if !current {
				return nil, errProfileContextChanged
			}
			if ok {
				return cached, nil
			}
		}
		if !m.profileContextIsCurrent(agentType, baseRevision, runtimeGeneration, contextGeneration) {
			return nil, errProfileContextChanged
		}
		flightCtx, finishFlight := m.beginProfileDiscoveryFlight(
			probeCtx, agentType, req.ProfileContext.Scope, baseRevision, runtimeGeneration, contextGeneration,
		)
		defer finishFlight()
		return m.resolveProfileModelConfigFlightOrCached(
			flightCtx, inst, ia, command, localEnv, req, key, baseRevision, revision, runtimeGeneration, contextGeneration,
		)
	})
	if err != nil {
		return ModelConfigResolution{}, err
	}
	if !m.profileContextIsCurrent(agentType, baseRevision, runtimeGeneration, contextGeneration) {
		return ModelConfigResolution{}, errProfileContextChanged
	}
	return value.(ModelConfigResolution), nil
}

func (m *Manager) resolveProfileModelConfigFlightOrCached(
	ctx context.Context,
	inst *instance,
	ia agents.InferenceAgent,
	command agents.Command,
	localEnv map[string]string,
	req ModelConfigResolutionRequest,
	key, baseRevision, revision string,
	runtimeGeneration, contextGeneration uint64,
) (interface{}, error) {
	if !req.Refresh {
		cached, ok, current := m.getProfileResolutionIfCurrent(
			inst.agentType, baseRevision, runtimeGeneration, contextGeneration, key,
		)
		if !current {
			return nil, errProfileContextChanged
		}
		if ok {
			return cached, nil
		}
	}
	if !m.profileContextIsCurrent(inst.agentType, baseRevision, runtimeGeneration, contextGeneration) {
		return nil, errProfileContextChanged
	}
	return m.resolveProfileModelConfigFlight(
		ctx, inst, ia, command, localEnv, req, key, baseRevision, revision, runtimeGeneration, contextGeneration,
	)
}

func (m *Manager) resolveProfileModelConfigFlight(
	ctx context.Context,
	inst *instance,
	ia agents.InferenceAgent,
	command agents.Command,
	localEnv map[string]string,
	req ModelConfigResolutionRequest,
	key, baseRevision, revision string,
	runtimeGeneration, contextGeneration uint64,
) (ModelConfigResolution, error) {
	if !m.profileContextIsCurrent(inst.agentType, baseRevision, runtimeGeneration, contextGeneration) {
		return ModelConfigResolution{}, errProfileContextChanged
	}
	pluginFlags, err := m.profileCopilotPluginFlags(ctx, inst.agentType, *req.ProfileContext, inst.workDir, command.Args())
	if err != nil {
		status := profileCopilotPluginStatus(err)
		resolution := ModelConfigResolution{
			AgentType: inst.agentType, Model: req.Model,
			Status: status, Error: err.Error(),
			ContextRevision: revision,
			ReasonCode:      profileCopilotPluginReasonCode(inst.agentType, status, err),
		}
		return resolution, nil
	}
	probeReq := buildProbeRequest(inst, ia, req.Refresh, command)
	probeReq.ProfileContext = true
	probeReq.Model = req.Model
	probeReq.Mode = req.Mode
	probeReq.ConfigOptions = cloneStringMap(req.ConfigOptions)
	probeReq.InferenceConfig.Env = mergeInferenceEnv(cloneStringMap(req.ProfileContext.Env), localEnv)
	probeReq.InstallContext = managedRuntimeProbeInstallContext(
		probeReq.InferenceConfig,
		managedruntime.RepairOwnerScopeProfile(req.ProfileContext.Scope),
	)
	cliFlags := req.ProfileContext.CLIFlags
	if pluginFlags != nil {
		cliFlags = pluginFlags
	}
	probeReq.InferenceConfig.CLIFlags = append([]string(nil), cliFlags...)
	probeReq.InferenceConfig.LaunchWrapper = launchwrapper.Profile(req.ProfileContext.CommandPrefix)
	probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	release, err := inst.acquireOperation(probeCtx, false)
	if err != nil {
		if errors.Is(context.Cause(probeCtx), errProfileContextChanged) {
			return ModelConfigResolution{}, errProfileContextChanged
		}
		return ModelConfigResolution{}, err
	}
	resp, err := inst.client.Probe(probeCtx, probeReq)
	release()
	if err != nil {
		if errors.Is(context.Cause(probeCtx), errProfileContextChanged) {
			return ModelConfigResolution{}, errProfileContextChanged
		}
		return ModelConfigResolution{}, err
	}
	resp = m.retryManagedRuntimeProbe(
		probeCtx, inst, ia, agents.Command{}, command, probeReq, resp,
		func() bool {
			return !errors.Is(context.Cause(probeCtx), errProfileContextChanged) &&
				m.profileContextIsCurrent(inst.agentType, baseRevision, runtimeGeneration, contextGeneration)
		},
		true,
	)
	if errors.Is(context.Cause(probeCtx), errProfileContextChanged) ||
		!m.profileContextIsCurrent(inst.agentType, baseRevision, runtimeGeneration, contextGeneration) {
		return ModelConfigResolution{}, errProfileContextChanged
	}
	resolution := ModelConfigResolution{
		AgentType: inst.agentType, Model: req.Model, Status: profileProbeStatus(resp),
		ContextRevision: revision,
	}
	if resp.Success {
		resolution.Status = StatusOK
		resolution.ConfigOptions = configOptionsFromProbe(resp.ConfigOptions)
	} else {
		resolution.Error = profileProbeError(resp)
	}
	if !m.setProfileResolutionIfCurrent(inst.agentType, req.ProfileContext.Scope, baseRevision, runtimeGeneration, contextGeneration, key, resolution) {
		return ModelConfigResolution{}, errProfileContextChanged
	}
	m.log.Debug("profile model options probe completed",
		zap.String("agent_type", inst.agentType),
		zap.String("status", string(resolution.Status)),
		zap.String("context_revision", revision))
	if errors.Is(context.Cause(ctx), errProfileContextChanged) {
		return ModelConfigResolution{}, errProfileContextChanged
	}
	return resolution, nil
}

func (m *Manager) profileProbeInputs(
	ctx context.Context,
	agentType string,
) (*instance, agents.InferenceAgent, *agents.InferenceConfig, agents.Command, map[string]string, uint64, error) {
	runtimeGeneration := m.profileGeneration(agentType)
	inst, ia, err := m.getInstance(ctx, agentType)
	if err != nil {
		return nil, nil, nil, agents.Command{}, nil, 0, err
	}
	cfg := inferenceConfigForHostUtility(ia)
	if cfg == nil || !cfg.Supported {
		return nil, nil, nil, agents.Command{}, nil, 0, errors.New("inference configuration not available")
	}
	command, localEnv, err := m.resolveInferenceCommand(ctx, agentType, ia, agents.Command{})
	if err != nil {
		return nil, nil, nil, agents.Command{}, nil, 0, err
	}
	if m.profileGeneration(agentType) != runtimeGeneration {
		return nil, nil, nil, agents.Command{}, nil, 0, errProfileContextChanged
	}
	return inst, ia, cfg, command, localEnv, runtimeGeneration, nil
}

func (m *Manager) profileGeneration(agentType string) uint64 {
	m.modelGenerationMu.Lock()
	defer m.modelGenerationMu.Unlock()
	return m.profileGenerations[agentType]
}

func (m *Manager) invalidateProfileCapabilities(agentType string) uint64 {
	if m == nil {
		return 0
	}
	m.modelGenerationMu.Lock()
	if m.profileGenerations == nil {
		m.profileGenerations = make(map[string]uint64)
	}
	m.profileGenerations[agentType]++
	if m.profileCache != nil {
		m.profileCache.invalidateAgent(agentType)
	}
	generation := m.profileGenerations[agentType]
	m.cancelProfileDiscoveryFlights(agentType)
	m.modelGenerationMu.Unlock()
	return generation
}

func (m *Manager) profileContextGeneration(agentType, baseRevision string, runtimeGeneration uint64, refresh bool) (uint64, bool) {
	m.modelGenerationMu.Lock()
	defer m.modelGenerationMu.Unlock()
	if m.profileGenerations[agentType] != runtimeGeneration {
		return 0, false
	}
	if m.profileContextGenerations == nil {
		m.profileContextGenerations = make(map[string]uint64)
	}
	if m.profileContextGenerationNodes == nil {
		m.profileContextGenerationNodes = make(map[string]*list.Element)
	}
	if m.profileContextGenerationOrder == nil {
		m.profileContextGenerationOrder = list.New()
		for revision := range m.profileContextGenerations {
			m.profileContextGenerationNodes[revision] = m.profileContextGenerationOrder.PushFront(revision)
		}
	}

	node := m.profileContextGenerationNodes[baseRevision]
	if node == nil {
		m.nextProfileContextGeneration++
		m.profileContextGenerations[baseRevision] = m.nextProfileContextGeneration
		m.profileContextGenerationNodes[baseRevision] = m.profileContextGenerationOrder.PushFront(baseRevision)
	} else {
		m.profileContextGenerationOrder.MoveToFront(node)
		if refresh {
			m.nextProfileContextGeneration++
			m.profileContextGenerations[baseRevision] = m.nextProfileContextGeneration
		}
	}
	for len(m.profileContextGenerations) > profileCapabilityCacheMaxEntries {
		oldest := m.profileContextGenerationOrder.Back()
		if oldest == nil {
			break
		}
		revision := oldest.Value.(string)
		delete(m.profileContextGenerations, revision)
		delete(m.profileContextGenerationNodes, revision)
		m.profileContextGenerationOrder.Remove(oldest)
	}
	return m.profileContextGenerations[baseRevision], true
}

func (m *Manager) profileContextIsCurrent(agentType, baseRevision string, runtimeGeneration, contextGeneration uint64) bool {
	m.modelGenerationMu.Lock()
	defer m.modelGenerationMu.Unlock()
	return m.profileGenerations[agentType] == runtimeGeneration && m.profileContextGenerations[baseRevision] == contextGeneration
}

func (m *Manager) getProfileCapabilityIfCurrent(agentType, baseRevision string, runtimeGeneration, contextGeneration uint64, key string) (ProfileCapabilityResult, bool, bool) {
	m.modelGenerationMu.Lock()
	defer m.modelGenerationMu.Unlock()
	if m.profileGenerations[agentType] != runtimeGeneration || m.profileContextGenerations[baseRevision] != contextGeneration {
		return ProfileCapabilityResult{}, false, false
	}
	result, ok := m.profileCache.getCapability(key, time.Now())
	return result, ok, true
}

func (m *Manager) getProfileResolutionIfCurrent(agentType, baseRevision string, runtimeGeneration, contextGeneration uint64, key string) (ModelConfigResolution, bool, bool) {
	m.modelGenerationMu.Lock()
	defer m.modelGenerationMu.Unlock()
	if m.profileGenerations[agentType] != runtimeGeneration || m.profileContextGenerations[baseRevision] != contextGeneration {
		return ModelConfigResolution{}, false, false
	}
	resolution, ok := m.profileCache.getResolution(key, time.Now())
	return resolution, ok, true
}

func (m *Manager) setProfileCapabilityIfCurrent(agentType, scope, baseRevision string, runtimeGeneration, contextGeneration uint64, key string, result ProfileCapabilityResult) bool {
	m.modelGenerationMu.Lock()
	defer m.modelGenerationMu.Unlock()
	if m.profileGenerations[agentType] != runtimeGeneration || m.profileContextGenerations[baseRevision] != contextGeneration {
		return false
	}
	m.profileFlightMu.Lock()
	defer m.profileFlightMu.Unlock()
	if !m.profileDiscoveryFlightIsCurrentLocked(agentType, scope, baseRevision, runtimeGeneration, contextGeneration) {
		return false
	}
	m.profileCache.setCapability(key, agentType, result, time.Now())
	return true
}

func (m *Manager) setProfileResolutionIfCurrent(agentType, scope, baseRevision string, runtimeGeneration, contextGeneration uint64, key string, resolution ModelConfigResolution) bool {
	m.modelGenerationMu.Lock()
	defer m.modelGenerationMu.Unlock()
	if m.profileGenerations[agentType] != runtimeGeneration || m.profileContextGenerations[baseRevision] != contextGeneration {
		return false
	}
	m.profileFlightMu.Lock()
	defer m.profileFlightMu.Unlock()
	if !m.profileDiscoveryFlightIsCurrentLocked(agentType, scope, baseRevision, runtimeGeneration, contextGeneration) {
		return false
	}
	m.profileCache.setResolution(key, agentType, resolution, time.Now())
	return true
}

func profileProbeStatus(resp *agentctlutil.ProbeResponse) Status {
	if resp == nil {
		return StatusFailed
	}
	switch resp.FailureCode {
	case agentctlutil.ProbeFailureUnsupportedContext:
		return StatusUnsupported
	case agentctlutil.ProbeFailureAuthenticationRequired:
		return StatusAuthRequired
	default:
		if isAuthError(resp.Error) {
			return StatusAuthRequired
		}
		if resp.Success {
			return StatusOK
		}
		return StatusFailed
	}
}

func profileProbeError(resp *agentctlutil.ProbeResponse) string {
	if resp == nil {
		return "profile discovery failed"
	}
	switch profileProbeStatus(resp) {
	case StatusUnsupported:
		return "profile launch settings are unsupported by this agent"
	case StatusAuthRequired:
		return "agent authentication is required"
	default:
		return "profile discovery failed"
	}
}
