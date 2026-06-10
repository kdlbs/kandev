package acp

import (
	"context"
	"errors"
	"fmt"

	"github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	"github.com/kandev/kandev/internal/agentctl/sessionmodel"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"
)

// NewSession creates a new agent session.
func (a *Adapter) NewSession(ctx context.Context, mcpServers []types.McpServer) (string, error) {
	a.mu.Lock()
	conn := a.acpConn
	a.mu.Unlock()

	if conn == nil {
		return "", fmt.Errorf("adapter not initialized")
	}

	// A fresh session invalidates any pending wakeup keyed to the prior
	// session. Reset pendingWakeups and cancel the scheduler under one
	// a.mu critical section so a concurrent handleWakeupEvent can't slip
	// a stale entry between the two operations.
	a.mu.Lock()
	a.pendingWakeups = make(map[string]*pendingWakeup)
	a.wakeup.cancel()
	a.mu.Unlock()

	ctx, span := shared.TraceProtocolRequest(ctx, shared.ProtocolACP, a.agentID, "session.new")
	defer span.End()

	caps := effectiveMcpCapabilities(a.capabilities.McpCapabilities, a.cfg)
	filteredServers := filterMcpServersByCapabilities(mcpServers, caps, a.logger)
	resp, err := conn.NewSession(ctx, acp.NewSessionRequest{
		Cwd:        a.cfg.WorkDir,
		McpServers: toACPMcpServers(filteredServers),
	})
	if err != nil {
		span.RecordError(err)
		if a.maybeEmitAuthRequired(err) {
			return "", fmt.Errorf("authentication required: %w", err)
		}
		return "", fmt.Errorf("failed to create session: %w", err)
	}

	a.mu.Lock()
	a.sessionID = string(resp.SessionId)
	sessionID := a.sessionID
	// Reset session-scoped model caches before computing the new session's
	// state so a session without a model surface can't reuse the previous
	// session's models / configOptions for validation in SetModel.
	a.availableModels = nil
	a.availableConfigOptions = nil
	initialModels := initialSessionModelState(resp.Meta, resp.ConfigOptions, resp.LegacyModels)
	if initialModels != nil {
		a.availableModels = initialModels.AvailableModels
	}
	a.mu.Unlock()
	a.attachMgr.SetSessionID(sessionID)

	span.SetAttributes(attribute.String("session_id", sessionID))
	a.logger.Info("created new session", zap.String("session_id", sessionID))

	// Emit initial session mode if the agent returned mode state
	if resp.Modes != nil {
		a.emitInitialModeState(resp.Modes)
	}

	// Emit session models when the session exposes a model-shaped config option.
	if initialModels != nil {
		a.emitSessionModels(sessionID, initialModels, resp.Meta, resp.ConfigOptions)
	}

	// Emit session status event to normalize with other adapters.
	// This eliminates the need for ReportsStatusViaStream flag.
	a.sendUpdate(AgentEvent{
		Type:          streams.EventTypeSessionStatus,
		SessionID:     sessionID,
		SessionStatus: streams.SessionStatusNew,
		Data: map[string]any{
			"session_status": streams.SessionStatusNew,
			"init":           true,
		},
	})

	return sessionID, nil
}

// initialSessionModelState resolves the initial model state for a session.
// Returns nil when no model-shaped surface exists, signalling that the agent
// doesn't advertise model selection on this session.
//
// Precedence (in order):
//  1. Typed ConfigOptions list with category="model" (v0.13.4+ agents).
//  2. Pre-v0.13.5 top-level `models` field (e.g. auggie 0.29.x), exposed by the
//     kdlbs fork as acp.LegacyModels. Reached even when configOptions carries
//     non-model entries (e.g. `category="mode"`) so an agent that mixes typed
//     mode options with a legacy models block still surfaces its models.
//  3. _meta-only ConfigOption stub for legacy agents that surface options
//     under `_meta.configOptions` (returns an empty state so emitSessionModels
//     still fires; the event's ConfigOptions list is filled from _meta there).
func initialSessionModelState(
	meta map[string]any,
	configOptions []acp.SessionConfigOption,
	legacy *acp.LegacyModels,
) *sessionModelState {
	if state := modelsFromConfigOptions(configOptions); state != nil {
		return state
	}
	if state := modelsFromLegacy(legacy); state != nil {
		return state
	}
	if hasModelConfigOption(extractConfigOptions(meta)) {
		return &sessionModelState{}
	}
	return nil
}

