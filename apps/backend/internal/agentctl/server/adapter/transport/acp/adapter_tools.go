package acp

import (
	"encoding/base64"
	"strconv"

	"github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"go.uber.org/zap"
)

const (
	maxCodexCompletedSubagentCorrelations = 256
	codexSubagentRunningStatus            = "running"
)

type codexSubagentCorrelationKey struct {
	sessionID      string
	toolCallID     string
	childSessionID string
	occurrence     uint64
}

type codexSubagentCorrelation struct {
	emittedToolCallID string
	parentToolCallID  string
	payload           *streams.NormalizedPayload
	collaborationSeen bool
	activitySeen      bool
	lastSeen          uint64
}

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
					Type:    toolContentType,
					Content: cb,
				})
			}
		case c.Terminal != nil:
			items = append(items, streams.ToolCallContentItem{
				Type:       "terminal",
				TerminalID: string(c.Terminal.TerminalId),
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
		return &streams.ContentBlock{Type: contentTypeText, Text: cb.Text.Text}
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
		for k, v := range locationsArgsFromACP(tc.Locations) {
			args[k] = v
		}
	}

	if tc.RawInput != nil {
		args["raw_input"] = tc.RawInput
	}

	// Title + meta let the normalizer detect subagent (Task) tool calls
	// (OpenCode keys off title, Claude off `_meta.claudeCode.toolName`).
	args[argKeyTitle] = tc.Title
	if tc.Meta != nil {
		args[argKeyMeta] = tc.Meta
	}

	toolKind := string(tc.Kind)
	normalizedPayload := a.normalizer.NormalizeToolCall(toolKind, args)

	toolCallID := string(tc.ToolCallId)
	normalizedPayload, eventType, codexSignal, emittedToolCallID, codexParentToolCallID := a.trackToolCallPayload(
		sessionID,
		toolCallID,
		normalizedPayload,
		tc.Meta,
		string(tc.Status),
	)

	// ScheduleWakeup tracking: meta carries `_meta.claudeCode.toolName`
	// on the initial tool_call; rawInput is usually empty here but record
	// the prompt eagerly when it does arrive in the same notification.
	a.handleWakeupEvent(sessionID, toolCallID, tc.Meta, tc.RawInput, false)

	// Detect tool type for logging
	toolType := DetectToolOperationType(toolKind, args)
	_ = toolType // Used for normalization

	status := string(tc.Status)
	if status == "" {
		status = toolStatusInProgress
	}
	if codexSignal == codexSubagentSignalActivity {
		status = codexActivityToolStatus(normalizedPayload)
	}
	parentToolCallID := parentToolUseID(tc.Meta)
	if codexParentToolCallID != "" {
		parentToolCallID = codexParentToolCallID
	}

	return &AgentEvent{
		Type:              eventType,
		SessionID:         sessionID,
		ToolCallID:        emittedToolCallID,
		ParentToolCallID:  parentToolCallID,
		ToolName:          toolKind, // Kind is effectively the tool name
		ToolTitle:         tc.Title,
		ToolStatus:        status,
		NormalizedPayload: normalizedPayload,
		ToolCallContents:  a.convertToolCallContents(tc.Content),
	}
}

