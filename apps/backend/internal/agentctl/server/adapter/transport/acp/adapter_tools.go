package acp

import (
	"strings"

	"github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"go.uber.org/zap"
)

// convertToolCallContents converts ACP ToolCallContent items to our protocol-agnostic type.
func (a *Adapter) convertToolCallContents(contents []acp.ToolCallContent) []streams.ToolCallContentItem {
	if len(contents) == 0 {
		return nil
	}
	items := make([]streams.ToolCallContentItem, 0, len(contents))
	for _, c := range contents {
		switch {
		case c.Diff != nil:
			items = append(items, streams.ToolCallContentItem{
				Type:    "diff",
				Path:    c.Diff.Path,
				OldText: c.Diff.OldText,
				NewText: c.Diff.NewText,
			})
		case c.Content != nil:
			cb := a.convertContentBlockToStreams(c.Content.Content)
			if cb != nil {
				items = append(items, streams.ToolCallContentItem{
					Type:    "content",
					Content: cb,
				})
			}
		case c.Terminal != nil:
			items = append(items, streams.ToolCallContentItem{
				Type:       "terminal",
				TerminalID: c.Terminal.TerminalId,
			})
		}
	}
	if len(items) == 0 {
		return nil
	}
	return items
}

// convertContentBlockToStreams converts an ACP ContentBlock to a streams.ContentBlock.
func (a *Adapter) convertContentBlockToStreams(cb acp.ContentBlock) *streams.ContentBlock {
	switch {
	case cb.Text != nil:
		return &streams.ContentBlock{Type: "text", Text: cb.Text.Text}
	case cb.Image != nil:
		return &streams.ContentBlock{Type: contentTypeImage, Data: cb.Image.Data, MimeType: cb.Image.MimeType, URI: derefStr(cb.Image.Uri)}
	case cb.Audio != nil:
		return &streams.ContentBlock{Type: contentTypeAudio, Data: cb.Audio.Data, MimeType: cb.Audio.MimeType}
	case cb.ResourceLink != nil:
		return &streams.ContentBlock{
			Type: "resource_link", URI: cb.ResourceLink.Uri, Name: cb.ResourceLink.Name,
			MimeType: derefStr(cb.ResourceLink.MimeType), Title: derefStr(cb.ResourceLink.Title),
			Description: derefStr(cb.ResourceLink.Description), Size: cb.ResourceLink.Size,
		}
	case cb.Resource != nil:
		block := &streams.ContentBlock{Type: "resource"}
		res := cb.Resource.Resource
		switch {
		case res.TextResourceContents != nil:
			block.URI = res.TextResourceContents.Uri
			block.Text = res.TextResourceContents.Text
			block.MimeType = derefStr(res.TextResourceContents.MimeType)
		case res.BlobResourceContents != nil:
			block.URI = res.BlobResourceContents.Uri
			block.Data = res.BlobResourceContents.Blob
			block.MimeType = derefStr(res.BlobResourceContents.MimeType)
		}
		return block
	default:
		return nil
	}
}

// convertToolCallUpdate converts a ToolCall notification to an AgentEvent.
func (a *Adapter) convertToolCallUpdate(sessionID string, tc *acp.SessionUpdateToolCall) *AgentEvent {
	args := map[string]any{}

	if tc.Kind != "" {
		args["kind"] = string(tc.Kind)
	}

	if len(tc.Locations) > 0 {
		locations := make([]map[string]any, len(tc.Locations))
		for i, loc := range tc.Locations {
			locMap := map[string]any{"path": loc.Path}
			if loc.Line != nil {
				locMap["line"] = *loc.Line
			}
			locations[i] = locMap
		}
		args["locations"] = locations
		args["path"] = tc.Locations[0].Path
	}

	if tc.RawInput != nil {
		args["raw_input"] = tc.RawInput
	}

	toolKind := string(tc.Kind)
	normalizedPayload := a.normalizer.NormalizeToolCall(toolKind, args)

	toolCallID := string(tc.ToolCallId)
	a.mu.Lock()
	a.activeToolCalls[toolCallID] = normalizedPayload
	a.mu.Unlock()

	// ScheduleWakeup tracking: meta carries `_meta.claudeCode.toolName`
	// on the initial tool_call; rawInput is usually empty here but record
	// the prompt eagerly when it does arrive in the same notification.
	a.handleWakeupEvent(sessionID, toolCallID, tc.Meta, tc.RawInput, false)

	// Detect tool type for logging
	toolType := DetectToolOperationType(toolKind, args)
	_ = toolType // Used for normalization

	status := string(tc.Status)
	if status == "" {
		status = "in_progress"
	}

	return &AgentEvent{
		Type:              streams.EventTypeToolCall,
		SessionID:         sessionID,
		ToolCallID:        toolCallID,
		ToolName:          toolKind, // Kind is effectively the tool name
		ToolTitle:         tc.Title,
		ToolStatus:        status,
		NormalizedPayload: normalizedPayload,
		ToolCallContents:  a.convertToolCallContents(tc.Content),
	}
}

