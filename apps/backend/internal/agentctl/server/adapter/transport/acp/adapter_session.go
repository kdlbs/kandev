package acp

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	"github.com/kandev/kandev/internal/agentctl/sessionmodel"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"
)

const kandevMCPServerName = "kandev"

const sessionTransitionPollInterval = 10 * time.Millisecond

const configChangePollInterval = 10 * time.Millisecond

// PublishesMCPAttachmentResults reports that this adapter emits attachment
// results for the servers that survive its own capability filtering.
func (a *Adapter) PublishesMCPAttachmentResults() bool { return true }

// lockSessionTransition waits for the previous reset's cleanup to finish before
// allowing another session transition. The reset response remains independent
// from session/close, while this gate keeps a later load/reset from overtaking
// the cleanup that still owns the superseded session.
func (a *Adapter) lockSessionTransition(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		if a.sessionTransitionMu.TryLock() {
			a.mu.RLock()
			closed := a.closed
			a.mu.RUnlock()
			if closed {
				a.sessionTransitionMu.Unlock()
				return errors.New("adapter is closed")
			}
			if a.sessionCleanupDone == nil {
				return nil
			}
			if err := ctx.Err(); err != nil {
				a.sessionTransitionMu.Unlock()
				return err
			}
			done := a.sessionCleanupDone
			a.sessionTransitionMu.Unlock()
			select {
			case <-done:
			case <-ctx.Done():
				return ctx.Err()
			}
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		timer := time.NewTimer(sessionTransitionPollInterval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return ctx.Err()
		case <-timer.C:
		}
	}
}

