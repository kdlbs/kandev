package acp

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"
)

const maxNativeAttachmentBytes int64 = 8 << 20

// Prompt sends a prompt to the agent.
// If pending context is set (from SetPendingContext), it will be prepended to the message.
// Attachments (images) are converted to ACP ImageBlocks and included in the prompt.
// When the prompt completes, a complete event is emitted via the updates channel.
func (a *Adapter) Prompt(
	ctx context.Context,
	message string,
	attachments []v1.MessageAttachment,
	promptGeneration uint64,
) error {
	// A user prompt always targets the current session, so it is not pinned.
	return a.sendPrompt(ctx, message, attachments, "", promptGeneration, false)
}

// SupportsSteering reports whether this adapter can deliver a prompt into a turn
// that is still generating. It is the same negotiated advertisement that gates
// prompt handoff — the agent must accept a concurrent session/prompt.
func (a *Adapter) SupportsSteering() bool {
	return a.supportsPromptHandoff()
}

// PromptSteer delivers a prompt without waiting for the in-flight turn to end.
//
// It differs from Prompt in exactly one way: instead of blocking on the prompt
// gate until the current turn releases it (or a provider foreground-idle event
// attests a handoff), the operator's send is itself the handoff trigger. The
// predecessor's `session/prompt` stays open and the token transfers to this
// call, so two prompts briefly overlap on one ACP session with one logical
// owner — the arrangement ADR 0049 already built for the foreground-idle case.
//
// Delivery is opportunistic. Whether the agent folds this prompt into the
// running turn or runs it as the next turn is the agent's decision and is not
// advertised over the protocol, so both outcomes must be correct. See
// docs/specs/platform/requirements/mid-turn-steering.md.
func (a *Adapter) PromptSteer(
	ctx context.Context,
	message string,
	attachments []v1.MessageAttachment,
	promptGeneration uint64,
) error {
	return a.sendPrompt(ctx, message, attachments, "", promptGeneration, true)
}

