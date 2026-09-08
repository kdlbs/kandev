package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const (
	guardedTTYExecToolName    = "guarded_tty_exec_kandev"
	guardedTTYExecInputSchema = `{
	"type":"object",
	"properties":{
		"argv":{
			"type":"array",
			"description":"Command argv. The guarded execution derives cwd, sandbox, process identity, TTY mode, and limits.",
			"items":{"type":"string","minLength":1,"maxLength":4096},
			"minItems":1,
			"maxItems":64
		}
	},
	"required":["argv"],
	"additionalProperties":false
}`
)

func (s *Server) registerGuardedTTYExecTool() {
	s.mcpServer.AddTool(
		mcp.NewToolWithRawSchema(
			guardedTTYExecToolName,
			"Run one bounded command with a TTY inside this task's active guarded Codex execution.",
			json.RawMessage(guardedTTYExecInputSchema),
		),
		s.wrapHandler(guardedTTYExecToolName, s.guardedTTYExecHandler()),
	)
}

func (s *Server) guardedTTYExecHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		payload := map[string]interface{}{
			"task_id":    s.taskID,
			"session_id": s.sessionID,
			"argv":       req.GetArguments()["argv"],
		}
		var result map[string]interface{}
		if err := s.backend.RequestPayload(ctx, ws.ActionMCPGuardedTTYExec, payload, &result); err != nil {
			return mcp.NewToolResultError("Guarded TTY execution failed"), nil
		}
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to encode guarded TTY result: %v", err)), nil
		}
		return mcp.NewToolResultText(string(data)), nil
	}
}