func hasModelConfigOption(options []streams.ConfigOption) bool {
	for _, option := range options {
		if option.ID == configOptionIDModel || option.Category == configOptionIDModel {
			return true
		}
	}
	return false
}

func sessionConfigOptions(meta map[string]any, acpConfigOptions []acp.SessionConfigOption) []streams.ConfigOption {
	configOptions := convertACPConfigOptions(acpConfigOptions)
	if len(configOptions) > 0 {
		return configOptions
	}
	return extractConfigOptions(meta)
}

// effectiveMcpCapabilities applies the adapter's AssumeMcpSse/AssumeMcpHttp
// config overrides on top of the capabilities advertised by the agent during
// the ACP handshake. Some agents (e.g. Auggie) support SSE/HTTP MCP transports
// but don't advertise the corresponding capability, which would otherwise cause
// filterMcpServersByCapabilities to drop user-configured remote MCP servers.
func effectiveMcpCapabilities(caps acp.McpCapabilities, cfg *shared.Config) acp.McpCapabilities {
	if cfg == nil {
		return caps
	}
	if cfg.AssumeMcpSse {
		caps.Sse = true
	}
	if cfg.AssumeMcpHttp {
		caps.Http = true
	}
	return caps
}

// filterMcpServersByCapabilities removes MCP servers that the agent doesn't support.
// Stdio servers are always allowed; SSE/HTTP servers require the corresponding capability.
// If multiple servers share the same name (e.g., dual SSE+HTTP injection), only the first
// surviving entry is kept to prevent duplicate tool registration.
//
//nolint:goconst // "sse"/"http"/"streamable_http" are ACP protocol-type string literals; constants would obscure the type discriminant
func filterMcpServersByCapabilities(servers []types.McpServer, caps acp.McpCapabilities, logger *logger.Logger) []types.McpServer {
	filtered := make([]types.McpServer, 0, len(servers))
	seenNames := make(map[string]bool)
	for _, s := range servers {
		switch s.Type {
		case "sse":
			if !caps.Sse {
				logger.Warn("filtering out SSE MCP server (agent does not support SSE)", zap.String("name", s.Name))
				continue
			}
		case "http", "streamable_http":
			if !caps.Http {
				logger.Warn("filtering out HTTP MCP server (agent does not support HTTP)", zap.String("name", s.Name), zap.String("type", s.Type))
				continue
			}
		}
		// Skip duplicate names - first surviving entry wins
		if seenNames[s.Name] {
			logger.Debug("skipping duplicate MCP server name", zap.String("name", s.Name), zap.String("type", s.Type))
			continue
		}
		seenNames[s.Name] = true
		filtered = append(filtered, s)
	}
	return filtered
}

//nolint:goconst // "sse"/"http"/"streamable_http" are ACP protocol-type string literals; constants would obscure the type discriminant
func toACPMcpServers(servers []types.McpServer) []acp.McpServer {
	if len(servers) == 0 {
		return []acp.McpServer{}
	}
	out := make([]acp.McpServer, 0, len(servers))
	for _, server := range servers {
		switch server.Type {
		case "sse":
			out = append(out, acp.McpServer{
				Sse: &acp.McpServerSseInline{
					Name:    server.Name,
					Url:     server.URL,
					Type:    "sse",
					Headers: mapToHTTPHeaders(server.Headers),
				},
			})
		case "http", "streamable_http":
			out = append(out, acp.McpServer{
				Http: &acp.McpServerHttpInline{
					Name:    server.Name,
					Url:     server.URL,
					Type:    server.Type,
					Headers: mapToHTTPHeaders(server.Headers),
				},
			})
		default: // stdio
			out = append(out, acp.McpServer{
				Stdio: &acp.McpServerStdio{
					Name:    server.Name,
					Command: server.Command,
					Args:    append([]string{}, server.Args...),
					Env:     mapToEnvVars(server.Env),
				},
			})
		}
	}
	return out
}