// sendPrompt serializes session/prompt calls through promptGate and sends one
// prompt to the agent. When expectSession is non-empty the prompt is pinned to
// that session: if the adapter's active session changed (or the adapter closed)
// while this call waited on the gate, the prompt is dropped instead of being
// sent to whatever session is now current. This is the ScheduleWakeup path —
// a wakeup must reach the session it was scheduled for or not at all.
//
//nolint:cyclop,funlen // pre-existing complexity preserved from adapter.go file split
func (a *Adapter) sendPrompt(
	ctx context.Context,
	message string,
	attachments []v1.MessageAttachment,
	expectSession string,
	promptGeneration uint64,
	steer bool,
) error {
	humanPrompt := expectSession == ""
	promptCtx, turn := newPromptTurnState(
		ctx,
		promptGeneration,
		humanPrompt && a.supportsPromptHandoff() && promptGeneration != 0,
	)
	// A steer initiates the handoff itself rather than waiting for a provider
	// foreground-idle event. Best-effort by design: if there is no handoff-eligible
	// turn in flight (idle session, or a synthetic wakeup holding the gate), this
	// is a no-op and the call falls through to ordinary gate acquisition, which is
	// exactly the specified behavior for those cases.
	//
	// Guarded on a live context: beginSteerHandoff protects the predecessor's
	// background work and closes its handoff channel, which only pays off once a
	// successor actually acquires the gate. If ctx is already cancelled the
	// acquisition below will fail immediately, so triggering the handoff first
	// would strand that protection with no successor to clear it.
	if steer && humanPrompt && promptGeneration != 0 && a.supportsPromptHandoff() && ctx.Err() == nil {
		a.beginSteerHandoff()
	}
	if err := a.acquirePromptTurn(ctx, turn, humanPrompt); err != nil {
		return err
	}
	defer a.finishPromptTurn(turn)

	a.mu.Lock()
	conn := a.acpConn
	sessionID := a.sessionID
	closed := a.closed
	// A pinned prompt that no longer matches the active session (or a closed
	// adapter) is dropped. Wakeup-pinned prompts are synthetic turns — they
	// must not consume pendingContext reserved for the next user prompt (e.g.
	// fork_session resume context).
	drop := expectSession != "" && (closed || sessionID != expectSession)
	var pendingContext string
	if !drop && expectSession == "" {
		pendingContext = a.pendingContext
		a.pendingContext = "" // Clear after use
	}
	a.mu.Unlock()

	if drop {
		a.logger.Info("dropping queued wakeup prompt: session changed or adapter closed before it ran",
			zap.String("scheduled_for", expectSession),
			zap.String("current_session", sessionID),
			zap.Bool("closed", closed))
		return nil
	}

	if conn == nil {
		return fmt.Errorf("adapter not initialized")
	}

	// Inject pending context if available (fork_session pattern)
	finalMessage := message
	if pendingContext != "" {
		finalMessage = pendingContext
		a.logger.Info("injecting resume context into prompt",
			zap.String("session_id", sessionID),
			zap.Int("context_length", len(pendingContext)))
	}
	a.beginPromptTurn(sessionID)

	contentBlocks := a.buildPromptContentBlocks(finalMessage, attachments)

	// Start prompt span — notification spans become children via getPromptTraceCtx()
	traceCtx, promptSpan := shared.TraceProtocolRequest(
		promptCtx,
		shared.ProtocolACP,
		a.agentID,
		"prompt",
	)
	promptSpan.SetAttributes(
		attribute.String("session_id", sessionID),
		attribute.Int("prompt_length", len(finalMessage)),
		attribute.Int("image_count", len(attachments)),
	)
	a.setPromptTraceCtx(turn, traceCtx)

	// Clear the loading flag before sending the prompt.
	// If we're resuming a session, history replay is complete by the time we send a new prompt.
	a.mu.Lock()
	wasLoading := a.isLoadingSession
	a.isLoadingSession = false
	a.mu.Unlock()

	if wasLoading {
		a.logger.Info("cleared session load suppression flag before sending new prompt",
			zap.String("session_id", sessionID))
	}

	a.logger.Info("sending prompt",
		zap.String("session_id", sessionID),
		zap.Int("content_blocks", len(contentBlocks)),
		zap.Int("image_attachments", len(attachments)))

	var resp acp.PromptResponse
	var err error
	go func() {
		defer close(turn.rpcDone)
		resp, err = conn.Prompt(traceCtx, acp.PromptRequest{
			SessionId: acp.SessionId(sessionID),
			Prompt:    contentBlocks,
		})
	}()

	if waitErr := a.waitForPromptRPCAfterUserCancel(turn, sessionID); waitErr != nil {
		promptSpan.RecordError(waitErr)
		promptSpan.End()
		a.clearPromptTraceCtx(turn)
		return waitErr
	}

	ownsCompletion := a.claimPromptTurnCompletion(turn)
	// Clear prompt context and end span regardless of outcome. A transferred
	// predecessor cannot clear the successor's trace.
	a.clearPromptTraceCtx(turn)
	stopReason := ""
	if err != nil {
		promptSpan.RecordError(err)
	} else {
		stopReason = string(resp.StopReason)
		promptSpan.SetAttributes(attribute.String("stop_reason", stopReason))
	}
	promptSpan.End()

	if !ownsCompletion {
		a.logger.Debug("suppressing completion from handed-off prompt RPC",
			zap.String("session_id", sessionID),
			zap.Uint64("prompt_generation", promptGeneration),
			zap.Error(err))
		return nil
	}
	if err != nil {
		return normalizePromptErrorAfterCancel(traceCtx, err)
	}

	// Drain queued ACP notifications before running the post-prompt sweeps and
	// the complete event. The worker (now async) may still hold final frames
	// the agent emitted right before its prompt response — the final text
	// chunk, the terminal monitor_end tool_call_update, the registration
	// frame for a Monitor whose events haven't yet been routed. Without this
	// barrier:
	//   - cancelActiveToolCalls / sweepMonitorsOnPromptEnd race the worker
	//     for activeToolCalls and activeMonitors. If the worker hasn't added
	//     a Monitor yet when the sweep takes the map, subsequent monitor_event
	//     text frames find no tracking and drop their events on the floor.
	//   - The complete event emitted to updatesCh outruns the final text chunk,
	//     so the downstream buffer flush yields empty and the turn persists as
	//     had_output=false even when the agent did produce text.
	a.syncNotifQueue()

	// Cancel any tool calls still in-flight (e.g. a denied permission leaves the
	// tool_call without a terminal status update from the agent).
	a.cancelPromptEndToolCalls(sessionID)

	// Mark any tracked Monitors as ended. They live longer than a typical tool
	// call (the script keeps running across model turns), so this sweep runs
	// after `cancelActiveToolCalls` to give the Monitor card a clean terminal
	// state when the parent prompt completes naturally.
	a.sweepMonitorsOnPromptEnd(sessionID)

	// Drop any Cursor `cursor/task` metadata for this session that never matched
	// a subagent tool_call this turn.
	a.sweepCursorTaskMetaOnPromptEnd(sessionID)

	if a.agentID == codexAgentID && turn.codexCapacityFailure() {
		const safeMessage = codexModelCapacityErrorMessage
		a.logger.Info("codex prompt ended with model-capacity evidence",
			zap.String("session_id", sessionID),
			zap.Uint64("prompt_generation", promptGeneration))
		a.cancelAsyncTurnComplete(sessionID)
		a.sendUpdate(AgentEvent{
			Type:             streams.EventTypeError,
			SessionID:        sessionID,
			PromptGeneration: promptGeneration,
			Error:            safeMessage,
			ProviderError: &streams.ProviderError{
				Source:     streams.ProviderErrorSourceCodexACP,
				ProviderID: codexAgentID,
				Message:    safeMessage,
				OccurredAt: time.Now().UTC(),
			},
		})
		return nil
	}

	// Emit complete event via the stream, including the StopReason from the agent.
	a.logger.Debug("emitting complete event after prompt",
		zap.String("session_id", sessionID),
		zap.String("stop_reason", stopReason))
	a.cancelAsyncTurnComplete(sessionID)
	usage := a.dialect.promptUsage(extractUsage(&resp), resp.Meta)
	// Typed per-turn usage frames aren't universal — an adapter with no
	// result.usage and no recognized _meta shape leaves usage nil here.
	// usageBySession tracks cumulative context-window occupancy
	// (usage_update frames) as a fallback signal for that case; see
	// fallbackUsageForNilTypedUsage's doc comment. usage_update cost is
	// cumulative session cost; consumeUsageDelta converts it to the
	// current turn's nonnegative delta.
	delta, costSubcents, costPresent := a.consumeUsageDeltaWithPresence(sessionID)
	if usage == nil {
		usage = fallbackUsageForNilTypedUsage(delta, costSubcents, costPresent)
	} else if costPresent {
		// claude-acp: usage_update.cost.amount carries authoritative cumulative
		// USD cost — attach the derived turn delta so Layer A wins
		// downstream and the office cost subscriber stores the row
		// verbatim instead of falling back to models.dev. claude-acp's
		// model id is a logical alias (sonnet / haiku) that won't match
		// any pricing entry, so this is the only accurate cost path.
		usage.ProviderReportedCostSubcents = costSubcents
		usage.ProviderReportedCostPresent = true
	}
	a.sendUpdate(AgentEvent{
		Type:             streams.EventTypeComplete,
		SessionID:        sessionID,
		PromptGeneration: promptGeneration,
		Data:             map[string]any{"stop_reason": stopReason},
		Usage:            usage,
	})

	return nil
}