// lockConfigChange serializes full config snapshots without making a canceled
// request wait for another provider RPC to finish.
func (a *Adapter) lockConfigChange(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	ticker := time.NewTicker(configChangePollInterval)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if a.configChangeMu.TryLock() {
			if err := ctx.Err(); err != nil {
				a.configChangeMu.Unlock()
				return err
			}
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (a *Adapter) waitForSessionCleanup() {
	a.sessionTransitionMu.Lock()
	cleanupInFlight := a.sessionCleanupDone != nil
	a.sessionTransitionMu.Unlock()
	if !cleanupInFlight {
		return
	}
	a.sessionCleanupWg.Wait()
}

// GetSessionRestoreCapabilities exposes the negotiated ACP load capability.
// ACP session/load is the supported native restore operation today; resume
// remains an optional protocol extension and is not assumed here.
func (a *Adapter) GetSessionRestoreCapabilities() shared.SessionRestoreCapabilities {
	a.mu.Lock()
	defer a.mu.Unlock()
	return shared.SessionRestoreCapabilities{
		SupportsNativeLoad:    a.capabilities.LoadSession,
		SupportsNativeResume:  false,
		SupportsDirectoryMove: true,
		RequiresNativeState:   true,
	}
}

// NewSession creates a new agent session.
func (a *Adapter) NewSession(ctx context.Context, mcpServers []types.McpServer) (string, error) {
	if err := a.lockSessionTransition(ctx); err != nil {
		return "", err
	}
	defer a.sessionTransitionMu.Unlock()
	return a.newSession(ctx, mcpServers)
}

//nolint:funlen // pre-existing session creation flow retained for transition ordering
func (a *Adapter) newSession(ctx context.Context, mcpServers []types.McpServer) (string, error) {
	// Session replacement and live config changes share one ordering boundary:
	// each ACP response carries a complete config snapshot for its session.
	if err := a.lockConfigChange(ctx); err != nil {
		return "", err
	}
	defer a.configChangeMu.Unlock()
	if err := ctx.Err(); err != nil {
		return "", err
	}

	a.mu.Lock()
	conn := a.acpConn
	a.mu.Unlock()

	if conn == nil {
		return "", fmt.Errorf("adapter not initialized")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	priorPromptTurn := a.currentPromptTurn()

	// A fresh session invalidates any pending wakeup keyed to the prior
	// session. Reset pendingWakeups and cancel the scheduler under one
	// a.mu critical section so a concurrent handleWakeupEvent can't slip
	// a stale entry between the two operations.
	a.mu.Lock()
	a.pendingWakeups = make(map[string]*pendingWakeup)
	a.clearCodexSubagentCorrelationsLocked("")
	a.clearCursorTaskMetaLocked("")
	a.clearPromptHandoffToolTrackingLocked()
	clear(a.usageBySession)
	a.wakeup.cancel()
	a.mu.Unlock()
	a.cancelAllAsyncTurnCompletes()

	ctx, span := shared.TraceProtocolRequest(ctx, shared.ProtocolACP, a.agentID, "session.new")
	defer span.End()

	caps := effectiveMcpCapabilities(a.capabilities.McpCapabilities, a.cfg)
	filteredServers, decisions := filterMcpServersWithDecisions(mcpServers, caps, a.logger)
	for _, decision := range decisions {
		kind := streams.MCPAttachmentEvidenceDelivered
		if !decision.Included {
			kind = streams.MCPAttachmentEvidenceFiltered
		}
		a.emitMCPAttachmentEvidence(ctx, decision.Server, kind, decision.ReasonCode, "")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	resp, err := conn.NewSession(ctx, acp.NewSessionRequest{
		Cwd:        a.cfg.WorkDir,
		McpServers: toACPMcpServers(filteredServers),
	})
	if err != nil {
		for _, server := range filteredServers {
			a.emitMCPAttachmentEvidence(ctx, server, streams.MCPAttachmentEvidenceExplicitError, "session_new_failed", err.Error())
		}
		// An agent may have emitted provisional usage before returning an RPC
		// error. Clear at the FIFO boundary so an already queued notification
		// cannot repopulate the tracker after this failure cleanup.
		if !a.syncNotifQueueThen(a.clearUsageTrackers) {
			a.clearUsageTrackers()
		}
		span.RecordError(err)
		if a.maybeEmitAuthRequired(err) {
			return "", fmt.Errorf("authentication required: %w", err)
		}
		return "", fmt.Errorf("failed to create session: %w", err)
	}
	for _, server := range filteredServers {
		a.emitMCPAttachmentEvidence(ctx, server, streams.MCPAttachmentEvidenceSessionAccepted, "", "")
	}

	sessionID := string(resp.SessionId)
	if !a.syncNotifQueueThen(func() {
		a.mu.Lock()
		a.retainConsumedUsageBaselineLocked(sessionID)
		a.mu.Unlock()
	}) {
		a.clearUsageTrackers()
		barrierErr := context.Cause(a.lifetimeCtx)
		if barrierErr == nil {
			barrierErr = context.Canceled
		}
		return "", fmt.Errorf("failed to synchronize new session notifications: %w", barrierErr)
	}

	initialModels, initialConfigOptions := a.initialSessionConfigState(resp.Meta, resp.ConfigOptions, resp.LegacyModels)
	a.mu.Lock()
	a.sessionID = sessionID
	a.sessionSettingsPolicy = ""
	a.configGeneration++
	clear(a.contextSamples)
	// Reset session-scoped model caches before computing the new session's
	// state so a session without a model surface can't reuse the previous
	// session's models / configOptions for validation in SetModel.
	a.availableModels = nil
	a.availableConfigOptions = cloneConfigOptions(initialConfigOptions)
	a.resetSessionModeLocked()
	if initialModels != nil {
		a.availableModels = initialModels.AvailableModels
	}
	if resp.Modes == nil {
		a.availableModes, _ = sessionModesFromConfig(initialConfigOptions)
	}
	a.mu.Unlock()
	a.invalidatePromptTurnOwnership(priorPromptTurn)
	a.attachMgr.SetSessionID(sessionID)

	span.SetAttributes(attribute.String("session_id", sessionID))
	a.logger.Info("created new session", zap.String("session_id", sessionID))

	// Emit initial session mode if the agent returned mode state
	if resp.Modes != nil {
		a.emitInitialModeState(sessionID, resp.Modes, "")
	} else {
		a.emitInitialModeConfigState(sessionID, initialConfigOptions, "")
	}

	// Emit session models when the session exposes a model-shaped config option.
	if initialModels != nil {
		a.emitSessionModels(sessionID, initialModels, resp.Meta, resp.ConfigOptions, "")
	} else if len(initialConfigOptions) > 0 {
		a.emitSessionConfigOptionsState(sessionID, initialConfigOptions, "")
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

func (a *Adapter) initialSessionConfigState(
	meta map[string]any,
	configOptions []acp.SessionConfigOption,
	legacy *acp.LegacyModels,
) (*sessionModelState, []streams.ConfigOption) {
	models := initialSessionModelState(meta, configOptions, legacy)
	var available []modelInfo
	var currentModelID string
	if models != nil {
		available = models.AvailableModels
		currentModelID = models.CurrentModelId
	}
	return models, a.dialect.sessionConfigOptions(meta, configOptions, available, currentModelID)
}

func hasModelConfigOption(options []streams.ConfigOption) bool {
	for _, option := range options {
		if option.ID == configOptionIDModel || option.Category == configOptionIDModel {
			return true
		}
	}
	return false
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
const (
	mcpFilterReasonSSEUnsupported  = "sse_unsupported"
	mcpFilterReasonHTTPUnsupported = "http_unsupported"
	mcpFilterReasonDuplicateName   = "duplicate_name"
)

type mcpServerFilterDecision struct {
	Server     types.McpServer
	Included   bool
	ReasonCode string
}

func filterMcpServersByCapabilities(servers []types.McpServer, caps acp.McpCapabilities, logger *logger.Logger) []types.McpServer {
	filtered, _ := filterMcpServersWithDecisions(servers, caps, logger)
	return filtered
}

// filterMcpServersWithDecisions preserves the existing first-surviving-server
// selection while retaining an explicit decision for every input server.
func filterMcpServersWithDecisions(
	servers []types.McpServer,
	caps acp.McpCapabilities,
	logger *logger.Logger,
) ([]types.McpServer, []mcpServerFilterDecision) {
	filtered := make([]types.McpServer, 0, len(servers))
	decisions := make([]mcpServerFilterDecision, 0, len(servers))
	seenNames := make(map[string]bool)
	for _, s := range servers {
		decision := mcpServerFilterDecision{Server: s}
		switch s.Type {
		case "sse":
			if !caps.Sse {
				logger.Warn("filtering out SSE MCP server (agent does not support SSE)", zap.String("name", s.Name))
				decision.ReasonCode = mcpFilterReasonSSEUnsupported
				decisions = append(decisions, decision)
				continue
			}
		case "http", "streamable_http":
			if !caps.Http {
				logger.Warn("filtering out HTTP MCP server (agent does not support HTTP)", zap.String("name", s.Name), zap.String("type", s.Type))
				decision.ReasonCode = mcpFilterReasonHTTPUnsupported
				decisions = append(decisions, decision)
				continue
			}
		}
		// Skip duplicate names - first surviving entry wins
		if seenNames[s.Name] {
			logger.Debug("skipping duplicate MCP server name", zap.String("name", s.Name), zap.String("type", s.Type))
			decision.ReasonCode = mcpFilterReasonDuplicateName
			decisions = append(decisions, decision)
			continue
		}
		seenNames[s.Name] = true
		filtered = append(filtered, s)
		decision.Included = true
		decisions = append(decisions, decision)
	}
	return filtered, decisions
}

// emitMCPAttachmentEvidence forwards only safe, backend-attributed attachment
// facts through the adapter's existing update stream. A missing context means
// this call was made outside agentctl's lifecycle boundary, so it deliberately
// produces no ambiguous attribution.
func (a *Adapter) emitMCPAttachmentEvidence(
	ctx context.Context,
	server types.McpServer,
	kind streams.MCPAttachmentEvidenceKind,
	reasonCode string,
	summary string,
) {
	attachmentContext, ok := streams.MCPAttachmentContextFromContext(ctx)
	if !ok {
		return
	}
	target := streams.SanitizeMCPStdioTarget(server.Command)
	if server.Type == "sse" || server.Type == "http" || server.Type == "streamable_http" {
		target = streams.SanitizeMCPNetworkTarget(server.URL)
	}
	source := streams.MCPServerSourceProfile
	if server.Name == kandevMCPServerName {
		source = streams.MCPServerSourceKandev
	}
	a.sendUpdate(AgentEvent{
		Type: streams.EventTypeMCPAttachment,
		MCPAttachment: &streams.MCPAttachmentEvidence{
			AttemptID:  attachmentContext.Attempt.AttemptID,
			ServerName: server.Name,
			Kind:       kind,
			OccurredAt: time.Now().UTC(),
			Source:     source,
			Transport:  server.Type,
			Target:     target,
			ReasonCode: reasonCode,
			Summary:    streams.SanitizeMCPErrorSummary(summary),
		},
	})
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

// LoadSession restores an existing session, preferring advertised session/resume
// without history replay and otherwise using session/load.
// mcpServers are passed to the agent so it can reconnect to MCP servers on the new
// agentctl instance (critical for agents that receive MCP configs via the protocol).
//
//nolint:funlen // pre-existing length preserved from adapter.go file split
func (a *Adapter) LoadSession(ctx context.Context, sessionID string, mcpServers []types.McpServer) error {
	if err := a.lockSessionTransition(ctx); err != nil {
		return err
	}
	defer a.sessionTransitionMu.Unlock()
	if err := a.lockConfigChange(ctx); err != nil {
		return err
	}
	defer a.configChangeMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	settingsPolicy := streams.SessionSettingsPolicyFromContext(ctx)

	a.mu.Lock()
	conn := a.acpConn
	capabilities := a.capabilities
	a.mu.Unlock()

	if conn == nil {
		return fmt.Errorf("adapter not initialized")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	if !capabilities.LoadSession && capabilities.SessionCapabilities.Resume == nil {
		a.logger.Debug("session/load rejected: agent does not advertise LoadSession capability",
			zap.String("session_id", sessionID))
		return &SessionRestoreError{
			Reason: SessionRestoreReasonNativeResumeUnsupported,
			Cause:  fmt.Errorf("agent does not support session loading (LoadSession capability is false)"),
		}
	}
	priorPromptTurn := a.currentPromptTurn()

	// Loading a different session invalidates any pending wakeup or async turn
	// finalizer keyed to the prior session — same reset block as NewSession to
	// avoid leaving an armed timer for a session id that's about to change and
	// accumulating stale pendingWakeups entries across reloads.
	a.mu.Lock()
	a.pendingWakeups = make(map[string]*pendingWakeup)
	a.clearCodexSubagentCorrelationsLocked("")
	a.clearCursorTaskMetaLocked("")
	a.clearPromptHandoffToolTrackingLocked()
	a.wakeup.cancel()
	a.mu.Unlock()
	a.cancelAllAsyncTurnCompletes()

	ctx, span := shared.TraceProtocolRequest(ctx, shared.ProtocolACP, a.agentID, "session.restore")
	defer span.End()

	// Filter MCP servers by agent capabilities (same logic as NewSession).
	caps := effectiveMcpCapabilities(a.capabilities.McpCapabilities, a.cfg)
	filteredServers, decisions := filterMcpServersWithDecisions(mcpServers, caps, a.logger)
	for _, decision := range decisions {
		kind := streams.MCPAttachmentEvidenceDelivered
		if !decision.Included {
			kind = streams.MCPAttachmentEvidenceFiltered
		}
		a.emitMCPAttachmentEvidence(ctx, decision.Server, kind, decision.ReasonCode, "")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	// Suppress history replay notifications during load.
	// ACP session/load replays the entire conversation history asynchronously.
	// We set a flag to suppress these notifications to avoid duplicating messages in the database.
	// The flag will be cleared when we send the next prompt (see Prompt method).
	a.mu.Lock()
	a.isLoadingSession = true
	a.loadReplayPlan = nil
	delete(a.usageBySession, sessionID)
	a.mu.Unlock()

	resp, err := a.restoreSessionState(ctx, conn, capabilities, acp.LoadSessionRequest{
		SessionId:  acp.SessionId(sessionID),
		Cwd:        a.cfg.WorkDir,
		McpServers: toACPMcpServers(filteredServers),
	})

	if err != nil {
		for _, server := range filteredServers {
			a.emitMCPAttachmentEvidence(ctx, server, streams.MCPAttachmentEvidenceExplicitError, "session_load_failed", err.Error())
		}
		// Notifications sent before the failed RPC response may still be queued
		// on the adapter worker. Clear replay suppression only at a FIFO barrier,
		// otherwise those frames can escape as live messages, tools, or plans
		// after LoadSession returns its error.
		clearFailedLoad := func() {
			a.mu.Lock()
			a.isLoadingSession = false
			a.loadReplayPlan = nil
			a.mu.Unlock()
		}
		if !a.syncNotifQueueThen(clearFailedLoad) {
			// Close cancels lifetimeCtx and drops the remaining queue. The
			// barrier callback will not run in that case, so clean up directly.
			clearFailedLoad()
		}
		span.RecordError(err)
		return &SessionRestoreError{Reason: sessionRestoreReason(err), Cause: err}
	}
	for _, server := range filteredServers {
		a.emitMCPAttachmentEvidence(ctx, server, streams.MCPAttachmentEvidenceSessionAccepted, "", "")
	}

	initialModels, initialConfigOptions := a.initialSessionConfigState(resp.Meta, resp.ConfigOptions, resp.LegacyModels)

	// The SDK may finish the load RPC while replay notifications are still
	// queued in the adapter worker. Keep suppression active until a FIFO barrier
	// proves every replay frame has been processed, then mark replayed cumulative
	// usage/cost as the baseline for the first new prompt.
	a.syncNotifQueue()

	a.mu.Lock()
	a.sessionID = sessionID
	a.sessionSettingsPolicy = providerRestoredPolicy(settingsPolicy)
	a.configGeneration++
	clear(a.contextSamples)
	a.consumeUsageBaselineLocked(sessionID)
	replayPlan := a.loadReplayPlan
	a.loadReplayPlan = nil
	a.isLoadingSession = false
	// Reset session-scoped model caches so a load that lands on a session
	// without a model surface can't reuse the previous session's data.
	a.availableModels = nil
	a.availableConfigOptions = cloneConfigOptions(initialConfigOptions)
	a.resetSessionModeLocked()
	if initialModels != nil {
		a.availableModels = initialModels.AvailableModels
	}
	if resp.Modes == nil {
		a.availableModes, _ = sessionModesFromConfig(initialConfigOptions)
	}
	a.mu.Unlock()
	a.invalidatePromptTurnOwnership(priorPromptTurn)
	a.attachMgr.SetSessionID(sessionID)

	span.SetAttributes(attribute.String("session_id", sessionID))
	a.logger.Info("loaded session", zap.String("session_id", sessionID))

	// Emit initial session mode if the agent returned mode state
	if resp.Modes != nil {
		a.emitInitialModeState(sessionID, resp.Modes, settingsPolicy)
	} else if _, hasModeOption := modeConfigOption(initialConfigOptions); hasModeOption {
		a.emitInitialModeConfigState(sessionID, initialConfigOptions, settingsPolicy)
	} else if providerRestoredPolicy(settingsPolicy) == streams.SessionSettingsPolicyProviderRestored {
		a.emitEmptyInitialModeState(sessionID, settingsPolicy)
	}

	// Emit session models if the agent returned model state, or if it exposes
	// model selection only through configOptions.
	if initialModels != nil {
		a.emitSessionModels(sessionID, initialModels, resp.Meta, resp.ConfigOptions, settingsPolicy)
	} else if len(initialConfigOptions) > 0 {
		a.emitSessionConfigOptionsState(sessionID, initialConfigOptions, settingsPolicy)
	} else if providerRestoredPolicy(settingsPolicy) == streams.SessionSettingsPolicyProviderRestored {
		a.emitEmptyInitialModelsState(sessionID, settingsPolicy)
	}

	// Any Monitor still tracked at this point was running in pre-restart history
	// but has no live process to back it now — emit synthetic cancellations so
	// the frontend doesn't render a stuck "watching" card.
	a.sweepMonitorsOnReplayEnd(sessionID)

	a.emitReplayPlan(sessionID, replayPlan)

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
//
// The session/new call spawns a fresh agent-side child process without releasing the
// superseded one, so a successful reset closes the outgoing session to release its
// resources. The old session is captured before NewSession overwrites a.sessionID.
func (a *Adapter) ResetSession(ctx context.Context, mcpServers []types.McpServer) (string, error) {
	if err := a.lockSessionTransition(ctx); err != nil {
		return "", err
	}

	a.mu.RLock()
	previous, conn := a.sessionID, a.acpConn
	a.mu.RUnlock()

	newID, err := a.newSession(ctx, mcpServers)
	if err != nil {
		a.sessionTransitionMu.Unlock()
		return "", err
	}
	// session/new has committed the replacement session. Superseded-session
	// cleanup is best effort and has its own bounded context, so it must not
	// hold the reset request open while a provider ignores session/close. The
	// cleanup remains queued behind the transition mutex so another session
	// transition cannot make the still-current check stale.
	if previous != "" && previous != newID && conn != nil {
		done := make(chan struct{})
		a.sessionCleanupDone = done
		a.sessionCleanupWg.Add(1)
		go a.runSupersededSessionCleanup(done, conn, previous, newID)
	}
	a.sessionTransitionMu.Unlock()
	return newID, nil
}

func (a *Adapter) runSupersededSessionCleanup(
	done chan struct{}, conn *acp.ClientSideConnection, previous, newID string,
) {
	defer a.sessionCleanupWg.Done()
	a.sessionTransitionMu.Lock()
	defer a.sessionTransitionMu.Unlock()
	a.closeSupersededSessionLocked(context.Background(), conn, previous, newID)
	close(done)
	a.sessionCleanupDone = nil
}

// closeSupersededSessionTimeout bounds session/close after a reset so cleanup
// can't hang the caller when the agent doesn't respond.
const closeSupersededSessionTimeout = 10 * time.Second

// closeSupersededSession releases the session a successful reset just replaced.
// It fails closed: absent capability, an empty or unchanged previous id, a nil
// connection, or a.sessionID no longer matching newID all skip the close
// silently rather than risk closing the live session. The last case guards a
// concurrent session transition: WS requests are dispatched to the adapter
// without serialization, so a LoadSession or another ResetSession can replace
// newID with a different session between NewSession returning and this check
// running. A close error is logged and never surfaces to the caller, since the
// reset itself already succeeded.
func (a *Adapter) closeSupersededSession(ctx context.Context, conn *acp.ClientSideConnection, previous, newID string) {
	if err := a.lockSessionTransition(ctx); err != nil {
		return
	}
	defer a.sessionTransitionMu.Unlock()
	a.closeSupersededSessionLocked(ctx, conn, previous, newID)
}

// closeSupersededSessionLocked requires sessionTransitionMu to be held so the
// still-current check and session/close request form one transition.
func (a *Adapter) closeSupersededSessionLocked(_ context.Context, conn *acp.ClientSideConnection, previous, newID string) {
	if previous == "" || previous == newID || conn == nil {
		return
	}
	a.mu.RLock()
	supportsClose := a.capabilities.SessionCapabilities.Close != nil
	stillCurrent := a.sessionID == newID
	a.mu.RUnlock()
	if !supportsClose || !stillCurrent {
		return
	}

	cleanupBase := a.lifetimeCtx
	if cleanupBase == nil {
		cleanupBase = context.Background()
	}
	closeCtx, cancel := context.WithTimeout(context.WithoutCancel(cleanupBase), closeSupersededSessionTimeout)
	defer cancel()
	if _, err := conn.CloseSession(closeCtx, acp.CloseSessionRequest{
		SessionId: acp.SessionId(previous),
	}); err != nil {
		a.logger.Warn("failed to close superseded session after reset",
			zap.String("session_id", previous), zap.Error(err))
	}
}

// emitReplayPlan re-emits the plan captured during session/load replay so the todo
// indicator survives a resume. An empty plan is dropped: on the ACP ingest path an
// empty entry list carries no information (same reasoning as extractTodoItems), and
// re-emitting it would both wipe the live indicator and persist an empty todo snapshot
// that shadows the last real one in the chat timeline. A genuine "agent cleared the
// todos" still arrives as a live plan update and keeps its persisted empty snapshot.
func (a *Adapter) emitReplayPlan(sessionID string, replayPlan *acp.SessionUpdatePlan) {
	if replayPlan == nil || len(replayPlan.Entries) == 0 {
		return
	}
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

// emitInitialModeState emits a session_mode event from the session response's Modes field.
// Called after session/new and session/load to provide the initial mode state.
func (a *Adapter) nextSessionSettingsGeneration(sessionID string) uint64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.sessionID != sessionID || a.closed {
		return 0
	}
	a.sessionSettingsGeneration++
	return a.sessionSettingsGeneration
}

func (a *Adapter) emitInitialModeState(sessionID string, modes *acp.SessionModeState, settingsPolicies ...streams.SessionSettingsPolicy) {
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
	a.noteCurrentMode(sessionID, string(modes.CurrentModeId))

	a.sendUpdate(AgentEvent{
		Type:                      streams.EventTypeSessionMode,
		SessionID:                 sessionID,
		CurrentModeID:             string(modes.CurrentModeId),
		AvailableModes:            availModes,
		SessionSettingsPolicy:     providerRestoredPolicy(settingsPolicies...),
		SessionSettingsGeneration: a.nextSessionSettingsGeneration(sessionID),
	})
}

func (a *Adapter) emitInitialModeConfigState(sessionID string, configOptions []streams.ConfigOption, settingsPolicies ...streams.SessionSettingsPolicy) {
	modeOption, ok := modeConfigOption(configOptions)
	if !ok {
		return
	}
	availableModes := sessionModesFromConfigOption(modeOption)
	a.mu.Lock()
	if a.sessionID != sessionID || a.closed {
		a.mu.Unlock()
		return
	}
	a.availableModes = availableModes
	a.mu.Unlock()
	if modeOption.CurrentValue != "" {
		a.noteCurrentMode(sessionID, modeOption.CurrentValue)
	}
	a.sendUpdate(AgentEvent{
		Type:                      streams.EventTypeSessionMode,
		SessionID:                 sessionID,
		CurrentModeID:             modeOption.CurrentValue,
		AvailableModes:            availableModes,
		SessionSettingsPolicy:     providerRestoredPolicy(settingsPolicies...),
		SessionSettingsGeneration: a.nextSessionSettingsGeneration(sessionID),
	})
}

func (a *Adapter) emitEmptyInitialModeState(sessionID string, settingsPolicies ...streams.SessionSettingsPolicy) {
	a.sendUpdate(AgentEvent{
		Type:                      streams.EventTypeSessionMode,
		SessionID:                 sessionID,
		SessionSettingsPolicy:     providerRestoredPolicy(settingsPolicies...),
		SessionSettingsGeneration: a.nextSessionSettingsGeneration(sessionID),
	})
}

func (a *Adapter) emitEmptyInitialModelsState(sessionID string, settingsPolicies ...streams.SessionSettingsPolicy) {
	a.sendUpdate(AgentEvent{
		Type:                      streams.EventTypeSessionModels,
		SessionID:                 sessionID,
		SessionSettingsPolicy:     providerRestoredPolicy(settingsPolicies...),
		SessionSettingsGeneration: a.nextSessionSettingsGeneration(sessionID),
		Data:                      map[string]any{"config_options_settled": true},
	})
}

func sessionModesFromConfig(options []streams.ConfigOption) ([]streams.SessionModeInfo, bool) {
	modeOption, ok := modeConfigOption(options)
	if !ok {
		return nil, false
	}
	return sessionModesFromConfigOption(modeOption), true
}

func sessionModesFromConfigOption(option streams.ConfigOption) []streams.SessionModeInfo {
	modes := make([]streams.SessionModeInfo, 0, len(option.Options))
	for _, value := range option.Options {
		modes = append(modes, streams.SessionModeInfo{
			ID:          value.Value,
			Name:        value.Name,
			Description: value.Description,
		})
	}
	return modes
}

// emitSessionModels emits a session_models event from the session response.
func (a *Adapter) emitSessionModels(sessionID string, models *sessionModelState, meta map[string]any, acpConfigOptions []acp.SessionConfigOption, settingsPolicies ...streams.SessionSettingsPolicy) {
	currentModelID := models.CurrentModelId
	configOptions := a.dialect.sessionConfigOptions(
		meta, acpConfigOptions, models.AvailableModels, currentModelID,
	)

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
		Type:                      streams.EventTypeSessionModels,
		SessionID:                 sessionID,
		CurrentModelID:            currentModelID,
		SessionModels:             convertSessionModels(models.AvailableModels),
		ConfigOptions:             configOptions,
		SessionSettingsPolicy:     providerRestoredPolicy(settingsPolicies...),
		SessionSettingsGeneration: a.nextSessionSettingsGeneration(sessionID),
	})
}

func (a *Adapter) emitSessionConfigOptionsState(sessionID string, configOptions []streams.ConfigOption, settingsPolicies ...streams.SessionSettingsPolicy) {
	a.sendUpdate(AgentEvent{
		Type:                      streams.EventTypeSessionModels,
		SessionID:                 sessionID,
		ConfigOptions:             cloneConfigOptions(configOptions),
		SessionSettingsPolicy:     providerRestoredPolicy(settingsPolicies...),
		SessionSettingsGeneration: a.nextSessionSettingsGeneration(sessionID),
	})
}

func providerRestoredPolicy(policies ...streams.SessionSettingsPolicy) streams.SessionSettingsPolicy {
	if len(policies) > 0 && policies[0] == streams.SessionSettingsPolicyProviderRestored {
		return policies[0]
	}
	return ""
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
func (a *Adapter) emitSetModelEvent(
	sessionID string,
	modelID string,
	cachedModels []modelInfo,
	cachedConfig []streams.ConfigOption,
	expectedGeneration ...uint64,
) {
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
	}
	outConfig = a.dialect.modelConfigAfterChange(outConfig, cachedModels, modelID)
	a.mu.Lock()
	if a.sessionID != sessionID ||
		(len(expectedGeneration) > 0 && a.configGeneration != expectedGeneration[0]) {
		a.mu.Unlock()
		return
	}
	if len(outConfig) > 0 {
		// Refresh the cached config options so a subsequent SetConfigOption
		// (e.g. user toggles reasoning effort after switching model) doesn't
		// reuse the stale model CurrentValue from session/new and clobber the
		// just-applied model in the convergence event.
		a.availableConfigOptions = outConfig
	}
	if tracker := a.usageBySession[sessionID]; tracker != nil {
		tracker.maxSize = 0
	}
	delete(a.contextSamples, sessionID)
	a.sessionSettingsGeneration++
	event := AgentEvent{
		Type:                      streams.EventTypeSessionModels,
		SessionID:                 sessionID,
		CurrentModelID:            modelID,
		SessionModels:             convertSessionModels(cachedModels),
		ConfigOptions:             outConfig,
		SessionSettingsGeneration: a.sessionSettingsGeneration,
	}
	a.mu.Unlock()

	a.logger.Info("emitting session_models convergence event after SetModel",
		zap.String("session_id", sessionID),
		zap.String("model_id", modelID),
	)
	// COVERED site (AC-EXECUTORS-SURVIVAL-001.5/.6): sendUpdate blocks rather
	// than drops on a full updatesCh and must run without a.mu held -- see
	// emitDialectContextWindow's doc comment for the deadlock this avoids.
	a.sendUpdate(event)
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

func currentModeFromConfig(options []streams.ConfigOption) string {
	for _, option := range options {
		if isModeConfigOption(option) {
			return option.CurrentValue
		}
	}
	return ""
}

// SetMode changes the agent's session mode via ACP session/set_mode and
// reports what the agent actually ended up in.
//
// The emitted event carries the agent's reported mode, not the requested one.
// Echoing the request made a clamped or ignored mode look identical to an
// applied one.
func (a *Adapter) SetMode(ctx context.Context, modeID string) (streams.ModeResult, error) {
	return a.setSessionMode(ctx, modeID, "")
}

// setSessionMode applies a permission mode through the advertised mode config
// option when available, then falls back to advertised legacy modes only when
// the option method itself is not implemented.
func (a *Adapter) setSessionMode(ctx context.Context, modeID, requestedConfigID string) (streams.ModeResult, error) {
	if err := a.lockModeChange(ctx); err != nil {
		return streams.ModeResult{Requested: modeID}, err
	}
	defer a.modeChangeMu.Unlock()
	if err := a.lockConfigChange(ctx); err != nil {
		return streams.ModeResult{Requested: modeID}, err
	}
	defer a.configChangeMu.Unlock()
	if err := ctx.Err(); err != nil {
		return streams.ModeResult{Requested: modeID}, err
	}

	a.mu.RLock()
	conn := a.acpConn
	sessionID := a.sessionID
	closed := a.closed
	availableModes := append([]streams.SessionModeInfo(nil), a.availableModes...)
	cachedModels := append([]modelInfo(nil), a.availableModels...)
	cachedConfig := cloneConfigOptions(a.availableConfigOptions)
	a.mu.RUnlock()

	if conn == nil {
		return streams.ModeResult{Requested: modeID}, fmt.Errorf("adapter not initialized")
	}
	if closed {
		return streams.ModeResult{Requested: modeID}, errors.New("adapter is closed")
	}
	if sessionID == "" {
		return streams.ModeResult{Requested: modeID}, fmt.Errorf("no active session: call NewSession before SetMode")
	}
	modeOption, hasModeOption, err := selectSessionModeOption(cachedConfig, availableModes, modeID, requestedConfigID)
	if err != nil {
		return streams.ModeResult{Requested: modeID}, err
	}
	if err := ctx.Err(); err != nil {
		return streams.ModeResult{Requested: modeID}, err
	}
	if !hasModeOption {
		settingsGeneration, satisfied, err := a.alreadySatisfiedLegacyMode(ctx, conn, sessionID, modeID)
		if err != nil {
			return streams.ModeResult{Requested: modeID}, err
		}
		if satisfied {
			result := streams.ModeResult{Requested: modeID, Effective: modeID, Confirmed: true}
			a.emitSessionModeResult(sessionID, modeID, result, settingsGeneration)
			return result, nil
		}
	}
	generation := a.beginConfigChange()
	baseline, uncertain := a.beginModeChange()
	modeRPCAttempted := false
	unconfirmed := true
	defer func() { a.endModeChange(unconfirmed && modeRPCAttempted) }()

	request := sessionModeRequest{
		conn: conn, sessionID: sessionID, modeID: modeID, option: modeOption,
		legacyModes: availableModes, models: cachedModels, generation: generation,
		baseline: baseline, uncertain: uncertain,
	}
	var result streams.ModeResult
	if hasModeOption {
		result, modeRPCAttempted, err = a.setConfigSessionMode(ctx, request)
	} else {
		result, modeRPCAttempted, err = a.setLegacySessionMode(ctx, conn, sessionID, modeID, baseline, uncertain)
	}
	if err != nil {
		return result, err
	}
	if result.Confirmed {
		unconfirmed = false
	}
	a.emitSessionModeResult(sessionID, modeID, result, 0)
	return result, nil
}

func (a *Adapter) isActiveSession(sessionID string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.sessionID == sessionID && !a.closed
}

func (a *Adapter) setLegacySessionMode(
	ctx context.Context,
	conn *acp.ClientSideConnection,
	sessionID, modeID string,
	afterGeneration uint64,
	uncertain bool,
) (streams.ModeResult, bool, error) {
	if err := ctx.Err(); err != nil {
		return streams.ModeResult{Requested: modeID}, false, err
	}
	_, err := conn.SetSessionMode(ctx, acp.SetSessionModeRequest{
		SessionId: acp.SessionId(sessionID),
		ModeId:    acp.SessionModeId(modeID),
	})
	if err != nil {
		return streams.ModeResult{Requested: modeID}, true, fmt.Errorf("set session mode failed: %w", err)
	}
	result := a.awaitModeSettle(ctx, sessionID, modeID, afterGeneration)
	if uncertain {
		return streams.ModeResult{Requested: modeID}, true, nil
	}
	return result, true, nil
}

func isModeConfigOption(option streams.ConfigOption) bool {
	return option.Type == "select" && option.Category == string(acp.SessionConfigOptionCategoryMode)
}

func modeConfigOption(options []streams.ConfigOption) (streams.ConfigOption, bool) {
	for _, option := range options {
		if isModeConfigOption(option) {
			return option, true
		}
	}
	return streams.ConfigOption{}, false
}

func isModeConfigOptionID(options []streams.ConfigOption, configID string) bool {
	for _, option := range options {
		if option.ID == configID && isModeConfigOption(option) {
			return true
		}
	}
	return false
}

func configOptionAdvertisesValue(option streams.ConfigOption, value string) bool {
	for _, candidate := range option.Options {
		if candidate.Value == value {
			return true
		}
	}
	return false
}

func legacyModesAdvertise(modes []streams.SessionModeInfo, modeID string) bool {
	for _, mode := range modes {
		if mode.ID == modeID {
			return true
		}
	}
	return false
}

// sessionModeEventFields decides what a session-mode event reports after the
// agent answered session/set_mode.
//
// Under ACP a successful answer means the mode changed, so only an observed
// report can contradict it. Silence within the settle window leaves the
// requested mode standing; the uncertainty belongs in ModeResult.Confirmed,
// not in a current mode that would overwrite the caller's choice on persist.
// An observed report that differs is a clamp, and carries the request
// alongside it so the mismatch stays visible.
func sessionModeEventFields(requested string, result streams.ModeResult) (currentModeID, requestedModeID string) {
	if !result.Confirmed || result.Effective == "" {
		return "", requested
	}
	if result.Effective == requested {
		return requested, ""
	}
	return result.Effective, requested
}

// SetModel changes the agent's model via the ACP mechanism advertised by session/new.
// If the model ID doesn't exist in the agent's available models, the call
// fails before sending an RPC so callers do not wait for convergence that can
// never arrive.
func (a *Adapter) SetModel(ctx context.Context, modelID string) error {
	// Bind the request to this session. Read its config after acquiring the
	// shared config gate so earlier changes have finished.
	a.mu.RLock()
	conn := a.acpConn
	sessionID := a.sessionID
	a.mu.RUnlock()

	if conn == nil {
		return fmt.Errorf("adapter not initialized")
	}
	return a.setModelWithConn(ctx, conn, sessionID, modelID)
}

func (a *Adapter) setModelWithConn(
	ctx context.Context,
	conn sessionmodel.SDKConn,
	sessionID string,
	modelID string,
) error {
	if err := a.lockConfigChange(ctx); err != nil {
		return err
	}
	defer a.configChangeMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	a.mu.RLock()
	if a.sessionID != sessionID {
		a.mu.RUnlock()
		return nil
	}
	available := append([]modelInfo(nil), a.availableModels...)
	cachedConfig := cloneConfigOptions(a.availableConfigOptions)
	a.mu.RUnlock()
	generation := a.beginConfigChange()
	if !a.isCurrentConfigChange(sessionID, generation) {
		return nil
	}

	rpc, err := a.dialect.modelRequest(dialectConfigChange{
		sessionID: sessionID,
		value:     modelID,
		models:    available,
		config:    cachedConfig,
	})
	if err != nil {
		return err
	}
	if rpc != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
		_, err = conn.UnstableSetSessionModel(ctx, rpc.request)
		if !a.isCurrentConfigChange(sessionID, generation) {
			return nil
		}
		if err != nil {
			return rpc.formatError(err)
		}
		a.finalizeSetModel(
			sessionmodel.MethodSetModel,
			sessionID,
			modelID,
			available,
			cachedConfig,
			configOptionIDModel,
			nil,
			generation,
		)
		return nil
	}

	// Validate model exists in the agent's available models (if known).
	if len(available) > 0 {
		if err := validateAvailableModel(available, modelID); err != nil {
			return err
		}
	}

	if err := ctx.Err(); err != nil {
		return err
	}
	method, responseConfig, configID, err := applySessionModelWithConfigOptions(ctx, conn, sessionID, modelID, cachedConfig)
	if !a.isCurrentConfigChange(sessionID, generation) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("set session model failed via %s: %w", method, err)
	}
	a.finalizeSetModel(method, sessionID, modelID, available, cachedConfig, configID, responseConfig, generation)
	return nil
}

func validateAvailableModel(available []modelInfo, modelID string) error {
	for _, model := range available {
		if model.ModelId == modelID {
			return nil
		}
	}
	return fmt.Errorf("model %q is not in the agent's %d available models", modelID, len(available))
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
	configID string,
	responseConfig []acp.SessionConfigOption,
	expectedGeneration ...uint64,
) {
	if method == sessionmodel.MethodNone {
		return
	}
	if len(responseConfig) > 0 {
		a.emitAuthoritativeConfigOptions(sessionID, configID, responseConfig, available, false, expectedGeneration...)
		return
	}
	a.emitSetModelEvent(sessionID, modelID, available, cachedConfig, expectedGeneration...)
}

func (a *Adapter) beginConfigChange() uint64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.configGeneration++
	return a.configGeneration
}

func (a *Adapter) isCurrentConfigChange(sessionID string, generation uint64) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.configChangeIsCurrentLocked(sessionID, generation)
}

func (a *Adapter) configChangeIsCurrentLocked(sessionID string, generation uint64) bool {
	return a.sessionID == sessionID && a.configGeneration == generation
}

func applySessionModel(
	ctx context.Context,
	conn sessionmodel.SDKConn,
	sessionID string,
	modelID string,
	configOptions []streams.ConfigOption,
) (sessionmodel.Method, error) {
	method, _, _, err := applySessionModelWithConfigOptions(ctx, conn, sessionID, modelID, configOptions)
	return method, err
}

func applySessionModelWithConfigOptions(
	ctx context.Context,
	conn sessionmodel.SDKConn,
	sessionID string,
	modelID string,
	configOptions []streams.ConfigOption,
) (sessionmodel.Method, []acp.SessionConfigOption, string, error) {
	request := sessionmodel.Request{
		SessionID:     sessionID,
		ModelID:       modelID,
		ConfigOptions: sessionmodel.FromStreams(configOptions),
	}
	method, responseConfig, err := sessionmodel.ApplySDKWithConfigOptions(ctx, conn, request)
	return method, responseConfig, modelConfigOptionID(configOptions), err
}

func modelConfigOptionID(options []streams.ConfigOption) string {
	for _, option := range options {
		if option.ID == configOptionIDModel || option.Category == configOptionIDModel {
			return option.ID
		}
	}
	return configOptionIDModel
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
	settingsPolicy := streams.SessionSettingsPolicyFromContext(ctx)
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
	if isModeConfigOptionID(cachedConfig, configID) {
		result, err := a.setSessionMode(ctx, value, configID)
		if err != nil {
			return err
		}
		if !result.Confirmed {
			return fmt.Errorf("requested mode %q was not confirmed by the agent", value)
		}
		return nil
	}

	if err := a.lockConfigChange(ctx); err != nil {
		return err
	}
	defer a.configChangeMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	a.mu.RLock()
	if a.sessionID != sessionID {
		a.mu.RUnlock()
		return nil
	}
	cachedModels = append([]modelInfo(nil), a.availableModels...)
	cachedConfig = cloneConfigOptions(a.availableConfigOptions)
	a.mu.RUnlock()
	generation := a.beginConfigChange()
	if !a.isCurrentConfigChange(sessionID, generation) {
		return nil
	}

	rpc, err := a.dialect.configRequest(dialectConfigChange{
		sessionID: sessionID,
		configID:  configID,
		value:     value,
		models:    cachedModels,
		config:    cachedConfig,
	})
	if err != nil {
		return err
	}
	if rpc != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
		_, err = conn.UnstableSetSessionModel(ctx, rpc.request)
		if !a.isCurrentConfigChange(sessionID, generation) {
			return nil
		}
		if err != nil {
			return rpc.formatError(err)
		}
		if isModelConfigID(configID, cachedConfig) {
			a.finalizeSetModel(
				sessionmodel.MethodSetModel,
				sessionID,
				value,
				cachedModels,
				cachedConfig,
				configID,
				nil,
				generation,
			)
		} else {
			a.emitSetConfigOptionEventWithPolicy(sessionID, configID, value, cachedModels, cachedConfig, settingsPolicy, generation)
		}
		return nil
	}

	if err := ctx.Err(); err != nil {
		return err
	}
	resp, err := conn.SetSessionConfigOption(ctx, acp.SetSessionConfigOptionRequest{
		ValueId: &acp.SetSessionConfigOptionValueId{
			SessionId: acp.SessionId(sessionID),
			ConfigId:  acp.SessionConfigId(configID),
			Value:     acp.SessionConfigValueId(value),
		},
	})
	if !a.isCurrentConfigChange(sessionID, generation) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("set session config option failed: %w", err)
	}
	a.mu.RLock()
	sessionActive := a.sessionID == sessionID
	a.mu.RUnlock()
	if !sessionActive {
		return nil
	}
	if len(resp.ConfigOptions) > 0 {
		a.emitAuthoritativeConfigOptionsWithPolicy(sessionID, configID, resp.ConfigOptions, cachedModels, false, settingsPolicy, generation)
		return nil
	}
	if isModelConfigID(configID, cachedConfig) {
		a.emitSetModelEvent(sessionID, value, cachedModels, cachedConfig, generation)
	} else {
		a.emitSetConfigOptionEventWithPolicy(sessionID, configID, value, cachedModels, cachedConfig, settingsPolicy, generation)
	}
	return nil
}

func (a *Adapter) emitAuthoritativeConfigOptions(
	sessionID string,
	configID string,
	options []acp.SessionConfigOption,
	cachedModels []modelInfo,
	correlatedModeResponse bool,
	expectedGeneration ...uint64,
) {
	a.emitAuthoritativeConfigOptionsWithPolicy(
		sessionID, configID, options, cachedModels, correlatedModeResponse, streams.SessionSettingsPolicyStrict, expectedGeneration...,
	)
}

func (a *Adapter) emitAuthoritativeConfigOptionsWithPolicy(
	sessionID string,
	configID string,
	options []acp.SessionConfigOption,
	cachedModels []modelInfo,
	correlatedModeResponse bool,
	settingsPolicy streams.SessionSettingsPolicy,
	expectedGeneration ...uint64,
) {
	configOptions := convertACPConfigOptions(options)
	event := AgentEvent{
		Type:                  streams.EventTypeSessionModels,
		SessionID:             sessionID,
		SessionSettingsPolicy: providerRestoredPolicy(settingsPolicy),
		CurrentModelID:        currentModelFromConfig(configOptions),
		SessionModels:         convertSessionModels(cachedModels),
		ConfigOptions:         configOptions,
		Data: map[string]any{
			"config_options_source":    "provider_response",
			"config_options_config_id": configID,
		},
	}
	a.mu.Lock()
	if len(expectedGeneration) > 0 && !a.configChangeIsCurrentLocked(sessionID, expectedGeneration[0]) {
		a.mu.Unlock()
		return
	}
	if a.sessionID != sessionID || a.closed {
		a.mu.Unlock()
		return
	}
	a.availableConfigOptions = configOptions
	if modes, found := sessionModesFromConfig(configOptions); found {
		a.availableModes = modes
	}
	modeEvent := (*AgentEvent)(nil)
	if _, hasModeOption := modeConfigOption(configOptions); hasModeOption {
		if currentMode := currentModeFromConfig(configOptions); currentMode != "" {
			a.noteConfigModeSnapshotLocked(sessionID, currentMode, correlatedModeResponse)
		}
		if !correlatedModeResponse {
			modeEvent = &AgentEvent{
				Type:                  streams.EventTypeSessionMode,
				SessionID:             sessionID,
				SessionSettingsPolicy: providerRestoredPolicy(settingsPolicy),
				CurrentModeID:         currentModeFromConfig(configOptions),
				AvailableModes:        append([]streams.SessionModeInfo(nil), a.availableModes...),
			}
		}
	}
	a.sessionSettingsGeneration++
	event.SessionSettingsGeneration = a.sessionSettingsGeneration
	if modeEvent != nil {
		modeEvent.SessionSettingsGeneration = a.sessionSettingsGeneration
	}
	if isModelConfigID(configID, configOptions) {
		if tracker := a.usageBySession[sessionID]; tracker != nil {
			tracker.maxSize = 0
		}
		delete(a.contextSamples, sessionID)
	}
	a.mu.Unlock()
	if modeEvent != nil {
		a.sendUpdate(*modeEvent)
	}
	// COVERED site (AC-EXECUTORS-SURVIVAL-001.5/.6): sendUpdate blocks rather
	// than drops on a full updatesCh and must run without a.mu held -- see
	// emitDialectContextWindow's doc comment for the deadlock this avoids.
	a.sendUpdate(event)
}

// emitSetConfigOptionEvent emits a session_models convergence event after a
// non-model SetConfigOption RPC succeeds. The frontend uses this to keep the
// option dropdowns in sync with the agent without waiting for an agent-driven
// ConfigOptionUpdate.
func (a *Adapter) emitSetConfigOptionEvent(
	sessionID string,
	configID string,
	value string,
	cachedModels []modelInfo,
	cachedConfig []streams.ConfigOption,
	expectedGeneration ...uint64,
) {
	a.emitSetConfigOptionEventWithPolicy(
		sessionID, configID, value, cachedModels, cachedConfig, streams.SessionSettingsPolicyStrict, expectedGeneration...,
	)
}

func (a *Adapter) emitSetConfigOptionEventWithPolicy(
	sessionID string,
	configID string,
	value string,
	cachedModels []modelInfo,
	cachedConfig []streams.ConfigOption,
	settingsPolicy streams.SessionSettingsPolicy,
	expectedGeneration ...uint64,
) {
	if len(cachedConfig) == 0 {
		// In normal operation session/new populates availableConfigOptions
		// before the frontend can fire a SetConfigOption. Hitting this branch
		// means we accepted the RPC against a cache that was never seeded —
		// skip the convergence event entirely (an empty one would briefly
		// blank the UI selectors) and rely on the agent's own
		// ConfigOptionUpdate notification to correct the snapshot.
		a.logger.Warn("SetConfigOption succeeded but local config cache is empty; skipping convergence event",
			zap.String("session_id", sessionID),
			zap.String("config_id", configID),
		)
		return
	}
	outConfig := make([]streams.ConfigOption, len(cachedConfig))
	copy(outConfig, cachedConfig)
	found := false
	for i := range outConfig {
		if outConfig[i].ID == configID {
			outConfig[i].CurrentValue = value
			found = true
			break
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
	if a.sessionID != sessionID ||
		(len(expectedGeneration) > 0 && a.configGeneration != expectedGeneration[0]) {
		a.mu.Unlock()
		return
	}
	a.availableConfigOptions = outConfig
	a.sessionSettingsGeneration++
	event := AgentEvent{
		Type:                      streams.EventTypeSessionModels,
		SessionID:                 sessionID,
		CurrentModelID:            currentModelFromConfig(outConfig),
		SessionModels:             convertSessionModels(cachedModels),
		ConfigOptions:             outConfig,
		SessionSettingsPolicy:     providerRestoredPolicy(settingsPolicy),
		SessionSettingsGeneration: a.sessionSettingsGeneration,
	}
	a.mu.Unlock()

	a.logger.Info("emitting session_models convergence event after SetConfigOption",
		zap.String("session_id", sessionID),
		zap.String("config_id", configID),
		zap.String("value", value),
	)
	// COVERED site (AC-EXECUTORS-SURVIVAL-001.5/.6): sendUpdate blocks rather
	// than drops on a full updatesCh and must run without a.mu held -- see
	// emitDialectContextWindow's doc comment for the deadlock this avoids.
	a.sendUpdate(event)
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
		MethodId: acp.AuthMethodId(methodID),
	})
	if err != nil {
		return fmt.Errorf("authenticate failed: %w", err)
	}
	return nil
}