// mapToEnvVars converts a string map to ACP EnvVariable slice.
// Returns an empty (non-nil) slice when the map is empty to satisfy the ACP SDK's non-omitempty field.
func mapToEnvVars(env map[string]string) []acp.EnvVariable {
	if len(env) == 0 {
		return []acp.EnvVariable{}
	}
	vars := make([]acp.EnvVariable, 0, len(env))
	for k, v := range env {
		vars = append(vars, acp.EnvVariable{Name: k, Value: v})
	}
	return vars
}

// mapToHTTPHeaders converts a string map to ACP HttpHeader slice.
// Returns an empty (non-nil) slice when the map is empty to satisfy the ACP SDK's non-omitempty field.
func mapToHTTPHeaders(headers map[string]string) []acp.HttpHeader {
	if len(headers) == 0 {
		return []acp.HttpHeader{}
	}
	hdrs := make([]acp.HttpHeader, 0, len(headers))
	for k, v := range headers {
		hdrs = append(hdrs, acp.HttpHeader{Name: k, Value: v})
	}
	return hdrs
}

// LoadSession resumes an existing session.
// Returns an error if the agent does not support session loading (LoadSession capability).
// mcpServers are passed to the agent so it can reconnect to MCP servers on the new
// agentctl instance (critical for agents that receive MCP configs via the protocol).
//
//nolint:funlen // pre-existing length preserved from adapter.go file split
func (a *Adapter) LoadSession(ctx context.Context, sessionID string, mcpServers []types.McpServer) error {
	a.mu.Lock()
	conn := a.acpConn
	supportsLoad := a.capabilities.LoadSession
	a.mu.Unlock()

	if conn == nil {
		return fmt.Errorf("adapter not initialized")
	}

	// Check if the agent supports session loading
	if !supportsLoad {
		a.logger.Debug("session/load rejected: agent does not advertise LoadSession capability",
			zap.String("session_id", sessionID))
		return fmt.Errorf("agent does not support session loading (LoadSession capability is false)")
	}

	// Loading a different session invalidates any pending wakeup keyed to the
	// prior session — same reset block as NewSession to avoid leaving an armed
	// timer for a session id that's about to change and accumulating stale
	// pendingWakeups entries across reloads.
	a.mu.Lock()
	a.pendingWakeups = make(map[string]*pendingWakeup)
	a.wakeup.cancel()
	a.mu.Unlock()

	ctx, span := shared.TraceProtocolRequest(ctx, shared.ProtocolACP, a.agentID, "session.load")
	defer span.End()

	// Filter MCP servers by agent capabilities (same logic as NewSession).
	caps := effectiveMcpCapabilities(a.capabilities.McpCapabilities, a.cfg)
	filteredServers := filterMcpServersByCapabilities(mcpServers, caps, a.logger)

	// Suppress history replay notifications during load.
	// ACP session/load replays the entire conversation history asynchronously.
	// We set a flag to suppress these notifications to avoid duplicating messages in the database.
	// The flag will be cleared when we send the next prompt (see Prompt method).
	a.mu.Lock()
	a.isLoadingSession = true
	a.mu.Unlock()

	resp, err := conn.LoadSession(ctx, acp.LoadSessionRequest{
		SessionId:  acp.SessionId(sessionID),
		Cwd:        a.cfg.WorkDir,
		McpServers: toACPMcpServers(filteredServers),
	})

	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to load session: %w", err)
	}

	a.mu.Lock()
	a.sessionID = sessionID
	// Reset session-scoped model caches so a load that lands on a session
	// without a model surface can't reuse the previous session's data.
	a.availableModels = nil
	a.availableConfigOptions = nil
	initialModels := initialSessionModelState(resp.Meta, resp.ConfigOptions, resp.LegacyModels)
	if initialModels != nil {
		a.availableModels = initialModels.AvailableModels
	}
	a.mu.Unlock()
	a.attachMgr.SetSessionID(sessionID)

	span.SetAttributes(attribute.String("session_id", sessionID))
	a.logger.Info("loaded session", zap.String("session_id", sessionID))

	// Emit initial session mode if the agent returned mode state
	if resp.Modes != nil {
		a.emitInitialModeState(resp.Modes)
	}

	// Emit session models if the agent returned model state, or if it exposes
	// model selection only through configOptions.
	if initialModels != nil {
		a.emitSessionModels(sessionID, initialModels, resp.Meta, resp.ConfigOptions)
	}

	// Re-emit plan captured during history replay and clear the loading flag.
	// The ACP SDK guarantees all replay notifications are processed before
	// LoadSession returns (via notificationWg.Wait), so captured state is complete.
	// Clearing isLoadingSession here allows post-replay notifications (e.g.
	// AvailableCommandsUpdate "ready" signals) to pass through normally.
	a.mu.Lock()
	replayPlan := a.loadReplayPlan
	a.loadReplayPlan = nil
	a.isLoadingSession = false
	a.mu.Unlock()

	// Any Monitor still tracked at this point was running in pre-restart history
	// but has no live process to back it now — emit synthetic cancellations so
	// the frontend doesn't render a stuck "watching" card.
	a.sweepMonitorsOnReplayEnd(sessionID)

	if replayPlan != nil {
		entries := make([]PlanEntry, len(replayPlan.Entries))
		for i, e := range replayPlan.Entries {
			entries[i] = PlanEntry{
				Description: e.Content,
				Status:      string(e.Status),
				Priority:    string(e.Priority),
			}
		}
		a.sendUpdate(AgentEvent{
			Type:        streams.EventTypePlan,
			SessionID:   sessionID,
			PlanEntries: entries,
		})
	}

	// Emit session status event to normalize with other adapters.
	// This eliminates the need for ReportsStatusViaStream flag.
	a.sendUpdate(AgentEvent{
		Type:          streams.EventTypeSessionStatus,
		SessionID:     sessionID,
		SessionStatus: streams.SessionStatusResumed,
		Data: map[string]any{
			"session_status": streams.SessionStatusResumed,
			"init":           true,
		},
	})

	return nil
}

