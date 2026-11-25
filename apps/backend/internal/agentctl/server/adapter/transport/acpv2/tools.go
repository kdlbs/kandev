package acpv2

import (
	"encoding/json"
	"slices"

	acp "github.com/coder/acp-go-sdk"
	wire "github.com/coder/acp-go-sdk/experimental/v2"
	legacy "github.com/kandev/kandev/internal/agentctl/server/adapter/transport/acp"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

const (
	toolStatusRunning   = "running"
	toolStatusCompleted = "completed"
	toolStatusError     = "error"
	toolLocationPathKey = "path"
)

func applyPatch[T any](previous *acp.Optional[T], patch acp.Optional[T]) {
	if patch.IsSet() {
		*previous = patch
	}
}

func applyToolPatch(previous *wire.ToolCallUpdate, patch wire.ToolCallUpdate) {
	applyPatch(&previous.Name, patch.Name)
	applyPatch(&previous.Title, patch.Title)
	applyPatch(&previous.Kind, patch.Kind)
	applyPatch(&previous.Status, patch.Status)
	applyPatch(&previous.Content, patch.Content)
	applyPatch(&previous.Locations, patch.Locations)
	applyPatch(&previous.RawInput, patch.RawInput)
	applyPatch(&previous.RawOutput, patch.RawOutput)
	applyPatch(&previous.Capabilities, patch.Capabilities)
	applyPatch(&previous.Meta, patch.Meta)
}

func (a *Adapter) projectTool(
	state *projection, session wire.SessionID, patch wire.ToolCallUpdate,
	chunk *wire.ToolCallContent, event *streams.AgentEvent, live bool,
) bool {
	if patch.ToolCallID == "" {
		return false
	}
	id := entityID(session, patch.ToolCallID)
	old, known := state.tools[id]
	applyToolPatch(&old, patch)
	old.ToolCallID = patch.ToolCallID
	content, _ := old.Content.Get()
	if chunk != nil {
		content = append(slices.Clone(content), *chunk)
		old.Content = acp.OptionalValue(content)
	}
	if state.tools == nil {
		state.tools = make(map[string]wire.ToolCallUpdate)
	}
	state.tools[id] = old
	for _, item := range content {
		if item.SessionID != nil {
			state.bindChild(*item.SessionID, id)
		}
	}
	if !live {
		return false
	}
	a.setToolEvent(event, old, content, id, known)
	return true
}

func (a *Adapter) setToolEvent(
	event *streams.AgentEvent, tool wire.ToolCallUpdate, content []wire.ToolCallContent, id string, known bool,
) {
	name, _ := tool.Name.Get()
	title, _ := tool.Title.Get()
	kind, _ := tool.Kind.Get()
	status, hasStatus := tool.Status.Get()
	locations, _ := tool.Locations.Get()
	rawInput, _ := tool.RawInput.Get()
	rawOutput, _ := tool.RawOutput.Get()
	meta, _ := tool.Meta.Get()
	var input, metadata map[string]any
	var output any
	_ = json.Unmarshal(rawInput, &input)
	_ = json.Unmarshal(rawOutput, &output)
	_ = json.Unmarshal(meta, &metadata)
	args := map[string]any{
		"title": title, "kind": kind, "raw_input": input, "_meta": metadata,
	}
	if len(locations) != 0 {
		locationArgs := make([]map[string]any, len(locations))
		for i, location := range locations {
			locationArgs[i] = map[string]any{toolLocationPathKey: location.Path}
			if location.Line != nil {
				locationArgs[i]["line"] = *location.Line
			}
		}
		args["locations"] = locationArgs
	}
	normalizer := legacy.NewNormalizer(a.cfg.AgentID)
	payload := normalizer.NormalizeToolCall(name, args)
	if output != nil {
		normalizer.NormalizeToolResult(payload, output)
	}
	event.Type = toolEventType(known)
	event.ToolCallID, event.ToolName, event.ToolTitle = id, name, title
	event.ToolStatus, event.ToolStatusMissing = toolStatus(status), !hasStatus
	event.NormalizedPayload = payload
	for _, item := range content {
		if item.Content != nil {
			event.ToolCallContents = append(event.ToolCallContents, streams.ToolCallContentItem{
				Type: "content", Content: projectedContent(item.Content),
			})
		}
	}
}

func projectedContent(block *wire.ContentBlock) *streams.ContentBlock {
	return &streams.ContentBlock{
		Type: block.Type, Text: valueOrZero(block.Text), Data: valueOrZero(block.Data),
		MimeType: valueOrZero(block.MIMEType), URI: valueOrZero(block.URI), Name: valueOrZero(block.Name),
		Title: valueOrZero(block.Title), Description: valueOrZero(block.Description),
	}
}

func toolStatus(status string) string {
	switch status {
	case "in_progress":
		return toolStatusRunning
	case toolStatusCompleted:
		return toolStatusCompleted
	case "failed":
		return toolStatusError
	case "pending":
		return "started"
	default:
		return ""
	}
}

func toolEventType(known bool) string {
	if known {
		return streams.EventTypeToolUpdate
	}
	return streams.EventTypeToolCall
}

func (p *projection) bindChild(session wire.SessionID, parent string) {
	if p.children == nil {
		p.children = make(map[wire.SessionID]string)
	}
	p.children[session] = parent
}

func (p *projection) projectSubagent(parent wire.SessionID, update *wire.SubagentUpdate, event *streams.AgentEvent, live bool) bool {
	if update.SessionID == "" || update.SessionID == parent || string(update.SessionID) == event.SessionID {
		return false
	}
	id, known := p.children[update.SessionID]
	if !known {
		id = entityID(parent, "subagent:"+string(update.SessionID))
		p.bindChild(update.SessionID, id)
	}
	if p.subagents == nil {
		p.subagents = make(map[wire.SessionID]wire.SubagentUpdate)
	}
	snapshot := p.subagents[update.SessionID]
	applyPatch(&snapshot.Title, update.Title)
	applyPatch(&snapshot.Description, update.Description)
	applyPatch(&snapshot.State, update.State)
	p.subagents[update.SessionID] = snapshot
	if !live {
		return false
	}
	title, _ := snapshot.Title.Get()
	description, _ := snapshot.Description.Get()
	status, _ := snapshot.State.Get()
	event.Type, event.ToolCallID, event.ToolName, event.ToolTitle = toolEventType(known), id, "Task", title
	event.NormalizedPayload = streams.NewSubagentTask(title, description, "")
	switch status.State {
	case wire.StateIdle:
		event.ToolStatus = toolStatusCompleted
	case wire.StateRunning:
		event.ToolStatus = toolStatusRunning
	default:
		event.ToolStatusMissing = true
	}
	return true
}

type terminalState struct {
	command, cwd, output string
	exit                 acp.Optional[wire.TerminalExitStatus]
	truncated            bool
}

func (p *projection) projectTerminal(
	session wire.SessionID, update wire.TerminalUpdate, chunk string, event *streams.AgentEvent, live bool,
) bool {
	const maxTerminalOutput = 64 << 10
	id := entityID(session, "terminal:"+string(update.TerminalID))
	old, known := p.terminals[id]
	if update.Cwd.IsSet() {
		old.cwd, _ = update.Cwd.Get()
	}
	if update.Command.IsSet() {
		old.command, _ = update.Command.Get()
	}
	if update.Output.IsSet() {
		output, _ := update.Output.Get()
		old.output, old.truncated = output.Data, false
	}
	old.output += chunk
	if len(old.output) > maxTerminalOutput {
		old.output = old.output[len(old.output)-maxTerminalOutput:]
		old.truncated = true
	}
	applyPatch(&old.exit, update.ExitStatus)
	if p.terminals == nil {
		p.terminals = make(map[string]terminalState)
	}
	p.terminals[id] = old
	if !live {
		return false
	}
	event.Type, event.ToolCallID, event.ToolName, event.ToolTitle = toolEventType(known), id, "terminal", old.command
	event.ToolStatus = toolStatusRunning
	payload := streams.NewShellExec(old.command, old.cwd, "", 0, false)
	payload.ShellExec().Output = &streams.ShellExecOutput{Stdout: old.output, Truncated: old.truncated}
	if exit, ok := old.exit.Get(); ok {
		event.ToolStatus = toolStatusCompleted
		if exit.ExitCode != nil {
			code := int(*exit.ExitCode)
			payload.ShellExec().Output.ExitCode = &code
		}
	}
	event.NormalizedPayload = payload
	return true
}