func (a *Adapter) trackToolCallPayload(
	sessionID string,
	toolCallID string,
	payload *streams.NormalizedPayload,
	meta map[string]any,
	status string,
) (*streams.NormalizedPayload, string, codexSubagentSignal, string, string) {
	eventType := streams.EventTypeToolCall
	signal := codexSubagentSignalNone
	if a.agentID == codexAgentID {
		signal = codexSubagentSignalFromMeta(meta)
	}
	if signal == codexSubagentSignalCollaboration && payload != nil && payload.SubagentTask() != nil && payload.SubagentTask().Status == "" {
		fillCodexStatus(payload.SubagentTask(), status)
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if signal == codexSubagentSignalNone {
		a.activeToolCalls[toolCallID] = payload
		return payload, eventType, signal, toolCallID, ""
	}

	correlation, duplicate := a.correlateCodexSubagentToolCallLocked(
		sessionID,
		toolCallID,
		payload,
		signal,
		codexSenderThreadID(meta),
	)
	if !duplicate {
		a.activeToolCalls[correlation.emittedToolCallID] = correlation.payload
		return cloneSubagentPayload(correlation.payload), eventType, signal,
			correlation.emittedToolCallID, correlation.parentToolCallID
	}
	if active := a.activeToolCalls[correlation.emittedToolCallID]; sameCodexSubagentChild(active, correlation.payload) {
		a.activeToolCalls[correlation.emittedToolCallID] = correlation.payload
	}
	return cloneSubagentPayload(correlation.payload), streams.EventTypeToolUpdate, signal,
		correlation.emittedToolCallID, correlation.parentToolCallID
}

func (a *Adapter) correlateCodexSubagentToolCallLocked(
	sessionID string,
	toolCallID string,
	candidate *streams.NormalizedPayload,
	signal codexSubagentSignal,
	parentThreadID string,
) (*codexSubagentCorrelation, bool) {
	if a.codexSubagentCorrelations == nil {
		a.codexSubagentCorrelations = make(map[codexSubagentCorrelationKey]*codexSubagentCorrelation)
	}
	childSessionID := codexSubagentChildID(candidate)
	key, correlation, found := a.findCodexSubagentCorrelationLocked(sessionID, toolCallID, childSessionID)
	if found {
		mergeCodexSubagentPayload(correlation.payload, candidate)
		a.touchCodexSubagentCorrelationLocked(correlation, signal)
		if childSessionID != "" && key.childSessionID == "" {
			delete(a.codexSubagentCorrelations, key)
			key.childSessionID = childSessionID
			key.occurrence = 0
			a.codexSubagentCorrelations[key] = correlation
		}
		if correlation.parentToolCallID == "" {
			correlation.parentToolCallID = a.codexParentToolCallIDLocked(sessionID, parentThreadID)
		}
		a.pruneCodexCompletedCorrelationsLocked()
		return correlation, true
	}
	key = codexSubagentCorrelationKey{
		sessionID:      sessionID,
		toolCallID:     toolCallID,
		childSessionID: childSessionID,
	}
	correlation = &codexSubagentCorrelation{payload: cloneSubagentPayload(candidate)}
	if _, occupied := a.codexSubagentCorrelations[key]; occupied {
		key.occurrence = a.nextCodexCorrelationOccurrenceLocked(key)
	}
	correlation.emittedToolCallID = a.allocateCodexEmittedToolCallIDLocked(
		sessionID,
		toolCallID,
		childSessionID,
		correlation,
	)
	correlation.parentToolCallID = a.codexParentToolCallIDLocked(sessionID, parentThreadID)
	a.touchCodexSubagentCorrelationLocked(correlation, signal)
	a.codexSubagentCorrelations[key] = correlation
	a.pruneCodexCompletedCorrelationsLocked()
	return correlation, false
}

func (a *Adapter) findCodexSubagentCorrelationLocked(
	sessionID string,
	toolCallID string,
	childSessionID string,
) (codexSubagentCorrelationKey, *codexSubagentCorrelation, bool) {
	exactKey := codexSubagentCorrelationKey{
		sessionID:      sessionID,
		toolCallID:     toolCallID,
		childSessionID: childSessionID,
	}
	if exact, ok := a.codexSubagentCorrelations[exactKey]; ok {
		if childSessionID != "" {
			return exactKey, exact, true
		}
		if a.codexCorrelationSiblingCountLocked(sessionID, toolCallID) == 1 {
			return exactKey, exact, true
		}
		return codexSubagentCorrelationKey{}, nil, false
	}

	// A frame with a known child may adopt one earlier child-less frame only
	// when it is the sole, incomplete correlation for this tool ID. A
	// child-less frame may match the sole known child even after completion:
	// codex-acp can replay or overlap one side of the pair without its child
	// ID. Additional siblings make that identity ambiguous, so the caller
	// creates a conservative standalone card.
	var candidateKey codexSubagentCorrelationKey
	var candidate *codexSubagentCorrelation
	count := 0
	for key, correlation := range a.codexSubagentCorrelations {
		if key.sessionID != sessionID || key.toolCallID != toolCallID {
			continue
		}
		count++
		candidateKey = key
		candidate = correlation
	}
	if count == 1 {
		if childSessionID == "" {
			return candidateKey, candidate, true
		}
		if candidateKey.childSessionID == "" && !codexCorrelationComplete(candidate) {
			return candidateKey, candidate, true
		}
	}
	return codexSubagentCorrelationKey{}, nil, false
}

func (a *Adapter) codexCorrelationSiblingCountLocked(sessionID, toolCallID string) int {
	count := 0
	for key := range a.codexSubagentCorrelations {
		if key.sessionID == sessionID && key.toolCallID == toolCallID {
			count++
		}
	}
	return count
}

func codexCorrelationComplete(correlation *codexSubagentCorrelation) bool {
	return correlation != nil && correlation.collaborationSeen && correlation.activitySeen
}

func codexSubagentChildID(payload *streams.NormalizedPayload) string {
	if payload == nil || payload.SubagentTask() == nil {
		return ""
	}
	return payload.SubagentTask().ChildSessionID
}

func sameCodexSubagentChild(left, right *streams.NormalizedPayload) bool {
	leftID, rightID := codexSubagentChildID(left), codexSubagentChildID(right)
	return left != nil && right != nil && leftID == rightID
}

func (a *Adapter) touchCodexSubagentCorrelationLocked(
	correlation *codexSubagentCorrelation,
	signal codexSubagentSignal,
) {
	a.codexSubagentSequence++
	correlation.lastSeen = a.codexSubagentSequence
	switch signal {
	case codexSubagentSignalCollaboration:
		correlation.collaborationSeen = true
	case codexSubagentSignalActivity:
		correlation.activitySeen = true
	}
}

func (a *Adapter) evictCodexSubagentCorrelationLocked() {
	key, found := a.oldestCodexSubagentCorrelationLocked()
	if found {
		delete(a.codexSubagentCorrelations, key)
	}
}

func (a *Adapter) oldestCodexSubagentCorrelationLocked() (codexSubagentCorrelationKey, bool) {
	var oldestKey codexSubagentCorrelationKey
	var oldestSequence uint64
	found := false
	for key, correlation := range a.codexSubagentCorrelations {
		if !codexCorrelationComplete(correlation) {
			continue
		}
		if !found || correlation.lastSeen < oldestSequence {
			oldestKey = key
			oldestSequence = correlation.lastSeen
			found = true
		}
	}
	return oldestKey, found
}

func (a *Adapter) pruneCodexCompletedCorrelationsLocked() {
	for a.codexCompletedCorrelationCountLocked() > maxCodexCompletedSubagentCorrelations {
		a.evictCodexSubagentCorrelationLocked()
	}
}

func (a *Adapter) codexCompletedCorrelationCountLocked() int {
	count := 0
	for _, correlation := range a.codexSubagentCorrelations {
		if codexCorrelationComplete(correlation) {
			count++
		}
	}
	return count
}

func (a *Adapter) nextCodexCorrelationOccurrenceLocked(key codexSubagentCorrelationKey) uint64 {
	occurrence := a.codexSubagentSequence + 1
	for {
		key.occurrence = occurrence
		if _, occupied := a.codexSubagentCorrelations[key]; !occupied {
			return occurrence
		}
		occurrence++
	}
}

func (a *Adapter) allocateCodexEmittedToolCallIDLocked(
	sessionID string,
	wireToolCallID string,
	childSessionID string,
	correlation *codexSubagentCorrelation,
) string {
	if a.codexEmittedToolCallIDs == nil {
		a.codexEmittedToolCallIDs = make(map[string]map[string]*codexSubagentCorrelation)
	}
	reserved := a.codexEmittedToolCallIDs[sessionID]
	if reserved == nil {
		reserved = make(map[string]*codexSubagentCorrelation)
		a.codexEmittedToolCallIDs[sessionID] = reserved
	}
	if _, exists := reserved[wireToolCallID]; !exists {
		reserved[wireToolCallID] = correlation
		return wireToolCallID
	}

	encodedChild := base64.RawURLEncoding.EncodeToString([]byte(childSessionID))
	base := wireToolCallID + "~codex-subagent~" + strconv.Itoa(len(childSessionID)) + "-" + encodedChild
	candidate := base
	for suffix := 2; reserved[candidate] != nil; suffix++ {
		candidate = base + "~" + strconv.Itoa(suffix)
	}
	reserved[candidate] = correlation
	return candidate
}

func (a *Adapter) codexParentToolCallIDLocked(sessionID, parentThreadID string) string {
	if parentThreadID == "" {
		return ""
	}
	var emittedID string
	matches := 0
	for key, correlation := range a.codexSubagentCorrelations {
		if key.sessionID != sessionID || codexSubagentChildID(correlation.payload) != parentThreadID {
			continue
		}
		emittedID = correlation.emittedToolCallID
		matches++
	}
	if matches == 1 {
		return emittedID
	}
	return ""
}

func (a *Adapter) codexSubagentUpdateTargetLocked(
	sessionID string,
	wireToolCallID string,
	meta map[string]any,
	title string,
	rawInput any,
) (*streams.NormalizedPayload, string, string, bool) {
	payload := a.activeToolCalls[wireToolCallID]
	if a.agentID != codexAgentID {
		return payload, wireToolCallID, "", false
	}
	signal := codexSubagentSignalFromMeta(meta)
	if signal == codexSubagentSignalNone {
		return payload, wireToolCallID, "", false
	}
	frame, ok := parseCodexSubagentFrame(meta, title, rawInput)
	if !ok {
		return nil, "", "", true
	}
	_, correlation, found := a.findCodexSubagentCorrelationLocked(
		sessionID,
		wireToolCallID,
		frame.result.ChildSessionID,
	)
	if !found {
		// A recognized Codex subagent update must never fall back to the bare
		// wire ID. codex-acp may reuse that ID for several sibling children;
		// without a unique child identity, mutating the active wire entry would
		// arbitrarily patch or complete the first sibling. Suppress the frame
		// until it can be correlated coherently.
		return nil, "", "", true
	}
	a.touchCodexSubagentCorrelationLocked(correlation, signal)
	a.pruneCodexCompletedCorrelationsLocked()
	return correlation.payload, correlation.emittedToolCallID, correlation.parentToolCallID, false
}

func (a *Adapter) clearCodexSubagentCorrelationsLocked(sessionID string) {
	if sessionID == "" {
		a.codexSubagentCorrelations = make(map[codexSubagentCorrelationKey]*codexSubagentCorrelation)
		a.codexEmittedToolCallIDs = make(map[string]map[string]*codexSubagentCorrelation)
		a.codexSubagentSequence = 0
		return
	}
	for key := range a.codexSubagentCorrelations {
		if key.sessionID == sessionID {
			delete(a.codexSubagentCorrelations, key)
		}
	}
	delete(a.codexEmittedToolCallIDs, sessionID)
}

func cloneSubagentPayload(payload *streams.NormalizedPayload) *streams.NormalizedPayload {
	if payload == nil || payload.SubagentTask() == nil {
		return payload
	}
	src := payload.SubagentTask()
	clone := streams.NewSubagentTask(src.Description, src.Prompt, src.SubagentType)
	dst := clone.SubagentTask()
	dst.Status = src.Status
	dst.AgentID = src.AgentID
	dst.Model = src.Model
	dst.ChildSessionID = src.ChildSessionID
	dst.DurationMs = src.DurationMs
	dst.TotalTokens = src.TotalTokens
	if src.ToolUseCount != nil {
		count := *src.ToolUseCount
		dst.ToolUseCount = &count
	}
	dst.ResultText = src.ResultText
	dst.IsAsync = src.IsAsync
	dst.OutputFile = src.OutputFile
	dst.CanReadOutputFile = src.CanReadOutputFile
	dst.SetIsAuggie(src.IsAuggie())
	return clone
}

func mergeCodexSubagentPayload(current, candidate *streams.NormalizedPayload) {
	if current == nil || candidate == nil {
		return
	}
	dst, src := current.SubagentTask(), candidate.SubagentTask()
	if dst == nil || src == nil {
		return
	}
	fillIfEmpty(&dst.Description, src.Description)
	fillIfEmpty(&dst.Prompt, src.Prompt)
	fillIfEmpty(&dst.SubagentType, src.SubagentType)
	fillIfEmpty(&dst.Model, src.Model)
	fillIfEmpty(&dst.ChildSessionID, src.ChildSessionID)
	if codexSubagentStatusRank(src.Status) >= codexSubagentStatusRank(dst.Status) {
		fillCodexStatus(dst, src.Status)
	}
}

func fillCodexStatus(payload *streams.SubagentTaskPayload, status string) {
	if status != "" {
		payload.Status = status
	}
}

func codexSubagentStatusRank(status string) int {
	switch status {
	case toolStatusCompleted, toolStatusComplete, "errored", "error", "interrupted", "shutdown", "notFound", toolStatusCancelled:
		return 3
	case codexSubagentRunningStatus, "inProgress", toolStatusInProgress:
		return 2
	case codexSubagentStarted, "pendingInit":
		return 1
	default:
		return 0
	}
}

func codexActivityToolStatus(payload *streams.NormalizedPayload) string {
	if payload != nil && payload.SubagentTask() != nil && codexSubagentStatusRank(payload.SubagentTask().Status) == 3 {
		return toolStatusComplete
	}
	return toolStatusInProgress
}

// convertToolCallResultUpdate converts a ToolCallUpdate notification to an AgentEvent.
//
//nolint:gocognit,cyclop,funlen // pre-existing complexity preserved from adapter.go file split
func (a *Adapter) convertToolCallResultUpdate(sessionID string, tcu *acp.SessionToolCallUpdate) *AgentEvent {
	toolCallID := string(tcu.ToolCallId)
	convertedContents := a.convertToolCallContents(tcu.Content)
	status := ""
	if tcu.Status != nil {
		status = string(*tcu.Status)
	}
	// Normalize ACP status spellings for frontend and lifecycle consistency.
	switch status {
	case toolStatusCompleted:
		status = toolStatusComplete
	case "failed":
		status = toolStatusError
	}
	// Claude-acp sends incremental updates (title, rawInput, content) with no
	// Status field — e.g. the second tool_call_update for Bash carries the actual
	// command and human-readable title. The orchestrator only persists updates
	// with a known status, so without a synthesized "in_progress" here those
	// fields are silently dropped and the message stays on the placeholder
	// "Terminal" title from the initial pending tool_call.
	if status == "" && (tcu.Title != nil || tcu.RawInput != nil || len(tcu.Content) > 0 || len(tcu.Locations) > 0) {
		status = toolStatusInProgress
	}

	// Recognize Monitor registration: claude-acp sends `tool_call_update` with
	// status="completed" and a `Monitor started (task X, …)` rawOutput about a
	// second after the Monitor starts. That status is misleading — the Monitor
	// itself is just beginning. Override to "in_progress" so the card stays
	// open, and remember taskID -> toolCallID so subsequent task-notification
	// envelopes can route their events back to this card.
	monitorTaskID, isMonitorRegistrationCandidate := recognizeMonitorRegistration(tcu.Meta, tcu.RawOutput)

	// Cheap pre-lock inspection of the meta envelope. The override itself runs
	// under the lock so it can gate on payload kind (subagent_task only).
	isAsyncLaunchedSub := isSubagentAsyncLaunched(tcu.Meta)
	supplemental := toolCallUpdateSupplemental(tcu)

	a.mu.Lock()
	updateTitle := ""
	if tcu.Title != nil {
		updateTitle = *tcu.Title
	}
	payload, emittedToolCallID, codexParentToolCallID, suppressCodexUpdate := a.codexSubagentUpdateTargetLocked(
		sessionID,
		toolCallID,
		tcu.Meta,
		updateTitle,
		tcu.RawInput,
	)
	if suppressCodexUpdate {
		a.mu.Unlock()
		return nil
	}
	monitorCommand := ""
	if payload != nil && (tcu.RawInput != nil || len(tcu.Locations) > 0) {
		a.normalizer.UpdatePayloadInput(payload, tcu.RawInput, supplemental)
	}
	if payload != nil && (tcu.Title != nil || tcu.Meta != nil || tcu.RawInput != nil || supplemental != nil) {
		a.normalizer.EnrichFromToolCallUpdate(payload, tcu.Title, tcu.Meta, tcu.RawInput, supplemental)
	}

	isMonitorRegistration := false
	if isMonitorRegistrationCandidate {
		monitorCommand, isMonitorRegistration, status = a.registerMonitorRegistrationLocked(
			sessionID,
			monitorTaskID,
			toolCallID,
			payload,
			status,
		)
	}

	// A terminal tool_call_update for an already-tracked Monitor (the agent
	// proactively ended the watch). NormalizeToolResult would otherwise stomp
	// the `{monitor: …}` view in Generic.Output with the raw string body, so
	// we suppress the normalize call and let the closing-out logic below mark
	// the view as ended instead.
	isTrackedMonitorTerminal := !isMonitorRegistration && isMonitorMeta(tcu.Meta) && a.isTrackedMonitorLocked(sessionID, toolCallID)

	// Recognize claude-acp's async-launched subagent envelope: the Task tool
	// successfully dispatched a background subagent. The dispatch IS terminal
	// for the Task tool itself — the subagent runs out-of-band and writes its
	// result to OutputFile, and no later tool_call_update arrives for it.
	// Override unconditionally (not gated on status == "") so a future SDK
	// version that adds Title/RawInput/Content to the async_launched frame
	// can't leave the card stuck after the earlier in_progress synthesis
	// kicked in. Gated on subagent_task payload kind so an unrelated
	// claudeCode-namespaced tool that happens to use the same status literal
	// can't accidentally terminate.
	if isAsyncLaunchedSub && payload != nil && payload.Kind() == streams.ToolKindSubagentTask {
		status = toolStatusComplete
	}

	// Update stored payload with tool result output. Skip for tracked-Monitor
	// terminal updates so Generic.Output stays the structured `{monitor: …}`
	// view rather than getting clobbered by the rawOutput string.
	recognizedShellUpdate := false
	if payload != nil && payload.Kind() == streams.ToolKindShellExec {
		recognizedShellUpdate = a.normalizer.NormalizeShellToolUpdate(payload, tcu.Meta, convertedContents, tcu.RawOutput)
	} else if tcu.RawOutput != nil && payload != nil && !isTrackedMonitorTerminal {
		a.normalizer.NormalizeToolResult(payload, tcu.RawOutput)
	}
	var shellExitCode *int
	if payload != nil && payload.ShellExec() != nil && payload.ShellExec().Output != nil {
		shellExitCode = payload.ShellExec().Output.ExitCode
	}
	if status == "" && recognizedShellUpdate {
		status = toolStatusInProgress
	}
	if shellExitCode != nil && status != toolStatusCancelled {
		if *shellExitCode != 0 {
			status = toolStatusError
		} else if recognizedShellUpdate && status != toolStatusError {
			status = toolStatusComplete
		}
	}
	isTerminal := status == toolStatusComplete || status == toolStatusError || status == toolStatusCancelled

	// Subagent (Task) result metadata is split across meta (Claude) and
	// rawOutput (OpenCode/Cursor); enrich the stored payload from both.
	if payload != nil && payload.Kind() == streams.ToolKindSubagentTask {
		a.normalizer.EnrichSubagentResult(payload, tcu.Meta, tcu.RawOutput)
	}

	// Seed the Monitor view AFTER NormalizeToolResult so we overwrite the
	// banner string the normalizer just stuffed into Generic.Output. The
	// Monitor card detects itself by `output.monitor` presence — the banner
	// would shadow it and the frontend would render this as a generic
	// tool_call instead.
	if isMonitorRegistration && payload != nil {
		seedMonitorView(payload, monitorTaskID, monitorCommand)
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

	if isTerminal {
		delete(a.activeToolCalls, emittedToolCallID)
		// Also drop tracked Monitor: this terminal update is the
		// agent-emitted close, so the prompt-end sweep must not re-emit a
		// "Monitor exited" event for this same toolCallID.
		if isTrackedMonitorTerminal {
			a.dropMonitorByToolCallIDLocked(sessionID, toolCallID)
		}
	}
	emittedPayload := cloneSubagentPayload(payload)
	a.mu.Unlock()

	// ScheduleWakeup tracking: tool_call_update is where rawInput.prompt and
	// `_meta.claudeCode.toolResponse.scheduledFor` typically arrive. Once both
	// are known, schedule the synthetic prompt; on terminal status, clean up.
	a.handleWakeupEvent(sessionID, toolCallID, tcu.Meta, tcu.RawInput, isTerminal)

	// Todo tools from MCP-style runtimes report the final list in the completion
	// output rather than through ACP's native plan notification. Feed those
	// entries into the same plan stream Claude/Codex already use so the existing
	// above-input todo indicator updates without a separate UI path. Emitted
	// after the adapter lock is released because sendUpdate takes a read lock
	// and sync.RWMutex is not reentrant.
	if tcu.RawOutput != nil {
		if entries, ok := planEntriesFromTodosResult(tcu.RawOutput); ok {
			a.sendUpdate(AgentEvent{
				Type:        streams.EventTypePlan,
				SessionID:   sessionID,
				PlanEntries: entries,
			})
		}
	}

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
	parentToolCallID := parentToolUseID(tcu.Meta)
	if codexParentToolCallID != "" {
		parentToolCallID = codexParentToolCallID
	}

	return &AgentEvent{
		Type:              streams.EventTypeToolUpdate,
		SessionID:         sessionID,
		ToolCallID:        emittedToolCallID,
		ParentToolCallID:  parentToolCallID,
		ToolTitle:         title,
		ToolStatus:        status,
		NormalizedPayload: emittedPayload,
		ToolCallContents:  convertedContents,
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

func (a *Adapter) registerMonitorRegistrationLocked(
	sessionID string,
	monitorTaskID string,
	toolCallID string,
	payload *streams.NormalizedPayload,
	status string,
) (string, bool, string) {
	if status != toolStatusComplete {
		a.logger.Warn("ignoring non-terminal monitor registration",
			zap.String("session_id", sessionID),
			zap.String("task_id", monitorTaskID),
			zap.String("tool_call_id", toolCallID),
			zap.String("status", status))
		return "", false, status
	}

	cmd, ok := validMonitorCommandFromPayload(payload)
	if !ok {
		a.logger.Warn("ignoring malformed monitor registration",
			zap.String("session_id", sessionID),
			zap.String("task_id", monitorTaskID),
			zap.String("tool_call_id", toolCallID))
		return "", false, status
	}

	a.trackMonitorLocked(sessionID, monitorTaskID, toolCallID)
	a.logger.Info("monitor registered",
		zap.String("session_id", sessionID),
		zap.String("task_id", monitorTaskID),
		zap.String("tool_call_id", toolCallID))
	return cmd, true, toolStatusInProgress
}

func toolCallUpdateSupplemental(tcu *acp.SessionToolCallUpdate) map[string]any {
	return locationsArgsFromACP(tcu.Locations)
}

// locationsArgsFromACP builds the locations/path args map shared by initial tool_call
// frames and tool_call_update supplemental maps.
func locationsArgsFromACP(locations []acp.ToolCallLocation) map[string]any {
	if len(locations) == 0 {
		return nil
	}
	locMaps := make([]map[string]any, len(locations))
	for i, loc := range locations {
		locMap := map[string]any{keyPath: loc.Path}
		if loc.Line != nil {
			locMap["line"] = *loc.Line
		}
		locMaps[i] = locMap
	}
	return map[string]any{
		keyLocations: locMaps,
		keyPath:      locations[0].Path,
	}
}
