package acp

import (
	"context"
	"errors"
	"fmt"

	"github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"
)

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
	return a.sendPrompt(ctx, message, attachments, "", promptGeneration)
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
) error {
	// Acquire the prompt gate, honouring ctx so a queued wakeup whose context
	// is cancelled (timeout / adapter Close) aborts instead of blocking on a
	// stuck in-flight turn.
	select {
	case a.promptGate <- struct{}{}:
		defer func() { <-a.promptGate }()
	case <-ctx.Done():
		return ctx.Err()
	}

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
	traceCtx, promptSpan := shared.TraceProtocolRequest(ctx, shared.ProtocolACP, a.agentID, "prompt")
	promptSpan.SetAttributes(
		attribute.String("session_id", sessionID),
		attribute.Int("prompt_length", len(finalMessage)),
		attribute.Int("image_count", len(attachments)),
	)
	promptCtx, turn := a.registerPromptTurn(traceCtx)
	defer a.clearPromptTurn(turn)
	a.setPromptTraceCtx(promptCtx)

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
		resp, err = conn.Prompt(promptCtx, acp.PromptRequest{
			SessionId: acp.SessionId(sessionID),
			Prompt:    contentBlocks,
		})
	}()

	if waitErr := a.waitForPromptRPCAfterUserCancel(turn); waitErr != nil {
		promptSpan.RecordError(waitErr)
		promptSpan.End()
		a.clearPromptTraceCtx()
		return waitErr
	}

	// Clear prompt context and end span regardless of outcome
	a.clearPromptTraceCtx()
	stopReason := ""
	if err != nil {
		promptSpan.RecordError(err)
	} else {
		stopReason = string(resp.StopReason)
		promptSpan.SetAttributes(attribute.String("stop_reason", stopReason))
	}
	promptSpan.End()

	if err != nil {
		return normalizePromptErrorAfterCancel(promptCtx, err)
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
	a.cancelActiveToolCalls(sessionID)

	// Mark any tracked Monitors as ended. They live longer than a typical tool
	// call (the script keeps running across model turns), so this sweep runs
	// after `cancelActiveToolCalls` to give the Monitor card a clean terminal
	// state when the parent prompt completes naturally.
	a.sweepMonitorsOnPromptEnd(sessionID)

	// Emit complete event via the stream, including the StopReason from the agent.
	// This normalizes ACP behavior to match other adapters (stream-json, amp, copilot, opencode).
	a.logger.Debug("emitting complete event after prompt",
		zap.String("session_id", sessionID),
		zap.String("stop_reason", stopReason))
	a.cancelAsyncTurnComplete(sessionID)
	usage := a.driver.normalizePromptUsage(extractUsage(&resp), resp.Meta)
	// codex-acp emits no per-turn usage frame, only cumulative
	// usage_update.used. Fall back to the inferred delta so the office
	// cost subscriber sees at least an approximate input count. The
	// resulting cost is off (no input/output split) — flagged via
	// PromptUsage.Estimated so downstream rows carry estimated=true.
	delta, costSubcents := a.consumeUsageDelta(sessionID)
	if usage == nil {
		if delta > 0 || costSubcents > 0 {
			usage = &streams.PromptUsage{
				InputTokens:                  delta,
				Estimated:                    true,
				ProviderReportedCostSubcents: costSubcents,
			}
		}
	} else if costSubcents > 0 {
		// claude-acp: usage_update.cost.amount carries the authoritative
		// USD cost — attach it to the typed usage frame so Layer A wins
		// downstream and the office cost subscriber stores the row
		// verbatim instead of falling back to models.dev. claude-acp's
		// model id is a logical alias (sonnet / haiku) that won't match
		// any pricing entry, so this is the only accurate cost path.
		usage.ProviderReportedCostSubcents = costSubcents
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
		if err := a.sendPrompt(ctx, prompt, nil, sessionID, 0); err != nil {
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
	a.mu.Lock()
	monitorToolCallIDs := make(map[string]bool)
	for _, tcID := range a.activeMonitors[sessionID] {
		monitorToolCallIDs[tcID] = true
	}
	toCancel := make(map[string]*streams.NormalizedPayload)
	preserved := make(map[string]*streams.NormalizedPayload)
	for tcID, payload := range a.activeToolCalls {
		switch {
		case monitorToolCallIDs[tcID]:
			preserved[tcID] = payload
		case payload != nil && payload.Kind() == streams.ToolKindSubagentTask:
			preserved[tcID] = payload
		default:
			toCancel[tcID] = payload
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