// fallbackUsageForNilTypedUsage synthesizes a usage frame from
// context-window-occupancy growth for an adapter that reported no typed
// per-turn usage at all (extractUsage found nothing recognizable). It
// fires on nonnegative context growth or a provider-reported cost sample —
// whichever is present — matching the pre-existing contract other callers
// (e.g. the steering handoff path, which reports usage via cumulative
// context growth alone with no cost) already depend on. It only ever
// carries InputTokens: this adapter shape has no way to observe output
// tokens, so OutputTokens is left at its zero value and Estimated=true is
// the signal downstream must use to treat the whole row, including that
// zero, as approximate rather than measured (see streams.PromptUsage's
// doc comment).
func fallbackUsageForNilTypedUsage(delta, costSubcents int64, costPresent bool) *streams.PromptUsage {
	if delta <= 0 && !costPresent {
		return nil
	}
	return &streams.PromptUsage{
		InputTokens:                  delta,
		Estimated:                    true,
		ProviderReportedCostSubcents: costSubcents,
		ProviderReportedCostPresent:  costPresent,
	}
}

// supportsPromptHandoff reports whether this adapter's connected agent may have
// its in-flight prompt handed off to a human successor. Gated on the negotiated
// prompt-queueing advertisement rather than the agent's id, per ADR 0049's
// rejection of a central agent-name whitelist.
func (a *Adapter) supportsPromptHandoff() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.promptQueueing
}

