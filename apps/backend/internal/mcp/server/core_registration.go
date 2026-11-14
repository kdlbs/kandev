package mcp

import (
	"slices"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	mcpproviders "github.com/kandev/kandev/internal/mcp/providers"
)

type coreToolGroup struct {
	surfaces []mcpprofile.Surface
	register func(*Server)
}

// Core membership is constrained by caller authority. Selecting another package
// cannot change these predicates or replace Core's definitions.
func (s *Server) registerCoreTools() {
	kanban := []mcpprofile.Surface{mcpprofile.SurfaceKanbanTask}
	external := []mcpprofile.Surface{mcpprofile.SurfaceExternal}
	workspace := []mcpprofile.Surface{mcpprofile.SurfaceConfiguration, mcpprofile.SurfaceExternal}
	tasks := []mcpprofile.Surface{mcpprofile.SurfaceKanbanTask, mcpprofile.SurfaceOfficeTask}
	discovery := []mcpprofile.Surface{mcpprofile.SurfaceKanbanTask, mcpprofile.SurfaceConfiguration, mcpprofile.SurfaceExternal}
	transfer := []mcpprofile.Surface{mcpprofile.SurfaceConfiguration, mcpprofile.SurfaceExternal, mcpprofile.SurfaceOfficeTask}
	groups := []coreToolGroup{
		{discovery, (*Server).registerCoreDiscoveryTools},
		{discovery, (*Server).registerCoreTaskTools},
		{workspace, (*Server).registerListRepositoriesTool},
		{workspace, (*Server).registerUpdateTaskStateTool},
		{transfer, (*Server).registerTaskTransferTool},
		{external, (*Server).registerCreateTaskTool},
		{external, (*Server).registerUpdateTaskTool},
		{external, (*Server).registerQuestionAnsweringTools},
		{external, (*Server).registerAgentPermissionTools},
		{[]mcpprofile.Surface{mcpprofile.SurfaceKanbanTask, mcpprofile.SurfaceExternal}, (*Server).registerTaskDependencyTools},
		{kanban, (*Server).registerTaskExecutionTools},
		{kanban, (*Server).registerCoreTaskCapabilities},
		{kanban, (*Server).registerReviewTools},
		// Review run, source and readback tools act on a task the caller can
		// write, so Office task agents and external clients use them too.
		{[]mcpprofile.Surface{mcpprofile.SurfaceKanbanTask, mcpprofile.SurfaceOfficeTask, mcpprofile.SurfaceExternal}, (*Server).registerTaskReviewTools},
		{kanban, (*Server).registerAddBranchToTaskTool},
		{kanban, (*Server).registerAddWorkspaceSourcesTool},
		{kanban, (*Server).registerUpdateRepositoryBaseBranchTool},
		{tasks, (*Server).registerPlanTools},
		{tasks, (*Server).registerRelatedTasksTool},
		{tasks, (*Server).registerStepCompleteTool},
		{[]mcpprofile.Surface{mcpprofile.SurfaceOfficeTask}, (*Server).registerTaskDocumentTools},
	}
	for _, group := range groups {
		if slices.Contains(group.surfaces, s.profile.Surface) {
			group.register(s)
		}
	}
	s.registerCoreQuestionTools()
}

func (s *Server) registerCoreQuestionTools() {
	if s.profile.HasCapability(mcpprofile.CapabilityUserQuestion) {
		s.registerInteractionTools()
	}
}

func (s *Server) registerCoreTaskCapabilities() {
	if mcpproviders.Contains(s.profile.Providers, mcpproviders.GitHub) || mcpproviders.Contains(s.profile.Providers, mcpproviders.GitLab) {
		s.registerTaskPRLinkTools()
	}
	if mcpproviders.Contains(s.profile.Providers, mcpproviders.GitHub) {
		s.registerPRAutomationTools()
	}
	if s.profile.HasCapability(mcpprofile.CapabilityParentQuestion) {
		s.registerParentQuestionTool()
	}
	if s.profile.HasCapability(mcpprofile.CapabilityTaskTitle) {
		s.registerSetTaskTitleTool()
	}
	if s.profile.HasCapability(mcpprofile.CapabilityCanvas) {
		s.registerCanvasTools()
	}
}

func (s *Server) registerCoreDiscoveryTools() {
	s.registerListWorkspacesTool()
	s.registerListWorkflowsTool()
	s.registerListWorkflowStepsTool()
	s.registerListAgentsTool()
	s.registerListExecutorsTool()
	s.registerListExecutorProfilesTool()
}

func (s *Server) registerCoreTaskTools() {
	s.registerListTasksTool()
	s.registerMoveTaskTool()
	s.registerDeleteTaskTool()
	s.registerArchiveTaskTool()
	s.registerGetTaskConversationTool()
	s.registerListTaskSessionsTool()
}