// ResetSession creates a new session on the existing connection, effectively resetting
// the agent's conversation context without restarting the subprocess. This is much faster
// than a full process restart since the ACP protocol supports multiple sessions per connection.
func (a *Adapter) ResetSession(ctx context.Context, mcpServers []types.McpServer) (string, error) {
	return a.NewSession(ctx, mcpServers)
}

// emitInitialModeState emits a session_mode event from the session response's Modes field.
// Called after session/new and session/load to provide the initial mode state.
func (a *Adapter) emitInitialModeState(modes *acp.SessionModeState) {
	availModes := make([]streams.SessionModeInfo, 0, len(modes.AvailableModes))
	for _, m := range modes.AvailableModes {
		availModes = append(availModes, streams.SessionModeInfo{
			ID:          string(m.Id),
			Name:        m.Name,
			Description: derefStr(m.Description),
		})
	}
	// Cache available modes so SetMode can include them in subsequent events.
	a.mu.Lock()
	a.availableModes = availModes
	a.mu.Unlock()

	a.sendUpdate(AgentEvent{
		Type:           streams.EventTypeSessionMode,
		SessionID:      a.sessionID,
		CurrentModeID:  string(modes.CurrentModeId),
		AvailableModes: availModes,
	})
}

// emitSessionModels emits a session_models event from the session response.
func (a *Adapter) emitSessionModels(sessionID string, models *sessionModelState, meta map[string]any, acpConfigOptions []acp.SessionConfigOption) {
	currentModelID := models.CurrentModelId
	// Prefer typed config options from the response; fall back to _meta
	// extraction for older agents.
	configOptions := sessionConfigOptions(meta, acpConfigOptions)

	// Fallback: if the SDK didn't parse currentModelId (some agents omit it),
	// take the model-shaped configOption's CurrentValue verbatim. We
	// deliberately do NOT fall back to AvailableModels[0]: agents like auggie
	// return an alphabetically-sorted list whose first entry is a pseudo-agent
	// ("Build Analyzer"), which clobbered the profile model in the UI. When
	// neither CurrentModelId nor a configOption surface a value, emit empty
	// and let the frontend fall through to its profile/snapshot resolution.
	if currentModelID == "" {
		currentModelID = currentModelFromConfig(configOptions)
	}

	// Cache config options so emitSetModelEvent can include them in the
	// convergence event emitted after a successful SetModel call.
	a.mu.Lock()
	a.availableConfigOptions = configOptions
	a.mu.Unlock()

	a.logger.Info("emitting session_models event",
		zap.String("session_id", sessionID),
		zap.String("current_model_id", currentModelID),
		zap.Int("available_models", len(models.AvailableModels)),
	)
	a.sendUpdate(AgentEvent{
		Type:           streams.EventTypeSessionModels,
		SessionID:      sessionID,
		CurrentModelID: currentModelID,
		SessionModels:  convertSessionModels(models.AvailableModels),
		ConfigOptions:  configOptions,
	})
}