// convertToolCallResultUpdate converts a ToolCallUpdate notification to an AgentEvent.
//
//nolint:gocognit,cyclop,funlen // pre-existing complexity preserved from adapter.go file split
func (a *Adapter) convertToolCallResultUpdate(sessionID string, tcu *acp.SessionToolCallUpdate) *AgentEvent {
	toolCallID := string(tcu.ToolCallId)
	status := ""
	if tcu.Status != nil {
		status = string(*tcu.Status)
	}
	// Normalize status - "completed" -> "complete" for frontend consistency
	if status == "completed" {
		status = toolStatusComplete
	}
	// Claude-acp sends incremental updates (title, rawInput, content) with no
	// Status field — e.g. the second tool_call_update for Bash carries the actual
	// command and human-readable title. The orchestrator only persists updates
	// with a known status, so without a synthesized "in_progress" here those
	// fields are silently dropped and the message stays on the placeholder
	// "Terminal" title from the initial pending tool_call.
	if status == "" && (tcu.Title != nil || tcu.RawInput != nil || len(tcu.Content) > 0) {
		status = "in_progress"
	}

	// Recognize Monitor registration: claude-acp sends `tool_call_update` with
	// status="completed" and a `Monitor started (task X, …)` rawOutput about a
	// second after the Monitor starts. That status is misleading — the Monitor
	// itself is just beginning. Override to "in_progress" so the card stays
	// open, and remember taskID -> toolCallID so subsequent task-notification
	// envelopes can route their events back to this card.
	monitorTaskID, isMonitorRegistration := recognizeMonitorRegistration(tcu.Meta, tcu.RawOutput)
	if isMonitorRegistration && status == toolStatusComplete {
		a.trackMonitor(sessionID, monitorTaskID, toolCallID)
		status = "in_progress"
		a.logger.Info("monitor registered",
			zap.String("session_id", sessionID),
			zap.String("task_id", monitorTaskID),
			zap.String("tool_call_id", toolCallID))
	}

	// A terminal tool_call_update for an already-tracked Monitor (the agent
	// proactively ended the watch). NormalizeToolResult would otherwise stomp
	// the `{monitor: …}` view in Generic.Output with the raw string body, so
	// we suppress the normalize call and let the closing-out logic below mark
	// the view as ended instead.
	isTrackedMonitorTerminal := !isMonitorRegistration && isMonitorMeta(tcu.Meta) && a.isTrackedMonitor(sessionID, toolCallID)

	isTerminal := status == toolStatusComplete || status == toolStatusError || status == "cancelled"

	a.mu.Lock()
	payload := a.activeToolCalls[toolCallID]

	// Update stored payload with incremental rawInput (e.g. Claude Code sends
	// command/cwd in a tool_call_update after the initial empty tool_call)
	if tcu.RawInput != nil && payload != nil {
		a.normalizer.UpdatePayloadInput(payload, tcu.RawInput)
	}

	// Update stored payload with tool result output. Skip for tracked-Monitor
	// terminal updates so Generic.Output stays the structured `{monitor: …}`
	// view rather than getting clobbered by the rawOutput string.
	if tcu.RawOutput != nil && payload != nil && !isTrackedMonitorTerminal {
		a.normalizer.NormalizeToolResult(payload, tcu.RawOutput)
	}

	// Seed the Monitor view AFTER NormalizeToolResult so we overwrite the
	// banner string the normalizer just stuffed into Generic.Output. The
	// Monitor card detects itself by `output.monitor` presence — the banner
	// would shadow it and the frontend would render this as a generic
	// tool_call instead.
	if isMonitorRegistration && payload != nil {
		seedMonitorView(payload, monitorTaskID, monitorCommandFromPayload(payload))
	}

	// Preserve and mark-ended the Monitor view on tracked-Monitor terminal
	// updates so the card flips from "watching" to "ended" without losing
	// the accumulated event count or recent-events tail.
	if isTrackedMonitorTerminal && payload != nil {
		markMonitorEnded(payload, "exited")
	}

	// Enrich modify_file payload from tool_call_contents.
	// Claude ACP sends path and content in tool_call_update, not in the initial tool_call.
	if payload != nil && payload.Kind() == streams.ToolKindModifyFile {
		if mf := payload.ModifyFile(); mf != nil {
			enrichModifyFileFromContents(mf, tcu.Content)
		}
	}

	// Enrich read_file payload path from title if still empty.
	if payload != nil && payload.Kind() == streams.ToolKindReadFile {
		if rf := payload.ReadFile(); rf != nil && rf.FilePath == "" && tcu.Title != nil {
			rf.FilePath = extractPathFromTitle(*tcu.Title)
		}
	}

	if isTerminal {
		delete(a.activeToolCalls, toolCallID)
		// Also drop tracked Monitor: this terminal update is the
		// agent-emitted close, so the prompt-end sweep must not re-emit a
		// "Monitor exited" event for this same toolCallID.
		if isTrackedMonitorTerminal {
			a.dropMonitorByToolCallIDLocked(sessionID, toolCallID)
		}
	}
	a.mu.Unlock()

	// ScheduleWakeup tracking: tool_call_update is where rawInput.prompt and
	// `_meta.claudeCode.toolResponse.scheduledFor` typically arrive. Once both
	// are known, schedule the synthetic prompt; on terminal status, clean up.
	a.handleWakeupEvent(sessionID, toolCallID, tcu.Meta, tcu.RawInput, isTerminal)

	// When a switch_mode tool carries a plan (e.g. ExitPlanMode), emit it
	// as an agent_plan event so the orchestrator creates a visible plan message.
	if tcu.RawInput != nil {
		if inputMap, ok := tcu.RawInput.(map[string]any); ok {
			if planContent, ok := inputMap["plan"].(string); ok && planContent != "" {
				a.sendUpdate(AgentEvent{
					Type:        streams.EventTypeAgentPlan,
					SessionID:   sessionID,
					PlanContent: planContent,
				})
			}
		}
	}

	// Extract title from update if present
	var title string
	if tcu.Title != nil {
		title = *tcu.Title
	}

	return &AgentEvent{
		Type:              streams.EventTypeToolUpdate,
		SessionID:         sessionID,
		ToolCallID:        toolCallID,
		ToolTitle:         title,
		ToolStatus:        status,
		NormalizedPayload: payload,
		ToolCallContents:  a.convertToolCallContents(tcu.Content),
	}
}