func normalizePromptErrorAfterCancel(promptCtx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(context.Cause(promptCtx), ErrTurnCancelNotAcknowledged) {
		return errPromptAbandonedAfterCancel
	}
	return err
}

func (a *Adapter) buildPromptContentBlocks(message string, attachments []v1.MessageAttachment) []acp.ContentBlock {
	contentBlocks := []acp.ContentBlock{acp.TextBlock(message)}

	for _, att := range attachments {
		if att.DeliveryMode == "path" {
			saved, saveErr := a.attachMgr.SaveAttachments([]v1.MessageAttachment{att})
			if saveErr != nil || len(saved) == 0 {
				a.logger.Warn("failed to save path-mode attachment to workspace",
					zap.String("name", att.Name), zap.Error(saveErr))
				contentBlocks = append(contentBlocks, buildAttachmentFallbackBlock(att))
				continue
			}
			contentBlocks = append(contentBlocks, acp.TextBlock(shared.BuildAttachmentPrompt(saved, true)))
			continue
		}
		if att.AttachmentID != "" && att.Data == "" && (att.Type == contentTypeImage || att.Type == contentTypeAudio) {
			data, saved, ok := a.loadMaterializedAttachmentData(att)
			switch {
			case ok:
				att.Data = data
			case len(saved) > 0:
				contentBlocks = append(contentBlocks, acp.TextBlock(shared.BuildAttachmentPrompt(saved, false)))
				continue
			default:
				a.logger.Warn("failed to load materialized prompt attachment",
					zap.String("attachment_id", att.AttachmentID), zap.String("name", att.Name))
			}
		}

		switch att.Type {
		case contentTypeImage:
			contentBlocks = append(contentBlocks, acp.ImageBlock(att.Data, att.MimeType))
		case contentTypeAudio:
			contentBlocks = append(contentBlocks, acp.AudioBlock(att.Data, att.MimeType))
		case contentTypeResource:
			if a.capabilities.PromptCapabilities.EmbeddedContext {
				contentBlocks = append(contentBlocks, buildResourceBlock(att))
			} else {
				// Agent doesn't support embedded resources — save to workspace and reference in text.
				saved, saveErr := a.attachMgr.SaveAttachments([]v1.MessageAttachment{att})
				if saveErr != nil || len(saved) == 0 {
					a.logger.Warn("failed to save attachment to workspace, falling back to resource block",
						zap.String("name", att.Name), zap.Error(saveErr))
					contentBlocks = append(contentBlocks, buildResourceBlock(att))
				} else {
					contentBlocks = append(contentBlocks, acp.TextBlock(shared.BuildAttachmentPrompt(saved, false)))
				}
			}
		}
	}

	return contentBlocks
}