// emitSetModelEvent emits a session_models convergence event after SetModel
// applies a new model. The frontend uses this to update its current-model
// view: without it the only session_models event is the one from session/new,
// which carries the agent's (possibly stale or empty) currentModelId.
//
// Callers MUST pass the sessionID and cached state captured under the same
// RLock used to read the connection, so concurrent session switches can't
// route this event to the wrong session. cachedConfig is copied before mutation
// so the model-shaped option's CurrentValue can be rewritten to match modelID
// — this prevents a downstream consumer that reads ConfigOptions[model]
// .CurrentValue (codex-style agents surface the current model there) from
// disagreeing with the CurrentModelID emitted on the same event.
func (a *Adapter) emitSetModelEvent(sessionID, modelID string, cachedModels []modelInfo, cachedConfig []streams.ConfigOption) {
	outConfig := cachedConfig
	if len(cachedConfig) > 0 {
		// Shallow copy: only CurrentValue (a string) is rewritten below, so
		// sharing the inner Options slice with the caller is safe today. If a
		// future caller mutates ConfigOption.Options in place, switch to a
		// deep copy to avoid aliasing the caller's backing array.
		outConfig = make([]streams.ConfigOption, len(cachedConfig))
		copy(outConfig, cachedConfig)
		for i := range outConfig {
			if outConfig[i].ID == configOptionIDModel || outConfig[i].Category == configOptionIDModel {
				outConfig[i].CurrentValue = modelID
			}
		}
		// Refresh the cached config options so a subsequent SetConfigOption
		// (e.g. user toggles reasoning effort after switching model) doesn't
		// reuse the stale model CurrentValue from session/new and clobber the
		// just-applied model in the convergence event.
		a.mu.Lock()
		a.availableConfigOptions = outConfig
		a.mu.Unlock()
	}

	a.logger.Info("emitting session_models convergence event after SetModel",
		zap.String("session_id", sessionID),
		zap.String("model_id", modelID),
	)
	a.sendUpdate(AgentEvent{
		Type:           streams.EventTypeSessionModels,
		SessionID:      sessionID,
		CurrentModelID: modelID,
		SessionModels:  convertSessionModels(cachedModels),
		ConfigOptions:  outConfig,
		UserInitiated:  true,
	})
}

// currentModelFromConfig returns the CurrentValue of the model-shaped
// configOption (matched by well-known ID or Category="model"), or empty
// when none is present.
func currentModelFromConfig(options []streams.ConfigOption) string {
	for _, opt := range options {
		if opt.ID == configOptionIDModel || opt.Category == configOptionIDModel {
			return opt.CurrentValue
		}
	}
	return ""
}

// SetMode changes the agent's session mode via ACP session/set_mode.
func (a *Adapter) SetMode(ctx context.Context, modeID string) error {
	a.mu.RLock()
	conn := a.acpConn
	sessionID := a.sessionID
	a.mu.RUnlock()

	if conn == nil {
		return fmt.Errorf("adapter not initialized")
	}

	_, err := conn.SetSessionMode(ctx, acp.SetSessionModeRequest{
		SessionId: acp.SessionId(sessionID),
		ModeId:    acp.SessionModeId(modeID),
	})
	if err != nil {
		return fmt.Errorf("set session mode failed: %w", err)
	}

	a.mu.RLock()
	cachedModes := a.availableModes
	a.mu.RUnlock()

	a.sendUpdate(AgentEvent{
		Type:           streams.EventTypeSessionMode,
		SessionID:      sessionID,
		CurrentModeID:  modeID,
		AvailableModes: cachedModes,
	})
	return nil
}

