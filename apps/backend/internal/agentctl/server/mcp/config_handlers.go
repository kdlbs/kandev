package mcp

import (
	"context"
	"encoding/json"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// Config-mode MCP action constants for backend WS requests.
// These must match the actions registered in pkg/websocket/actions.go.
const (
	ActionMCPCreateWorkflowStep  = "mcp.create_workflow_step"
	ActionMCPUpdateWorkflowStep  = "mcp.update_workflow_step"
	ActionMCPDeleteWorkflowStep  = "mcp.delete_workflow_step"
	ActionMCPReorderWorkflowStep = "mcp.reorder_workflow_steps"

	ActionMCPListAgents  = "mcp.list_agents"
	ActionMCPCreateAgent = "mcp.create_agent"
	ActionMCPUpdateAgent = "mcp.update_agent"
	ActionMCPDeleteAgent = "mcp.delete_agent"

	ActionMCPListAgentProfiles  = "mcp.list_agent_profiles"
	ActionMCPUpdateAgentProfile = "mcp.update_agent_profile"
	ActionMCPGetMcpConfig       = "mcp.get_mcp_config"
	ActionMCPUpdateMcpConfig    = "mcp.update_mcp_config"

	ActionMCPMoveTask        = "mcp.move_task"
	ActionMCPDeleteTask      = "mcp.delete_task"
	ActionMCPArchiveTask     = "mcp.archive_task"
	ActionMCPUpdateTaskState = "mcp.update_task_state"
)

// --- Workflow config tools ---

func (s *Server) registerConfigWorkflowTools() {
	s.mcpServer.AddTool(
		mcp.NewToolWithRawSchema("list_workspaces",
			"List all workspaces. Use this first to get workspace IDs.",
			json.RawMessage(`{"type":"object","properties":{}}`),
		),
		s.wrapHandler("list_workspaces", s.listWorkspacesHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("list_workflows",
			mcp.WithDescription("List all workflows in a workspace."),
			mcp.WithString("workspace_id", mcp.Required(), mcp.Description("The workspace ID")),
		),
		s.wrapHandler("list_workflows", s.listWorkflowsHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("list_workflow_steps",
			mcp.WithDescription("List all workflow steps (columns) in a workflow."),
			mcp.WithString("workflow_id", mcp.Required(), mcp.Description("The workflow ID")),
		),
		s.wrapHandler("list_workflow_steps", s.listWorkflowStepsHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("create_workflow_step",
			mcp.WithDescription("Create a new workflow step (column) in a workflow."),
			mcp.WithString("workflow_id", mcp.Required(), mcp.Description("The workflow ID")),
			mcp.WithString("name", mcp.Required(), mcp.Description("Step name")),
			mcp.WithNumber("position", mcp.Description("Step position (0-based). Defaults to end.")),
			mcp.WithString("color", mcp.Description("Step color hex code (e.g. '#3b82f6')")),
			mcp.WithString("prompt", mcp.Description("System prompt for agents in this step")),
			mcp.WithBoolean("is_start_step", mcp.Description("Whether this is the start step")),
		),
		s.wrapHandler("create_workflow_step", s.createWorkflowStepHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("update_workflow_step",
			mcp.WithDescription("Update an existing workflow step."),
			mcp.WithString("step_id", mcp.Required(), mcp.Description("The workflow step ID")),
			mcp.WithString("name", mcp.Description("New step name")),
			mcp.WithString("color", mcp.Description("New color hex code")),
			mcp.WithString("prompt", mcp.Description("New system prompt")),
			mcp.WithBoolean("is_start_step", mcp.Description("Whether this is the start step")),
		),
		s.wrapHandler("update_workflow_step", s.updateWorkflowStepHandler()),
	)
}

// --- Agent config tools ---

func (s *Server) registerConfigAgentTools() {
	s.mcpServer.AddTool(
		mcp.NewToolWithRawSchema("list_agents",
			"List all configured agents with their profiles.",
			json.RawMessage(`{"type":"object","properties":{}}`),
		),
		s.wrapHandler("list_agents", s.listAgentsHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("create_agent",
			mcp.WithDescription("Create a new agent configuration."),
			mcp.WithString("name", mcp.Required(), mcp.Description("Agent name")),
			mcp.WithString("workspace_id", mcp.Description("Optional workspace ID to scope the agent")),
		),
		s.wrapHandler("create_agent", s.createAgentHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("update_agent",
			mcp.WithDescription("Update an existing agent."),
			mcp.WithString("agent_id", mcp.Required(), mcp.Description("The agent ID")),
			mcp.WithBoolean("supports_mcp", mcp.Description("Whether the agent supports MCP")),
			mcp.WithString("mcp_config_path", mcp.Description("Path to MCP config file")),
		),
		s.wrapHandler("update_agent", s.updateAgentHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("delete_agent",
			mcp.WithDescription("Delete an agent and all its profiles."),
			mcp.WithString("agent_id", mcp.Required(), mcp.Description("The agent ID to delete")),
		),
		s.wrapHandler("delete_agent", s.deleteAgentHandler()),
	)
}

// --- MCP config tools ---

func (s *Server) registerConfigMcpTools() {
	s.mcpServer.AddTool(
		mcp.NewTool("list_agent_profiles",
			mcp.WithDescription("List all profiles for an agent."),
			mcp.WithString("agent_id", mcp.Required(), mcp.Description("The agent ID")),
		),
		s.wrapHandler("list_agent_profiles", s.listAgentProfilesHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("update_agent_profile",
			mcp.WithDescription("Update an agent profile's settings."),
			mcp.WithString("profile_id", mcp.Required(), mcp.Description("The profile ID")),
			mcp.WithString("name", mcp.Description("New profile name")),
			mcp.WithString("model", mcp.Description("New model name")),
			mcp.WithBoolean("auto_approve", mcp.Description("Auto-approve permissions")),
		),
		s.wrapHandler("update_agent_profile", s.updateAgentProfileHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("get_mcp_config",
			mcp.WithDescription("Get MCP server configuration for an agent profile."),
			mcp.WithString("profile_id", mcp.Required(), mcp.Description("The agent profile ID")),
		),
		s.wrapHandler("get_mcp_config", s.getMcpConfigHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("update_mcp_config",
			mcp.WithDescription("Update MCP server configuration for an agent profile. Pass the full servers map."),
			mcp.WithString("profile_id", mcp.Required(), mcp.Description("The agent profile ID")),
			mcp.WithBoolean("enabled", mcp.Description("Whether MCP is enabled for this profile")),
		),
		s.wrapHandler("update_mcp_config", s.updateMcpConfigHandler()),
	)
}

// --- Task config tools ---

func (s *Server) registerConfigTaskTools() {
	s.mcpServer.AddTool(
		mcp.NewTool("list_tasks",
			mcp.WithDescription("List all tasks in a workflow."),
			mcp.WithString("workflow_id", mcp.Required(), mcp.Description("The workflow ID")),
		),
		s.wrapHandler("list_tasks", s.listTasksHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("move_task",
			mcp.WithDescription("Move a task to a different workflow step."),
			mcp.WithString("task_id", mcp.Required(), mcp.Description("The task ID")),
			mcp.WithString("workflow_id", mcp.Required(), mcp.Description("Target workflow ID")),
			mcp.WithString("workflow_step_id", mcp.Required(), mcp.Description("Target workflow step ID")),
			mcp.WithNumber("position", mcp.Description("Position within the step (0-based)")),
		),
		s.wrapHandler("move_task", s.moveTaskHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("delete_task",
			mcp.WithDescription("Delete a task permanently."),
			mcp.WithString("task_id", mcp.Required(), mcp.Description("The task ID to delete")),
		),
		s.wrapHandler("delete_task", s.deleteTaskHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("archive_task",
			mcp.WithDescription("Archive a task."),
			mcp.WithString("task_id", mcp.Required(), mcp.Description("The task ID to archive")),
		),
		s.wrapHandler("archive_task", s.archiveTaskHandler()),
	)
}

// --- Handler implementations ---

func (s *Server) createWorkflowStepHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		workflowID, err := req.RequireString("workflow_id")
		if err != nil {
			return mcp.NewToolResultError("workflow_id is required"), nil
		}
		name, err := req.RequireString("name")
		if err != nil {
			return mcp.NewToolResultError("name is required"), nil
		}
		payload := map[string]interface{}{
			"workflow_id": workflowID,
			"name":        name,
		}
		if color := req.GetString("color", ""); color != "" {
			payload["color"] = color
		}
		if prompt := req.GetString("prompt", ""); prompt != "" {
			payload["prompt"] = prompt
		}
		if args := req.GetArguments(); args["position"] != nil {
			payload["position"] = args["position"]
		}
		if args := req.GetArguments(); args["is_start_step"] != nil {
			payload["is_start_step"] = args["is_start_step"]
		}
		return s.forwardToBackend(ctx, ActionMCPCreateWorkflowStep, payload)
	}
}

func (s *Server) updateWorkflowStepHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		stepID, err := req.RequireString("step_id")
		if err != nil {
			return mcp.NewToolResultError("step_id is required"), nil
		}
		payload := map[string]interface{}{"step_id": stepID}
		if name := req.GetString("name", ""); name != "" {
			payload["name"] = name
		}
		if color := req.GetString("color", ""); color != "" {
			payload["color"] = color
		}
		if prompt := req.GetString("prompt", ""); prompt != "" {
			payload["prompt"] = prompt
		}
		if args := req.GetArguments(); args["is_start_step"] != nil {
			payload["is_start_step"] = args["is_start_step"]
		}
		return s.forwardToBackend(ctx, ActionMCPUpdateWorkflowStep, payload)
	}
}

func (s *Server) listAgentsHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return s.forwardToBackend(ctx, ActionMCPListAgents, nil)
	}
}

func (s *Server) createAgentHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, err := req.RequireString("name")
		if err != nil {
			return mcp.NewToolResultError("name is required"), nil
		}
		payload := map[string]interface{}{"name": name}
		if wsID := req.GetString("workspace_id", ""); wsID != "" {
			payload["workspace_id"] = wsID
		}
		return s.forwardToBackend(ctx, ActionMCPCreateAgent, payload)
	}
}

func (s *Server) updateAgentHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		agentID, err := req.RequireString("agent_id")
		if err != nil {
			return mcp.NewToolResultError("agent_id is required"), nil
		}
		payload := map[string]interface{}{"agent_id": agentID}
		if args := req.GetArguments(); args["supports_mcp"] != nil {
			payload["supports_mcp"] = args["supports_mcp"]
		}
		if path := req.GetString("mcp_config_path", ""); path != "" {
			payload["mcp_config_path"] = path
		}
		return s.forwardToBackend(ctx, ActionMCPUpdateAgent, payload)
	}
}

func (s *Server) deleteAgentHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		agentID, err := req.RequireString("agent_id")
		if err != nil {
			return mcp.NewToolResultError("agent_id is required"), nil
		}
		payload := map[string]string{"agent_id": agentID}
		return s.forwardToBackend(ctx, ActionMCPDeleteAgent, payload)
	}
}

func (s *Server) listAgentProfilesHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		agentID, err := req.RequireString("agent_id")
		if err != nil {
			return mcp.NewToolResultError("agent_id is required"), nil
		}
		payload := map[string]string{"agent_id": agentID}
		return s.forwardToBackend(ctx, ActionMCPListAgentProfiles, payload)
	}
}

