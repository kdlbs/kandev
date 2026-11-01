package main

import (
	"encoding/json"
	"strings"

	acp "github.com/coder/acp-go-sdk"
)

// shell_stream emits controlled ACP frames without executing a shell command.
func executeShellStream(e *emitter, line string) {
	var command struct {
		Action    string `json:"action"`
		ID        string `json:"id"`
		Command   string `json:"command"`
		SeedBytes int    `json:"seed_bytes"`
		Delta     string `json:"delta"`
		Stdout    string `json:"stdout"`
		Stderr    string `json:"stderr"`
		ExitCode  *int   `json:"exit_code"`
		Failed    bool   `json:"failed"`
	}
	if err := json.Unmarshal([]byte(extractParenContent(line, "e2e:shell_stream(")), &command); err != nil || command.ID == "" {
		e.text("Script error: invalid shell_stream command")
		return
	}
	id := acp.ToolCallId(command.ID)
	switch command.Action {
	case "start":
		if command.SeedBytes < 0 || command.SeedBytes > 262144 {
			e.text("Script error: invalid shell_stream seed size")
			return
		}
		e.startTool(id, command.Command, acp.ToolKindExecute, map[string]any{rawInputCommandKey: command.Command})
		emitShellStreamDelta(e, id, strings.Repeat("x", command.SeedBytes))
	case "delta":
		emitShellStreamDelta(e, id, command.Delta)
	case "finish":
		status := acp.ToolCallStatusCompleted
		if command.Failed {
			status = acp.ToolCallStatusFailed
		}
		_ = e.conn.SessionUpdate(e.ctx, acp.SessionNotification{SessionId: e.sid, Update: acp.UpdateToolCall(id,
			acp.WithUpdateStatus(status), acp.WithUpdateRawOutput(map[string]any{"stdout": command.Stdout, "stderr": command.Stderr, "exit_code": command.ExitCode}))})
	default:
		e.text("Script error: unknown shell_stream action")
	}
}

func emitShellStreamDelta(e *emitter, id acp.ToolCallId, delta string) {
	meta := func(update *acp.SessionToolCallUpdate) {
		update.Meta = map[string]any{"terminal_output_delta": map[string]any{"data": delta}}
	}
	_ = e.conn.SessionUpdate(e.ctx, acp.SessionNotification{SessionId: e.sid, Update: acp.UpdateToolCall(id,
		acp.WithUpdateStatus(acp.ToolCallStatusInProgress), meta)})
}
