// Static catalog of MCP tools exposed by the Kandev backend's external endpoint.
// Mirrors apps/backend/internal/mcp/server (ModeExternal). Keep in sync when
// tools are added, removed, or renamed.
//
// The `name`s are the protocol identifiers an external agent calls, so they stay
// literal in every locale. Only the group titles and the human descriptions are
// copy, and they travel as catalog keys resolved at render — this module is
// evaluated at import, so a `t()` here would freeze at the boot locale.

export type ExternalMcpTool = {
  name: string;
  descriptionKey: string;
  /** Interpolated into `descriptionKey`; wire tokens the user must type verbatim. */
  descriptionValues?: Record<string, string>;
};

export type ExternalMcpToolGroup = {
  titleKey: string;
  descriptionKey: string;
  tools: ExternalMcpTool[];
};

/** Task-state enum accepted by `update_task_state_kandev`, verbatim wire values. */
// i18n-exempt: task state enum values, matched by the backend.
const TASK_STATES = "open, in_progress, complete, blocked, cancelled";

/** Request field on `create_task_kandev`, verbatim wire value. */
const PARENT_ID_PARAM = "parent_id";

export const EXTERNAL_MCP_TOOL_GROUPS: ExternalMcpToolGroup[] = [
  {
    titleKey: "settings:externalMcpGroupWorkspaces",
    descriptionKey: "settings:externalMcpGroupWorkspacesDescription",
    tools: [
      { name: "list_workspaces_kandev", descriptionKey: "settings:externalMcpToolListWorkspaces" },
      { name: "list_workflows_kandev", descriptionKey: "settings:externalMcpToolListWorkflows" },
      {
        name: "list_repositories_kandev",
        descriptionKey: "settings:externalMcpToolListRepositories",
      },
      { name: "create_workflow_kandev", descriptionKey: "settings:externalMcpToolCreateWorkflow" },
      { name: "update_workflow_kandev", descriptionKey: "settings:externalMcpToolUpdateWorkflow" },
      { name: "delete_workflow_kandev", descriptionKey: "settings:externalMcpToolDeleteWorkflow" },
      { name: "import_workflow_kandev", descriptionKey: "settings:externalMcpToolImportWorkflow" },
      { name: "export_workflow_kandev", descriptionKey: "settings:externalMcpToolExportWorkflow" },
    ],
  },
  {
    titleKey: "settings:externalMcpGroupWorkflowSteps",
    descriptionKey: "settings:externalMcpGroupWorkflowStepsDescription",
    tools: [
      {
        name: "list_workflow_steps_kandev",
        descriptionKey: "settings:externalMcpToolListWorkflowSteps",
      },
      {
        name: "create_workflow_step_kandev",
        descriptionKey: "settings:externalMcpToolCreateWorkflowStep",
      },
      {
        name: "update_workflow_step_kandev",
        descriptionKey: "settings:externalMcpToolUpdateWorkflowStep",
      },
      {
        name: "delete_workflow_step_kandev",
        descriptionKey: "settings:externalMcpToolDeleteWorkflowStep",
      },
      {
        name: "reorder_workflow_steps_kandev",
        descriptionKey: "settings:externalMcpToolReorderWorkflowSteps",
      },
    ],
  },
  {
    titleKey: "settings:externalMcpGroupAgents",
    descriptionKey: "settings:externalMcpGroupAgentsDescription",
    tools: [
      { name: "list_agents_kandev", descriptionKey: "settings:externalMcpToolListAgents" },
      { name: "update_agent_kandev", descriptionKey: "settings:externalMcpToolUpdateAgent" },
      {
        name: "create_agent_profile_kandev",
        descriptionKey: "settings:externalMcpToolCreateAgentProfile",
      },
      {
        name: "delete_agent_profile_kandev",
        descriptionKey: "settings:externalMcpToolDeleteAgentProfile",
      },
    ],
  },
  {
    titleKey: "settings:externalMcpGroupAgentProfiles",
    descriptionKey: "settings:externalMcpGroupAgentProfilesDescription",
    tools: [
      {
        name: "list_agent_profiles_kandev",
        descriptionKey: "settings:externalMcpToolListAgentProfiles",
      },
      {
        name: "update_agent_profile_kandev",
        descriptionKey: "settings:externalMcpToolUpdateAgentProfile",
      },
      { name: "get_mcp_config_kandev", descriptionKey: "settings:externalMcpToolGetMcpConfig" },
      {
        name: "update_mcp_config_kandev",
        descriptionKey: "settings:externalMcpToolUpdateMcpConfig",
      },
    ],
  },
  {
    titleKey: "settings:externalMcpGroupExecutors",
    descriptionKey: "settings:externalMcpGroupExecutorsDescription",
    tools: [
      { name: "list_executors_kandev", descriptionKey: "settings:externalMcpToolListExecutors" },
      {
        name: "list_executor_profiles_kandev",
        descriptionKey: "settings:externalMcpToolListExecutorProfiles",
      },
      {
        name: "create_executor_profile_kandev",
        descriptionKey: "settings:externalMcpToolCreateExecutorProfile",
      },
      {
        name: "update_executor_profile_kandev",
        descriptionKey: "settings:externalMcpToolUpdateExecutorProfile",
      },
      {
        name: "delete_executor_profile_kandev",
        descriptionKey: "settings:externalMcpToolDeleteExecutorProfile",
      },
    ],
  },
  {
    titleKey: "settings:externalMcpGroupTasks",
    descriptionKey: "settings:externalMcpGroupTasksDescription",
    tools: [
      { name: "list_tasks_kandev", descriptionKey: "settings:externalMcpToolListTasks" },
      {
        name: "list_task_sessions_kandev",
        descriptionKey: "settings:externalMcpToolListTaskSessions",
      },
      {
        name: "list_pending_agent_permissions_kandev",
        descriptionKey: "settings:externalMcpToolListPendingAgentPermissions",
      },
      {
        name: "resolve_agent_permission_kandev",
        descriptionKey: "settings:externalMcpToolResolveAgentPermission",
      },
      { name: "move_task_kandev", descriptionKey: "settings:externalMcpToolMoveTask" },
      { name: "delete_task_kandev", descriptionKey: "settings:externalMcpToolDeleteTask" },
      { name: "archive_task_kandev", descriptionKey: "settings:externalMcpToolArchiveTask" },
      {
        name: "update_task_state_kandev",
        descriptionKey: "settings:externalMcpToolUpdateTaskState",
        descriptionValues: { states: TASK_STATES },
      },
      {
        name: "get_task_conversation_kandev",
        descriptionKey: "settings:externalMcpToolGetTaskConversation",
      },
    ],
  },
  {
    titleKey: "settings:externalMcpGroupTaskCreation",
    descriptionKey: "settings:externalMcpGroupTaskCreationDescription",
    tools: [
      {
        name: "create_task_kandev",
        descriptionKey: "settings:externalMcpToolCreateTask",
        descriptionValues: { parentIdParam: PARENT_ID_PARAM },
      },
    ],
  },
  {
    titleKey: "settings:externalMcpGroupTaskDependencies",
    descriptionKey: "settings:externalMcpGroupTaskDependenciesDescription",
    tools: [
      {
        name: "add_task_dependency_kandev",
        descriptionKey: "settings:externalMcpToolAddTaskDependency",
      },
      {
        name: "remove_task_dependency_kandev",
        descriptionKey: "settings:externalMcpToolRemoveTaskDependency",
      },
    ],
  },
  {
    titleKey: "settings:externalMcpGroupQuestions",
    descriptionKey: "settings:externalMcpGroupQuestionsDescription",
    tools: [
      {
        name: "list_pending_questions_kandev",
        descriptionKey: "settings:externalMcpToolListPendingQuestions",
      },
      {
        name: "answer_question_kandev",
        descriptionKey: "settings:externalMcpToolAnswerQuestion",
      },
    ],
  },
];

export function countExternalMcpTools(): number {
  return EXTERNAL_MCP_TOOL_GROUPS.reduce((sum, group) => sum + group.tools.length, 0);
}