func (s *Server) updateAgentProfileHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		profileID, err := req.RequireString("profile_id")
		if err != nil {
			return mcp.NewToolResultError("profile_id is required"), nil
		}
		payload := map[string]interface{}{"profile_id": profileID}
		if name := req.GetString("name", ""); name != "" {
			payload["name"] = name
		}
		if model := req.GetString("model", ""); model != "" {
			payload["model"] = model
		}
		if args := req.GetArguments(); args["auto_approve"] != nil {
			payload["auto_approve"] = args["auto_approve"]
		}
		return s.forwardToBackend(ctx, ActionMCPUpdateAgentProfile, payload)
	}
}

func (s *Server) getMcpConfigHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		profileID, err := req.RequireString("profile_id")
		if err != nil {
			return mcp.NewToolResultError("profile_id is required"), nil
		}
		payload := map[string]string{"profile_id": profileID}
		return s.forwardToBackend(ctx, ActionMCPGetMcpConfig, payload)
	}
}

func (s *Server) updateMcpConfigHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		profileID, err := req.RequireString("profile_id")
		if err != nil {
			return mcp.NewToolResultError("profile_id is required"), nil
		}
		payload := map[string]interface{}{"profile_id": profileID}
		args := req.GetArguments()
		if args["enabled"] != nil {
			payload["enabled"] = args["enabled"]
		}
		if args["servers"] != nil {
			payload["servers"] = args["servers"]
		}
		return s.forwardToBackend(ctx, ActionMCPUpdateMcpConfig, payload)
	}
}

