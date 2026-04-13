package mcp

import (
	"context"
	"encoding/json"

	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// --- Workflow config tools ---

func (s *Server) registerConfigWorkflowTools() {
	s.mcpServer.AddTool(
		mcp.NewToolWithRawSchema("list_workspaces_kandev",
			"List all workspaces. Use this first to get workspace IDs.",
			json.RawMessage(`{"type":"object","properties":{}}`),
		),
		s.wrapHandler("list_workspaces_kandev", s.listWorkspacesHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("list_workflows_kandev",
			mcp.WithDescription("List all workflows in a workspace."),
			mcp.WithString("workspace_id", mcp.Required(), mcp.Description("The workspace ID")),
		),
		s.wrapHandler("list_workflows_kandev", s.listWorkflowsHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("create_workflow_kandev",
			mcp.WithDescription("Create a new workflow in a workspace."),
			mcp.WithString("workspace_id", mcp.Required(), mcp.Description("The workspace ID")),
			mcp.WithString("name", mcp.Required(), mcp.Description("Workflow name")),
			mcp.WithString("description", mcp.Description("Workflow description")),
		),
		s.wrapHandler("create_workflow_kandev", s.createWorkflowHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("update_workflow_kandev",
			mcp.WithDescription("Update an existing workflow."),
			mcp.WithString("workflow_id", mcp.Required(), mcp.Description("The workflow ID")),
			mcp.WithString("name", mcp.Description("New workflow name")),
			mcp.WithString("description", mcp.Description("New workflow description")),
		),
		s.wrapHandler("update_workflow_kandev", s.updateWorkflowHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("delete_workflow_kandev",
			mcp.WithDescription("Delete a workflow and all its steps. This is destructive and cannot be undone."),
			mcp.WithString("workflow_id", mcp.Required(), mcp.Description("The workflow ID to delete")),
		),
		s.wrapHandler("delete_workflow_kandev", s.deleteWorkflowHandler()),
	)
	s.registerConfigWorkflowStepTools()
}

func (s *Server) registerConfigWorkflowStepTools() {
	s.mcpServer.AddTool(
		mcp.NewTool("list_workflow_steps_kandev",
			mcp.WithDescription("List all workflow steps (columns) in a workflow."),
			mcp.WithString("workflow_id", mcp.Required(), mcp.Description("The workflow ID")),
		),
		s.wrapHandler("list_workflow_steps_kandev", s.listWorkflowStepsHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("create_workflow_step_kandev",
			mcp.WithDescription("Create a new workflow step (column) in a workflow."),
			mcp.WithString("workflow_id", mcp.Required(), mcp.Description("The workflow ID")),
			mcp.WithString("name", mcp.Required(), mcp.Description("Step name")),
			mcp.WithNumber("position", mcp.Description("Step position (0-based). Defaults to end.")),
			mcp.WithString("color", mcp.Description("Step color hex code (e.g. '#3b82f6')")),
			mcp.WithString("prompt", mcp.Description("System prompt for agents in this step")),
			mcp.WithBoolean("is_start_step", mcp.Description("Whether this is the start step")),
			mcp.WithBoolean("allow_manual_move", mcp.Description("Allow manual task moves into this step (default: false)")),
			mcp.WithBoolean("show_in_command_panel", mcp.Description("Show this step in the command panel")),
			mcp.WithObject("events", mcp.Description("Event-driven actions. Keys: on_enter, on_exit, on_turn_start, on_turn_complete. Each is an array of {type, config} objects.")),
		),
		s.wrapHandler("create_workflow_step_kandev", s.createWorkflowStepHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("update_workflow_step_kandev",
			mcp.WithDescription("Update an existing workflow step."),
			mcp.WithString("step_id", mcp.Required(), mcp.Description("The workflow step ID")),
			mcp.WithString("name", mcp.Description("New step name")),
			mcp.WithString("color", mcp.Description("New color hex code")),
			mcp.WithString("prompt", mcp.Description("New system prompt")),
			mcp.WithBoolean("is_start_step", mcp.Description("Whether this is the start step")),
			mcp.WithBoolean("allow_manual_move", mcp.Description("Allow manual task moves into this step")),
			mcp.WithBoolean("show_in_command_panel", mcp.Description("Show this step in the command panel")),
			mcp.WithNumber("auto_archive_after_hours", mcp.Description("Auto-archive tasks after N hours in this step (0 to disable)")),
			mcp.WithObject("events", mcp.Description("Event-driven actions. Keys: on_enter, on_exit, on_turn_start, on_turn_complete.")),
		),
		s.wrapHandler("update_workflow_step_kandev", s.updateWorkflowStepHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("delete_workflow_step_kandev",
			mcp.WithDescription("Delete a workflow step. This is destructive and cannot be undone."),
			mcp.WithString("step_id", mcp.Required(), mcp.Description("The workflow step ID to delete")),
		),
		s.wrapHandler("delete_workflow_step_kandev", s.deleteWorkflowStepHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("reorder_workflow_steps_kandev",
			mcp.WithDescription("Reorder workflow steps by providing the full ordered list of step IDs."),
			mcp.WithString("workflow_id", mcp.Required(), mcp.Description("The workflow ID")),
			mcp.WithArray("step_ids", mcp.Required(), mcp.Description("Ordered list of step IDs defining the new order")),
		),
		s.wrapHandler("reorder_workflow_steps_kandev", s.reorderWorkflowStepsHandler()),
	)
}

// --- Agent config tools ---

func (s *Server) registerConfigAgentTools() {
	s.mcpServer.AddTool(
		mcp.NewToolWithRawSchema("list_agents_kandev",
			"List all configured agents with their profiles.",
			json.RawMessage(`{"type":"object","properties":{}}`),
		),
		s.wrapHandler("list_agents_kandev", s.listAgentsHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("update_agent_kandev",
			mcp.WithDescription("Update an existing agent."),
			mcp.WithString("agent_id", mcp.Required(), mcp.Description("The agent ID")),
			mcp.WithBoolean("supports_mcp", mcp.Description("Whether the agent supports MCP")),
			mcp.WithString("mcp_config_path", mcp.Description("Path to MCP config file")),
		),
		s.wrapHandler("update_agent_kandev", s.updateAgentHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("create_agent_profile_kandev",
			mcp.WithDescription("Create a new agent profile for an agent."),
			mcp.WithString("agent_id", mcp.Required(), mcp.Description("The agent ID to create a profile for")),
			mcp.WithString("name", mcp.Required(), mcp.Description("Profile name")),
			mcp.WithString("model", mcp.Required(), mcp.Description("Model name (e.g. 'claude-sonnet-4-5-20250514')")),
			mcp.WithBoolean("auto_approve", mcp.Description("Auto-approve permissions (default: false)")),
		),
		s.wrapHandler("create_agent_profile_kandev", s.createAgentProfileHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("delete_agent_profile_kandev",
			mcp.WithDescription("Delete an agent profile. Fails if the profile is used by an active session."),
			mcp.WithString("profile_id", mcp.Required(), mcp.Description("The profile ID to delete")),
		),
		s.wrapHandler("delete_agent_profile_kandev", s.deleteAgentProfileHandler()),
	)
}

// --- MCP config tools ---

func (s *Server) registerConfigMcpTools() {
	s.mcpServer.AddTool(
		mcp.NewTool("list_agent_profiles_kandev",
			mcp.WithDescription("List all profiles for an agent."),
			mcp.WithString("agent_id", mcp.Required(), mcp.Description("The agent ID")),
		),
		s.wrapHandler("list_agent_profiles_kandev", s.listAgentProfilesHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("update_agent_profile_kandev",
			mcp.WithDescription("Update an agent profile's settings."),
			mcp.WithString("profile_id", mcp.Required(), mcp.Description("The profile ID")),
			mcp.WithString("name", mcp.Description("New profile name")),
			mcp.WithString("model", mcp.Description("New model name")),
			mcp.WithBoolean("auto_approve", mcp.Description("Auto-approve permissions")),
		),
		s.wrapHandler("update_agent_profile_kandev", s.updateAgentProfileHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("get_mcp_config_kandev",
			mcp.WithDescription("Get MCP server configuration for an agent profile."),
			mcp.WithString("profile_id", mcp.Required(), mcp.Description("The agent profile ID")),
		),
		s.wrapHandler("get_mcp_config_kandev", s.getMcpConfigHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("update_mcp_config_kandev",
			mcp.WithDescription("Update MCP server configuration for an agent profile. Pass the full servers map."),
			mcp.WithString("profile_id", mcp.Required(), mcp.Description("The agent profile ID")),
			mcp.WithBoolean("enabled", mcp.Description("Whether MCP is enabled for this profile")),
			mcp.WithObject("servers", mcp.Description("Full MCP servers map to set. Each key is a server name, value is the server configuration object.")),
		),
		s.wrapHandler("update_mcp_config_kandev", s.updateMcpConfigHandler()),
	)
}

// --- Executor config tools ---

func (s *Server) registerConfigExecutorTools() {
	s.mcpServer.AddTool(
		mcp.NewToolWithRawSchema("list_executors_kandev",
			"List all executors.",
			json.RawMessage(`{"type":"object","properties":{}}`),
		),
		s.wrapHandler("list_executors_kandev", s.listExecutorsHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("list_executor_profiles_kandev",
			mcp.WithDescription("List all profiles for an executor."),
			mcp.WithString("executor_id", mcp.Required(), mcp.Description("The executor ID")),
		),
		s.wrapHandler("list_executor_profiles_kandev", s.listExecutorProfilesHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("create_executor_profile_kandev",
			mcp.WithDescription("Create a new executor profile."),
			mcp.WithString("executor_id", mcp.Required(), mcp.Description("The executor ID")),
			mcp.WithString("name", mcp.Required(), mcp.Description("Profile name")),
			mcp.WithString("mcp_policy", mcp.Description("MCP policy for this profile")),
			mcp.WithObject("config", mcp.Description("Key-value configuration map")),
			mcp.WithString("prepare_script", mcp.Description("Script to run before agent starts")),
			mcp.WithString("cleanup_script", mcp.Description("Script to run after agent stops")),
		),
		s.wrapHandler("create_executor_profile_kandev", s.createExecutorProfileHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("update_executor_profile_kandev",
			mcp.WithDescription("Update an existing executor profile."),
			mcp.WithString("profile_id", mcp.Required(), mcp.Description("The executor profile ID")),
			mcp.WithString("name", mcp.Description("New profile name")),
			mcp.WithString("mcp_policy", mcp.Description("New MCP policy")),
			mcp.WithObject("config", mcp.Description("New configuration map")),
			mcp.WithString("prepare_script", mcp.Description("New prepare script")),
			mcp.WithString("cleanup_script", mcp.Description("New cleanup script")),
		),
		s.wrapHandler("update_executor_profile_kandev", s.updateExecutorProfileHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("delete_executor_profile_kandev",
			mcp.WithDescription("Delete an executor profile."),
			mcp.WithString("profile_id", mcp.Required(), mcp.Description("The executor profile ID to delete")),
		),
		s.wrapHandler("delete_executor_profile_kandev", s.deleteExecutorProfileHandler()),
	)
}

// --- Task config tools ---

func (s *Server) registerConfigTaskTools() {
	s.mcpServer.AddTool(
		mcp.NewTool("list_tasks_kandev",
			mcp.WithDescription("List all tasks in a workflow."),
			mcp.WithString("workflow_id", mcp.Required(), mcp.Description("The workflow ID")),
		),
		s.wrapHandler("list_tasks_kandev", s.listTasksHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("move_task_kandev",
			mcp.WithDescription("Move a task to a different workflow step."),
			mcp.WithString("task_id", mcp.Required(), mcp.Description("The task ID")),
			mcp.WithString("workflow_id", mcp.Required(), mcp.Description("Target workflow ID")),
			mcp.WithString("workflow_step_id", mcp.Required(), mcp.Description("Target workflow step ID")),
			mcp.WithNumber("position", mcp.Description("Position within the step (0-based)")),
		),
		s.wrapHandler("move_task_kandev", s.moveTaskHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("delete_task_kandev",
			mcp.WithDescription("Delete a task permanently."),
			mcp.WithString("task_id", mcp.Required(), mcp.Description("The task ID to delete")),
		),
		s.wrapHandler("delete_task_kandev", s.deleteTaskHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("archive_task_kandev",
			mcp.WithDescription("Archive a task."),
			mcp.WithString("task_id", mcp.Required(), mcp.Description("The task ID to archive")),
		),
		s.wrapHandler("archive_task_kandev", s.archiveTaskHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("update_task_state_kandev",
			mcp.WithDescription("Update the state of a task."),
			mcp.WithString("task_id", mcp.Required(), mcp.Description("The task ID")),
			mcp.WithString("state", mcp.Required(), mcp.Description("New state: open, in_progress, complete, blocked, cancelled")),
		),
		s.wrapHandler("update_task_state_kandev", s.updateTaskStateHandler()),
	)
}

// --- Handler implementations ---

func (s *Server) createWorkflowHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		workspaceID, err := req.RequireString("workspace_id")
		if err != nil {
			return mcp.NewToolResultError("workspace_id is required"), nil
		}
		name, err := req.RequireString("name")
		if err != nil {
			return mcp.NewToolResultError("name is required"), nil
		}
		payload := map[string]interface{}{
			"workspace_id": workspaceID,
			"name":         name,
		}
		if desc := req.GetString("description", ""); desc != "" {
			payload["description"] = desc
		}
		return s.forwardToBackend(ctx, ws.ActionMCPCreateWorkflow, payload)
	}
}

func (s *Server) updateWorkflowHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		workflowID, err := req.RequireString("workflow_id")
		if err != nil {
			return mcp.NewToolResultError("workflow_id is required"), nil
		}
		payload := map[string]interface{}{"workflow_id": workflowID}
		if name := req.GetString("name", ""); name != "" {
			payload["name"] = name
		}
		if desc := req.GetString("description", ""); desc != "" {
			payload["description"] = desc
		}
		return s.forwardToBackend(ctx, ws.ActionMCPUpdateWorkflow, payload)
	}
}

func (s *Server) deleteWorkflowHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		workflowID, err := req.RequireString("workflow_id")
		if err != nil {
			return mcp.NewToolResultError("workflow_id is required"), nil
		}
		return s.forwardToBackend(ctx, ws.ActionMCPDeleteWorkflow, map[string]string{"workflow_id": workflowID})
	}
}

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
		args := req.GetArguments()
		for _, key := range []string{"position", "is_start_step", "allow_manual_move", "show_in_command_panel", "events"} {
			if args[key] != nil {
				payload[key] = args[key]
			}
		}
		return s.forwardToBackend(ctx, ws.ActionMCPCreateWorkflowStep, payload)
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
		args := req.GetArguments()
		for _, key := range []string{"is_start_step", "allow_manual_move", "show_in_command_panel", "auto_archive_after_hours", "events"} {
			if args[key] != nil {
				payload[key] = args[key]
			}
		}
		return s.forwardToBackend(ctx, ws.ActionMCPUpdateWorkflowStep, payload)
	}
}

func (s *Server) listAgentsHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return s.forwardToBackend(ctx, ws.ActionMCPListAgents, nil)
	}
}

func (s *Server) createAgentProfileHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		agentID, err := req.RequireString("agent_id")
		if err != nil {
			return mcp.NewToolResultError("agent_id is required"), nil
		}
		name, err := req.RequireString("name")
		if err != nil {
			return mcp.NewToolResultError("name is required"), nil
		}
		model, err := req.RequireString("model")
		if err != nil {
			return mcp.NewToolResultError("model is required"), nil
		}
		payload := map[string]interface{}{
			"agent_id": agentID,
			"name":     name,
			"model":    model,
		}
		if args := req.GetArguments(); args["auto_approve"] != nil {
			payload["auto_approve"] = args["auto_approve"]
		}
		return s.forwardToBackend(ctx, ws.ActionMCPCreateAgentProfile, payload)
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
		return s.forwardToBackend(ctx, ws.ActionMCPUpdateAgent, payload)
	}
}

func (s *Server) deleteAgentProfileHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		profileID, err := req.RequireString("profile_id")
		if err != nil {
			return mcp.NewToolResultError("profile_id is required"), nil
		}
		payload := map[string]string{"profile_id": profileID}
		return s.forwardToBackend(ctx, ws.ActionMCPDeleteAgentProfile, payload)
	}
}

func (s *Server) listAgentProfilesHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		agentID, err := req.RequireString("agent_id")
		if err != nil {
			return mcp.NewToolResultError("agent_id is required"), nil
		}
		payload := map[string]string{"agent_id": agentID}
		return s.forwardToBackend(ctx, ws.ActionMCPListAgentProfiles, payload)
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
		return s.forwardToBackend(ctx, ws.ActionMCPUpdateAgentProfile, payload)
	}
}

func (s *Server) getMcpConfigHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		profileID, err := req.RequireString("profile_id")
		if err != nil {
			return mcp.NewToolResultError("profile_id is required"), nil
		}
		payload := map[string]string{"profile_id": profileID}
		return s.forwardToBackend(ctx, ws.ActionMCPGetMcpConfig, payload)
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
		return s.forwardToBackend(ctx, ws.ActionMCPUpdateMcpConfig, payload)
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
		return s.forwardToBackend(ctx, ws.ActionMCPMoveTask, payload)
	}
}

func (s *Server) deleteTaskHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		taskID, err := req.RequireString("task_id")
		if err != nil {
			return mcp.NewToolResultError("task_id is required"), nil
		}
		payload := map[string]string{"task_id": taskID}
		return s.forwardToBackend(ctx, ws.ActionMCPDeleteTask, payload)
	}
}

func (s *Server) updateTaskStateHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		taskID, err := req.RequireString("task_id")
		if err != nil {
			return mcp.NewToolResultError("task_id is required"), nil
		}
		state, err := req.RequireString("state")
		if err != nil {
			return mcp.NewToolResultError("state is required"), nil
		}
		payload := map[string]string{"task_id": taskID, "state": state}
		return s.forwardToBackend(ctx, ws.ActionMCPUpdateTaskState, payload)
	}
}

func (s *Server) deleteWorkflowStepHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		stepID, err := req.RequireString("step_id")
		if err != nil {
			return mcp.NewToolResultError("step_id is required"), nil
		}
		return s.forwardToBackend(ctx, ws.ActionMCPDeleteWorkflowStep, map[string]string{"step_id": stepID})
	}
}

func (s *Server) reorderWorkflowStepsHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		workflowID, err := req.RequireString("workflow_id")
		if err != nil {
			return mcp.NewToolResultError("workflow_id is required"), nil
		}
		args := req.GetArguments()
		stepIDs := args["step_ids"]
		if stepIDs == nil {
			return mcp.NewToolResultError("step_ids is required"), nil
		}
		payload := map[string]interface{}{
			"workflow_id": workflowID,
			"step_ids":    stepIDs,
		}
		return s.forwardToBackend(ctx, ws.ActionMCPReorderWorkflowStep, payload)
	}
}

func (s *Server) archiveTaskHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		taskID, err := req.RequireString("task_id")
		if err != nil {
			return mcp.NewToolResultError("task_id is required"), nil
		}
		payload := map[string]string{"task_id": taskID}
		return s.forwardToBackend(ctx, ws.ActionMCPArchiveTask, payload)
	}
}

// --- Executor handler implementations ---

func (s *Server) listExecutorsHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return s.forwardToBackend(ctx, ws.ActionMCPListExecutors, nil)
	}
}

func (s *Server) listExecutorProfilesHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		executorID, err := req.RequireString("executor_id")
		if err != nil {
			return mcp.NewToolResultError("executor_id is required"), nil
		}
		return s.forwardToBackend(ctx, ws.ActionMCPListExecutorProfiles, map[string]string{"executor_id": executorID})
	}
}