// enrichModifyFileFromContents updates a ModifyFilePayload with data from
// tool_call_contents. Claude ACP sends file path and content in tool_call_update
// events rather than in the initial tool_call rawInput.
func enrichModifyFileFromContents(mf *streams.ModifyFilePayload, contents []acp.ToolCallContent) {
	for _, c := range contents {
		if c.Diff == nil {
			continue
		}
		if mf.FilePath == "" && c.Diff.Path != "" {
			mf.FilePath = c.Diff.Path
		}
		if len(mf.Mutations) == 0 {
			continue
		}
		mut := &mf.Mutations[0]
		if mut.Diff != "" {
			continue // Already has diff, don't overwrite
		}
		if c.Diff.OldText != nil {
			diffPath := c.Diff.Path
			if diffPath == "" {
				diffPath = mf.FilePath
			}
			mut.Diff = shared.GenerateUnifiedDiff(*c.Diff.OldText, c.Diff.NewText, diffPath, mut.StartLine)
		} else if c.Diff.NewText != "" {
			mut.Type = streams.MutationCreate
			mut.Content = c.Diff.NewText
		}
		break
	}
}

// extractPathFromTitle extracts a file path from tool titles like "Read /path/to/file".
func extractPathFromTitle(title string) string {
	for _, prefix := range []string{"Read ", "Write ", "Edit "} {
		if strings.HasPrefix(title, prefix) {
			return strings.TrimPrefix(title, prefix)
		}
	}
	return ""
}