// SetModel changes the agent's model via the ACP mechanism advertised by session/new.
// If the model ID doesn't exist in the agent's available models, the call is skipped to avoid 404.
func (a *Adapter) SetModel(ctx context.Context, modelID string) error {
	// Snapshot sessionID + cached state under a single RLock so the
	// convergence event emitted on success is bound to the same session
	// (and the same cached models/options) used to issue the RPC.
	a.mu.RLock()
	conn := a.acpConn
	sessionID := a.sessionID
	available := a.availableModels
	cachedConfig := a.availableConfigOptions
	a.mu.RUnlock()

	if conn == nil {
		return fmt.Errorf("adapter not initialized")
	}

	// Validate model exists in the agent's available models (if known).
	if len(available) > 0 {
		found := false
		for _, m := range available {
			if m.ModelId == modelID {
				found = true
				break
			}
		}
		if !found {
			a.logger.Warn("skipping SetModel: model not in agent's available models",
				zap.String("model_id", modelID),
				zap.Int("available_count", len(available)))
			return nil
		}
	}

	method, err := applySessionModel(ctx, conn, sessionID, modelID, cachedConfig)
	if err != nil {
		return fmt.Errorf("set session model failed via %s: %w", method, err)
	}
	a.finalizeSetModel(method, sessionID, modelID, available, cachedConfig)
	return nil
}

// finalizeSetModel emits the post-apply convergence event when applySessionModel
// actually performed a switch. MethodNone means the agent supports neither the
// typed session/set_config_option nor the legacy session/set_model RPC, so no
// switch happened — skip the reset and the emit to avoid lying to the frontend.
func (a *Adapter) finalizeSetModel(
	method sessionmodel.Method,
	sessionID string,
	modelID string,
	available []modelInfo,
	cachedConfig []streams.ConfigOption,
) {
	if method == sessionmodel.MethodNone {
		return
	}
	a.resetContextWindowMaxSize(sessionID)
	a.emitSetModelEvent(sessionID, modelID, available, cachedConfig)
}

func applySessionModel(
	ctx context.Context,
	conn sessionmodel.SDKConn,
	sessionID string,
	modelID string,
	configOptions []streams.ConfigOption,
) (sessionmodel.Method, error) {
	return sessionmodel.ApplySDK(ctx, conn, sessionmodel.Request{
		SessionID:     sessionID,
		ModelID:       modelID,
		ConfigOptions: sessionmodel.FromStreams(configOptions),
	})
}

// maybeEmitAuthRequired inspects an ACP error and, if it represents an
// AuthenticationRequired (-32000) failure, emits an EventTypeAuthRequired
// carrying the cached auth methods so the frontend can drive the
// authenticate → session/new retry. Returns true when the event was emitted.
//
// The emitted event has no SessionID by design: the failure occurred while
// session/new was attempting to create a session, so no session ID exists
// yet. Consumers that correlate events by session must treat
// EventTypeAuthRequired as a connection-scoped (not session-scoped) signal.
//
// Returns false when no auth methods are cached. Without methods to choose
// from, the frontend can't drive the picker — letting the error fall through
// to the generic "failed to create session" path is more actionable than a
// pseudo-auth-required signal with no options.
func (a *Adapter) maybeEmitAuthRequired(err error) bool {
	var reqErr *acp.RequestError
	if !errors.As(err, &reqErr) || reqErr.Code != -32000 {
		return false
	}

	a.mu.RLock()
	methods := a.availableAuthMethods
	a.mu.RUnlock()

	if len(methods) == 0 {
		return false
	}

	a.sendUpdate(AgentEvent{
		Type:        streams.EventTypeAuthRequired,
		AuthMethods: methods,
		Error:       reqErr.Message,
	})
	return true
}

// SetConfigOption sets a session configuration option via ACP session/set_config_option.
// configID is the option's ID; value is the option-value ID to apply.
//
// On success a session_models convergence event is emitted with the updated
// option's CurrentValue. The orchestrator persists the change to
// AgentProfileSnapshot so model + secondary options (reasoning effort,
// thought level, …) survive page refresh and backend restart. Agents that
// proactively send a ConfigOptionUpdate notification will produce a second,
// equivalent event; downstream persistence is idempotent so duplicates are
// harmless.
func (a *Adapter) SetConfigOption(ctx context.Context, configID, value string) error {
	a.mu.RLock()
	conn := a.acpConn
	sessionID := a.sessionID
	cachedModels := a.availableModels
	cachedConfig := a.availableConfigOptions
	a.mu.RUnlock()

	if conn == nil {
		return fmt.Errorf("adapter not initialized")
	}
	if sessionID == "" {
		return fmt.Errorf("no active session: call NewSession before SetConfigOption")
	}

	_, err := conn.SetSessionConfigOption(ctx, acp.SetSessionConfigOptionRequest{
		ValueId: &acp.SetSessionConfigOptionValueId{
			SessionId: acp.SessionId(sessionID),
			ConfigId:  acp.SessionConfigId(configID),
			Value:     acp.SessionConfigValueId(value),
		},
	})
	if err != nil {
		return fmt.Errorf("set session config option failed: %w", err)
	}
	if isModelConfigID(configID, cachedConfig) {
		a.emitSetModelEvent(sessionID, value, cachedModels, cachedConfig)
	} else {
		a.emitSetConfigOptionEvent(sessionID, configID, value, cachedModels, cachedConfig)
	}
	return nil
}