func (a *Adapter) loadMaterializedAttachmentData(att v1.MessageAttachment) (string, []shared.SavedAttachment, bool) {
	if a.attachMgr == nil {
		return "", nil, false
	}
	saved, err := a.attachMgr.SaveAttachments([]v1.MessageAttachment{att})
	if err != nil || len(saved) == 0 {
		return "", nil, false
	}
	info, err := os.Stat(saved[0].AbsPath)
	if err != nil || info.Size() > maxNativeAttachmentBytes {
		return "", saved, false
	}
	data, err := os.ReadFile(saved[0].AbsPath)
	if err != nil {
		return "", saved, false
	}
	return base64.StdEncoding.EncodeToString(data), saved, true
}

func buildAttachmentFallbackBlock(att v1.MessageAttachment) acp.ContentBlock {
	switch att.Type {
	case contentTypeImage:
		return acp.ImageBlock(att.Data, att.MimeType)
	case contentTypeAudio:
		return acp.AudioBlock(att.Data, att.MimeType)
	case contentTypeResource:
		return buildResourceBlock(att)
	default:
		return acp.TextBlock("The user attached a file, but Kandev could not save it to the workspace.")
	}
}

// fireWakeup is invoked by wakeupScheduler when a ScheduleWakeup timer
// elapses. It issues a synthetic session/prompt so the upstream
// @agentclientprotocol/claude-agent-acp bridge drains the SDK's queued wakeup
// turn and emits visible ACP frames. The session must still match (the user
// hasn't started a fresh session) and the adapter must not be closed.
//
// Prompt serialization: the synthetic prompt goes through sendPrompt, which
// gates on promptGate so only one session/prompt is in flight at a time. If a
// user prompt is already running the wakeup waits behind it rather than racing
// it — two concurrent conn.Prompt() calls would let the bridge return each
// prompt's stop_reason against the wrong turn, shifting chat turns one prompt
// behind. Because the wakeup can wait, the session is re-validated inside
// sendPrompt (via the pinned expectSession argument): if a NewSession/LoadSession
// changed the active session while the wakeup queued, it is dropped instead of
// being injected into the new session.
func (a *Adapter) fireWakeup(sessionID, prompt string) {
	a.mu.RLock()
	closed := a.closed
	currentSession := a.sessionID
	a.mu.RUnlock()

	if closed {
		a.logger.Debug("skipping wakeup fire: adapter closed",
			zap.String("session_id", sessionID))
		return
	}
	if currentSession != sessionID {
		a.logger.Info("skipping wakeup fire: session changed",
			zap.String("scheduled_for", sessionID),
			zap.String("current", currentSession))
		return
	}

	a.logger.Info("injecting synthetic wakeup prompt",
		zap.String("session_id", sessionID),
		zap.Int("prompt_len", len(prompt)))

	go func() {
		// Derive from lifetimeCtx so a concurrent Close aborts the in-flight
		// prompt instead of letting it run against a dead subprocess.
		ctx, cancel := context.WithTimeout(a.lifetimeCtx, wakeupPromptTimeout)
		defer cancel()
		// Pin to the scheduled session: if the active session changed while this
		// wakeup waited on the prompt gate, sendPrompt drops it.
		// Never a steer: a synthetic wakeup must stay serialized behind the owning
		// prompt and must not consume a handoff meant for a human successor.
		if err := a.sendPrompt(ctx, prompt, nil, sessionID, 0, false); err != nil {
			a.logger.Error("synthetic wakeup prompt failed",
				zap.String("session_id", sessionID),
				zap.Error(err))
		}
	}()
}