func (s *Server) createExecutorProfileHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		executorID, err := req.RequireString("executor_id")
		if err != nil {
			return mcp.NewToolResultError("executor_id is required"), nil
		}
		name, err := req.RequireString("name")
		if err != nil {
			return mcp.NewToolResultError("name is required"), nil
		}
		payload := map[string]interface{}{
			"executor_id": executorID,
			"name":        name,
		}
		if mcpPolicy := req.GetString("mcp_policy", ""); mcpPolicy != "" {
			payload["mcp_policy"] = mcpPolicy
		}
		if prepareScript := req.GetString("prepare_script", ""); prepareScript != "" {
			payload["prepare_script"] = prepareScript
		}
		if cleanupScript := req.GetString("cleanup_script", ""); cleanupScript != "" {
			payload["cleanup_script"] = cleanupScript
		}
		args := req.GetArguments()
		if args["config"] != nil {
			payload["config"] = args["config"]
		}
		return s.forwardToBackend(ctx, ws.ActionMCPCreateExecutorProfile, payload)
	}
}

func (s *Server) updateExecutorProfileHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		profileID, err := req.RequireString("profile_id")
		if err != nil {
			return mcp.NewToolResultError("profile_id is required"), nil
		}
		payload := map[string]interface{}{"profile_id": profileID}
		if name := req.GetString("name", ""); name != "" {
			payload["name"] = name
		}
		if mcpPolicy := req.GetString("mcp_policy", ""); mcpPolicy != "" {
			payload["mcp_policy"] = mcpPolicy
		}
		if prepareScript := req.GetString("prepare_script", ""); prepareScript != "" {
			payload["prepare_script"] = prepareScript
		}
		if cleanupScript := req.GetString("cleanup_script", ""); cleanupScript != "" {
			payload["cleanup_script"] = cleanupScript
		}
		args := req.GetArguments()
		if args["config"] != nil {
			payload["config"] = args["config"]
		}
		return s.forwardToBackend(ctx, ws.ActionMCPUpdateExecutorProfile, payload)
	}
}

func (s *Server) deleteExecutorProfileHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		profileID, err := req.RequireString("profile_id")
		if err != nil {
			return mcp.NewToolResultError("profile_id is required"), nil
		}
		return s.forwardToBackend(ctx, ws.ActionMCPDeleteExecutorProfile, map[string]string{"profile_id": profileID})
	}
}

// forwardToBackend sends a request to the backend and returns the result as JSON text.
func (s *Server) forwardToBackend(ctx context.Context, action string, payload interface{}) (*mcp.CallToolResult, error) {
	var result map[string]interface{}
	if err := s.backend.RequestPayload(ctx, action, payload, &result); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return mcp.NewToolResultError("failed to marshal response: " + err.Error()), nil
	}
	return mcp.NewToolResultText(string(data)), nil
}
