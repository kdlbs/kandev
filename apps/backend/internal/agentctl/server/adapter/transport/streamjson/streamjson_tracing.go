package streamjson

import (
	"context"
	"encoding/json"

	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
)

// traceIncomingControl traces and logs an incoming control-plane message.
// Used for control_request messages that bypass the regular handleMessage path.
func (a *Adapter) traceIncomingControl(eventType string, msg any) {
	if !shared.DebugEnabled() {
		return
	}
	rawData, _ := json.Marshal(msg)
	shared.LogRawEvent(shared.ProtocolStreamJSON, a.agentID, eventType, rawData)
	shared.TraceControlMessage(a.getPromptTraceCtx(), shared.ProtocolStreamJSON, a.agentID,
		"recv", eventType, rawData)
}

// traceOutgoingControl traces and logs an outgoing control-plane message.
// Used for control_response messages sent back to the Claude CLI.
func (a *Adapter) traceOutgoingControl(eventType string, msg any) {
	if !shared.DebugEnabled() {
		return
	}
	rawData, _ := json.Marshal(msg)
	shared.LogRawEvent(shared.ProtocolStreamJSON, a.agentID, eventType, rawData)
	shared.TraceControlMessage(a.getPromptTraceCtx(), shared.ProtocolStreamJSON, a.agentID,
		"send", eventType, rawData)
}

// traceOutgoingSend traces and logs an outgoing data-plane message to Claude Code.
// Uses the current prompt trace context as parent span.
func (a *Adapter) traceOutgoingSend(eventType string, msg any) {
	if !shared.DebugEnabled() {
		return
	}
	rawData, _ := json.Marshal(msg)
	shared.LogRawEvent(shared.ProtocolStreamJSON, a.agentID, "send."+eventType, rawData)
	shared.TraceControlMessage(a.getPromptTraceCtx(), shared.ProtocolStreamJSON, a.agentID,
		"send", eventType, rawData)
}

// traceOutgoingSendCtx is like traceOutgoingSend but uses an explicit context as parent span.
// Used for messages sent outside of a prompt lifecycle (e.g., initialize).
func (a *Adapter) traceOutgoingSendCtx(ctx context.Context, eventType string, msg any) {
	if !shared.DebugEnabled() {
		return
	}
	rawData, _ := json.Marshal(msg)
	shared.LogRawEvent(shared.ProtocolStreamJSON, a.agentID, "send."+eventType, rawData)
	shared.TraceControlMessage(ctx, shared.ProtocolStreamJSON, a.agentID,
		"send", eventType, rawData)
}
