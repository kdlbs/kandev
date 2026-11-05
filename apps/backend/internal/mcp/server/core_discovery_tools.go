package mcp

import (
	"encoding/json"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/mark3labs/mcp-go/mcp"
)

func (s *Server) registerListWorkspacesTool() {
	s.mcpServer.AddTool(
		mcp.NewToolWithRawSchema("list_workspaces_kandev",
			"List all workspaces. Use this first to get workspace IDs.",
			json.RawMessage(`{"type":"object","properties":{}}`),
		),
		s.wrapHandler("list_workspaces_kandev", s.listWorkspacesHandler()),
	)
}

func (s *Server) registerListWorkflowsTool() {
	s.mcpServer.AddTool(
		mcp.NewTool("list_workflows_kandev",
			mcp.WithDescription("List all workflows in a workspace."),
			mcp.WithString("workspace_id", mcp.Required(), mcp.Description("The workspace ID")),
		),
		s.wrapHandler("list_workflows_kandev", s.listWorkflowsHandler()),
	)
}

func (s *Server) registerListWorkflowStepsTool() {
	description := "List all workflow steps (columns) in a workflow."
	if s.profile.Surface == mcpprofile.SurfaceKanbanTask {
		description = "List all workflow steps in a workflow."
	}
	s.mcpServer.AddTool(
		mcp.NewTool("list_workflow_steps_kandev",
			mcp.WithDescription(description),
			mcp.WithString("workflow_id", mcp.Required(), mcp.Description("The workflow ID")),
		),
		s.wrapHandler("list_workflow_steps_kandev", s.listWorkflowStepsHandler()),
	)
}

func (s *Server) registerListRepositoriesTool() {
	s.mcpServer.AddTool(
		mcp.NewTool("list_repositories_kandev",
			mcp.WithDescription("List repositories in a workspace. Use this to find a repository_id to attach to create_task_kandev when the task should target a specific codebase. Each repository entry includes default_branch — use it to pick or confirm a base_branch when creating a subtask in a different repo."),
			mcp.WithString("workspace_id", mcp.Required(), mcp.Description("The workspace ID")),
		),
		s.wrapHandler("list_repositories_kandev", s.listRepositoriesHandler()),
	)
}

func (s *Server) registerListAgentsTool() {
	description := "List all configured agents with their profiles."
	if s.profile.Surface == mcpprofile.SurfaceKanbanTask || s.profile.Surface == mcpprofile.SurfaceAutomation {
		description = "List all configured agents with their profiles. Use this to find available agent_profile_ids for create_task_kandev and spawn_session_kandev."
	}
	s.mcpServer.AddTool(
		mcp.NewToolWithRawSchema("list_agents_kandev",
			description,
			json.RawMessage(`{"type":"object","properties":{}}`),
		),
		s.wrapHandler("list_agents_kandev", s.listAgentsHandler()),
	)
}

func (s *Server) registerListExecutorsTool() {
	listTool := mcp.NewToolWithRawSchema("list_executors_kandev",
		"List every executor available to the authenticated caller, including custom executor IDs, with its visible profiles. Use a returned profile ID as executor_profile_id in create_task_kandev. Visibility does not grant authority to launch later; task creation and executor admission check current ownership.",
		json.RawMessage(`{"type":"object","properties":{}}`),
	)
	// NewToolWithRawSchema does not accept option functions, so set annotations directly.
	listTool.Annotations.ReadOnlyHint = mcp.ToBoolPtr(true)
	s.mcpServer.AddTool(
		listTool,
		s.wrapHandler("list_executors_kandev", s.listExecutorsHandler()),
	)
}

func (s *Server) registerListExecutorProfilesTool() {
	description := "List all profiles for an executor."
	if s.profile.Surface == mcpprofile.SurfaceKanbanTask {
		description = "List profiles for the specified executor. Call list_executors_kandev first to discover visible executors and their profiles, including custom executor IDs. A returned profile ID can be selected with executor_profile_id in create_task_kandev."
	}
	argumentDescription := "The executor ID"
	if s.profile.Surface == mcpprofile.SurfaceKanbanTask {
		argumentDescription = "The executor ID returned by list_executors_kandev"
	}
	s.mcpServer.AddTool(
		mcp.NewTool("list_executor_profiles_kandev",
			mcp.WithDescription(description),
			mcp.WithString("executor_id", mcp.Required(), mcp.Description(argumentDescription)),
		),
		s.wrapHandler("list_executor_profiles_kandev", s.listExecutorProfilesHandler()),
	)
}