// emitSetConfigOptionEvent emits a session_models convergence event after a
// non-model SetConfigOption RPC succeeds. The orchestrator persists the
// updated option to AgentProfileSnapshot so it survives page refresh /
// backend restart, mirroring how the model itself is persisted by
// emitSetModelEvent.
func (a *Adapter) emitSetConfigOptionEvent(sessionID, configID, value string, cachedModels []modelInfo, cachedConfig []streams.ConfigOption) {
	outConfig := cachedConfig
	if len(cachedConfig) == 0 {
		// In normal operation session/new populates availableConfigOptions
		// before the frontend can fire a SetConfigOption. Hitting this branch
		// means we accepted the RPC against a cache that was never seeded —
		// log it so it's observable during debugging.
		a.logger.Warn("SetConfigOption succeeded but local config cache is empty; emitted event will carry no options",
			zap.String("session_id", sessionID),
			zap.String("config_id", configID),
		)
	}
	if len(cachedConfig) > 0 {
		outConfig = make([]streams.ConfigOption, len(cachedConfig))
		copy(outConfig, cachedConfig)
		found := false
		for i := range outConfig {
			if outConfig[i].ID == configID {
				outConfig[i].CurrentValue = value
				found = true
			}
		}
		if !found {
			// The agent accepted a configID that wasn't in its own
			// availableConfigOptions list — most likely a stale frontend cache
			// or an agent-side drift between session/new and ConfigOptionUpdate.
			// The event carries the unmutated CurrentValues; the agent's own
			// ConfigOptionUpdate notification will correct the snapshot shortly.
			a.logger.Warn("SetConfigOption: configID not in local cache; convergence event carries stale options",
				zap.String("session_id", sessionID),
				zap.String("config_id", configID),
			)
		}
		// Refresh the cached config options so consecutive SetConfigOption
		// calls (or a follow-up SetModel) read the latest CurrentValues
		// instead of the stale session/new snapshot.
		a.mu.Lock()
		a.availableConfigOptions = outConfig
		a.mu.Unlock()
	}

	a.logger.Info("emitting session_models convergence event after SetConfigOption",
		zap.String("session_id", sessionID),
		zap.String("config_id", configID),
		zap.String("value", value),
	)
	a.sendUpdate(AgentEvent{
		Type:           streams.EventTypeSessionModels,
		SessionID:      sessionID,
		CurrentModelID: currentModelFromConfig(outConfig),
		SessionModels:  convertSessionModels(cachedModels),
		ConfigOptions:  outConfig,
		UserInitiated:  true,
	})
}

// isModelConfigID reports whether configID identifies the model-shaped
// SessionConfigOption — either the well-known "model" ID, or a custom ID that
// the agent tagged with Category="model" in its session config options.
func isModelConfigID(configID string, cachedConfig []streams.ConfigOption) bool {
	if configID == configOptionIDModel {
		return true
	}
	for _, opt := range cachedConfig {
		if opt.ID == configID && opt.Category == configOptionIDModel {
			return true
		}
	}
	return false
}

// Authenticate triggers ACP session/authenticate for a given auth method.
func (a *Adapter) Authenticate(ctx context.Context, methodID string) error {
	a.mu.RLock()
	conn := a.acpConn
	a.mu.RUnlock()

	if conn == nil {
		return fmt.Errorf("adapter not initialized")
	}

	_, err := conn.Authenticate(ctx, acp.AuthenticateRequest{
		MethodId: methodID,
	})
	if err != nil {
		return fmt.Errorf("authenticate failed: %w", err)
	}
	return nil
}