func (s *Server) moveTaskHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		taskID, err := req.RequireString("task_id")
		if err != nil {
			return mcp.NewToolResultError("task_id is required"), nil
		}
		workflowID, err := req.RequireString("workflow_id")
		if err != nil {
			return mcp.NewToolResultError("workflow_id is required"), nil
		}
		stepID, err := req.RequireString("workflow_step_id")
		if err != nil {
			return mcp.NewToolResultError("workflow_step_id is required"), nil
		}
		payload := map[string]interface{}{
			"task_id":          taskID,
			"workflow_id":      workflowID,
			"workflow_step_id": stepID,
		}
		if args := req.GetArguments(); args["position"] != nil {
			payload["position"] = args["position"]
		}
		return s.forwardToBackend(ctx, ActionMCPMoveTask, payload)
	}
}

func (s *Server) deleteTaskHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		taskID, err := req.RequireString("task_id")
		if err != nil {
			return mcp.NewToolResultError("task_id is required"), nil
		}
		payload := map[string]string{"task_id": taskID}
		return s.forwardToBackend(ctx, ActionMCPDeleteTask, payload)
	}
}

func (s *Server) archiveTaskHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		taskID, err := req.RequireString("task_id")
		if err != nil {
			return mcp.NewToolResultError("task_id is required"), nil
		}
		payload := map[string]string{"task_id": taskID}
		return s.forwardToBackend(ctx, ActionMCPArchiveTask, payload)
	}
}

// forwardToBackend sends a request to the backend and returns the result as JSON text.
func (s *Server) forwardToBackend(ctx context.Context, action string, payload interface{}) (*mcp.CallToolResult, error) {
	var result map[string]interface{}
	if err := s.backend.RequestPayload(ctx, action, payload, &result); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	data, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}