// handleWakeupEvent inspects a tool-call meta + rawInput pair, accumulates
// pending state per toolCallID, and schedules a wakeup once both the prompt
// and scheduledFor timestamp are known. terminal=true means the tool call has
// reached a terminal state, so any pending entry should be cleaned up.
func (a *Adapter) handleWakeupEvent(sessionID, toolCallID string, meta any, rawInput any, terminal bool) {
	if toolCallID == "" {
		return
	}

	scheduledForMs, isWakeup := extractScheduleWakeup(meta)

	a.mu.Lock()
	pw, tracked := a.pendingWakeups[toolCallID]
	if !tracked {
		if !isWakeup {
			a.mu.Unlock()
			return
		}
		pw = &pendingWakeup{}
		a.pendingWakeups[toolCallID] = pw
	}

	if scheduledForMs > 0 {
		pw.scheduledForMs = scheduledForMs
	}
	if prompt, ok := extractWakeupPrompt(rawInput); ok {
		pw.prompt = prompt
	}

	prompt := pw.prompt
	stamp := pw.scheduledForMs
	if (prompt != "" && stamp > 0) || terminal {
		delete(a.pendingWakeups, toolCallID)
	}
	a.mu.Unlock()

	if prompt != "" && stamp > 0 {
		a.wakeup.schedule(sessionID, prompt, stamp)
	}
}

