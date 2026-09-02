package mcp

import (
	"context"
	"encoding/json"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// registerHandoffTaskTool registers handoff_task_kandev, the Office-only tool
// that creates a kanban delivery task in a workspace other than the caller's
// own (docs/specs/cross-workspace-task-handoff/spec.md). Deliberately not in
// handoff_handlers.go — that file already owns the unrelated same-workspace
// document-handoff tools (AC-2a).
func (s *Server) registerHandoffTaskTool() {
	s.mcpServer.AddTool(
		mcp.NewTool("handoff_task_kandev",
			mcp.WithDescription(`Create a delivery task in a DIFFERENT workspace, for a decision that must become work somewhere this agent does not run. target_workspace_id must differ from your own workspace — for same-workspace work use the Office runtime create path instead. Both agent_profile_id and executor_profile_id are required and are used exactly as supplied: nothing here is inherited or defaulted, unlike create_task_kandev. start_agent defaults to false, diverging from create_task_kandev, because starting an agent immediately in a workspace you do not run in is the riskiest available default; the card is startable later from the board regardless. Without external_id the call is NOT idempotent: derive a stable external_id from the deciding artefact (never from title) before retrying, rather than replaying blindly.`),
			mcp.WithString("target_workspace_id", mcp.Required(), mcp.Description("The workspace to create the delivery task in. Must differ from your own workspace.")),
			mcp.WithString("workflow_id", mcp.Required(), mcp.Description("A delivery workflow of target_workspace_id. Must not be that workspace's own office workflow.")),
			mcp.WithString("title", mcp.Required(), mcp.MaxLength(service.TaskTitleMaxLength), mcp.Description("A concise, few-word task title (maximum 60 characters).")),
			mcp.WithString("prompt", mcp.Required(), mcp.Description("The delivery agent's first user message. This is the ONLY context it receives when it starts.")),
			mcp.WithString("agent_profile_id", mcp.Required(), mcp.Description("Agent profile to run the delivery task. Must exist and be either global (no workspace) or scoped to target_workspace_id — a profile scoped to your own workspace is refused. Used exactly as supplied: no inheritance or defaulting.")),
			mcp.WithString("executor_profile_id", mcp.Required(), mcp.Description("Executor profile to run the delivery task. Must exist. Used exactly as supplied: no inheritance or defaulting.")),
			mcp.WithString("repository_id", mcp.Description("Optional. Must already exist in target_workspace_id. When omitted the delivery task is created with no repositories — none is inherited from your own task.")),
			mcp.WithString("base_branch", mcp.Description("Optional, and only valid together with repository_id. Defaults to that repository's default_branch when omitted.")),
			mcp.WithBoolean("start_agent", mcp.Description("Whether to start the delivery agent immediately. Default: false, diverging from create_task_kandev.")),
			mcp.WithString("external_id", mcp.Description("A stable identifier from your own system (e.g. derived from the deciding artefact). Creating a handoff twice with the same external_id in target_workspace_id returns the existing delivery task instead of making a duplicate. Without it the call is not idempotent — do not derive one from title, which changes freely between attempts.")),
		),
		s.wrapHandler("handoff_task_kandev", s.handoffTaskHandler()),
	)
}

func (s *Server) handoffTaskHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if s.taskID == "" || s.sessionID == "" {
			return mcp.NewToolResultError("handoff_task_kandev requires a bound task and session"), nil
		}
		args := req.GetArguments()
		payload := make(map[string]interface{}, len(args)+2)
		for key, value := range args {
			payload[key] = value
		}
		payload[mcpKeyTaskID] = s.taskID
		payload[mcpKeySessionID] = s.sessionID
		var result map[string]interface{}
		if err := s.backend.RequestPayload(ctx, ws.ActionMCPHandoffTask, payload, &result); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	}
}