// Cancel cancels the current operation.
// Per ACP spec, the client must immediately mark non-finished tool calls as cancelled.
func (a *Adapter) Cancel(ctx context.Context) error {
	a.mu.RLock()
	conn := a.acpConn
	sessionID := a.sessionID
	a.mu.RUnlock()

	if conn == nil {
		return fmt.Errorf("adapter not initialized")
	}

	ctx, span := shared.TraceProtocolRequest(ctx, shared.ProtocolACP, a.agentID, "cancel")
	defer span.End()
	span.SetAttributes(attribute.String("session_id", sessionID))

	a.logger.Info("cancelling session", zap.String("session_id", sessionID))

	// Mark all active tool calls as cancelled before sending cancel to agent.
	a.cancelActiveToolCalls(sessionID)

	if err := conn.Cancel(ctx, acp.CancelNotification{
		SessionId: acp.SessionId(sessionID),
	}); err != nil {
		span.RecordError(err)
		// Still wake the in-flight prompt waiter so sendPrompt can exit within
		// promptCancelJoinTimeout rather than blocking the gate forever. The
		// timeout branch in waitForPromptRPCAfterUserCancel will cancel
		// promptCtx if the agent never acknowledges.
		a.signalPromptTurnAbort()
		return err
	}

	turn := a.signalPromptTurnAbort()
	if err := waitForPromptRPCAfterCancel(turn); err != nil {
		span.RecordError(err)
		a.logger.Warn("session/cancel sent but in-flight prompt did not end",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return err
	}

	a.logger.Info("cancel acknowledged by in-flight prompt",
		zap.String("session_id", sessionID))
	return nil
}

// cancelActiveToolCalls emits cancelled tool_update events for all in-flight tool calls
// and clears the activeToolCalls map.
//
// Monitor tool calls are intentionally skipped here — they are tracked in
// activeMonitors and given their own terminal sweep (sweepMonitorsOnPromptEnd
// or sweepMonitorsOnReplayEnd) which uses the appropriate status and a
// payload snapshot. Without this skip the Monitor would receive two
// terminal events with conflicting states.
//
// Subagent (Task) tool calls are also preserved: the Claude Agent SDK can
// return session/prompt with stop_reason while the subagent is still running
// (anthropics/claude-code#47936). Cancelling the card here would mark it
// terminated even though the SDK keeps streaming its real tool_call_update
// when the subagent finishes seconds later. Leaving the entry in
// activeToolCalls lets that authoritative terminal update land naturally.
func (a *Adapter) cancelActiveToolCalls(sessionID string) {
	a.cancelActiveToolCallsPreservingHandoff(sessionID, false)
}

// cancelPromptEndToolCalls preserves background work inherited from a
// handed-off predecessor. Explicit session cancellation still uses
// cancelActiveToolCalls so it can terminate the whole session.
func (a *Adapter) cancelPromptEndToolCalls(sessionID string) {
	a.cancelActiveToolCallsPreservingHandoff(sessionID, true)
}

func (a *Adapter) cancelActiveToolCallsPreservingHandoff(
	sessionID string,
	preserveHandoff bool,
) {
	a.mu.Lock()
	if !preserveHandoff {
		a.clearPromptHandoffToolTrackingLocked()
	}
	monitorToolCallIDs := make(map[string]bool)
	for _, tcID := range a.activeMonitors[sessionID] {
		monitorToolCallIDs[tcID] = true
	}
	toCancel := make(map[string]*streams.NormalizedPayload)
	preserved := make(map[string]*streams.NormalizedPayload)
	for tcID, payload := range a.activeToolCalls {
		switch {
		case preserveHandoff && a.isPromptHandoffToolProtectedLocked(tcID):
			preserved[tcID] = payload
		case monitorToolCallIDs[tcID]:
			preserved[tcID] = payload
		case payload != nil && payload.Kind() == streams.ToolKindSubagentTask:
			preserved[tcID] = payload
		default:
			toCancel[tcID] = payload
			a.forgetPromptHandoffToolLocked(tcID)
		}
	}
	a.activeToolCalls = preserved
	a.mu.Unlock()

	for toolCallID, normalized := range toCancel {
		a.logger.Debug("cancelling active tool call",
			zap.String("session_id", sessionID),
			zap.String("tool_call_id", toolCallID))
		a.sendUpdate(AgentEvent{
			Type:              streams.EventTypeToolUpdate,
			SessionID:         sessionID,
			ToolCallID:        toolCallID,
			ToolStatus:        toolStatusCancelled,
			NormalizedPayload: normalized,
		})
	}
}

func (a *Adapter) protectActiveBackgroundWorkForHandoff(sessionID string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	for toolCallID, payload := range a.activeToolCalls {
		if payload != nil && payload.IsActiveBackgroundWork() {
			a.handoffProtectedToolCalls[toolCallID] = struct{}{}
		}
	}
	for _, toolCallID := range a.activeMonitors[sessionID] {
		a.handoffProtectedToolCalls[toolCallID] = struct{}{}
	}

	// Include nested children that were already live at the handoff boundary.
	// Children arriving later inherit protection in trackToolCallLineage.
	for changed := true; changed; {
		changed = false
		for toolCallID, parentToolCallID := range a.toolCallParents {
			if _, parentProtected := a.handoffProtectedToolCalls[parentToolCallID]; !parentProtected {
				continue
			}
			if _, alreadyProtected := a.handoffProtectedToolCalls[toolCallID]; alreadyProtected {
				continue
			}
			a.handoffProtectedToolCalls[toolCallID] = struct{}{}
			changed = true
		}
	}
}

func (a *Adapter) trackToolCallLineage(toolCallID, parentToolCallID string) {
	if toolCallID == "" || parentToolCallID == "" {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.toolCallParents[toolCallID] = parentToolCallID
	if _, protected := a.handoffProtectedToolCalls[parentToolCallID]; protected {
		a.handoffProtectedToolCalls[toolCallID] = struct{}{}
	}
}

func (a *Adapter) isPromptHandoffToolProtectedLocked(toolCallID string) bool {
	_, protected := a.handoffProtectedToolCalls[toolCallID]
	return protected
}

func (a *Adapter) forgetPromptHandoffToolLocked(toolCallID string) {
	delete(a.toolCallParents, toolCallID)
	delete(a.handoffProtectedToolCalls, toolCallID)
}

func (a *Adapter) clearPromptHandoffToolTrackingLocked() {
	clear(a.toolCallParents)
	clear(a.handoffProtectedToolCalls)
}
