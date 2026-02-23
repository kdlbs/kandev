import type { Repository, Task } from "@/lib/types/http";

export function buildDebugEntries(params: {
  connectionStatus: string;
  task: Task | null;
  effectiveSessionId: string | null | undefined;
  taskSessionState: string | null;
  isAgentWorking: boolean;
  resumptionState: string;
  resumptionError: string | null;
  agentctlStatus: {
    status: string;
    isReady: boolean;
    errorMessage?: string | null;
    agentExecutionId?: string | null;
  };
  previewOpen: boolean;
  previewStage: string;
  previewUrl: string;
  devProcessId: string | undefined;
  devProcessStatus: string | null;
}): Record<string, unknown> {
  const {
    connectionStatus,
    task,
    effectiveSessionId,
    taskSessionState,
    isAgentWorking,
    resumptionState,
    resumptionError,
    agentctlStatus,
    previewOpen,
    previewStage,
    previewUrl,
    devProcessId,
    devProcessStatus,
  } = params;
  return {
    ws_status: connectionStatus,
    task_id: task?.id ?? null,
    session_id: effectiveSessionId ?? null,
    task_state: task?.state ?? null,
    task_session_state: taskSessionState ?? null,
    is_agent_working: isAgentWorking,
    resumption_state: resumptionState,
    resumption_error: resumptionError,
    agentctl_status: agentctlStatus.status,
    agentctl_ready: agentctlStatus.isReady,
    agentctl_error: agentctlStatus.errorMessage ?? null,
    agentctl_execution_id: agentctlStatus.agentExecutionId ?? null,
    preview_open: previewOpen,
    preview_stage: previewStage,
    preview_url: previewUrl || null,
    dev_process_id: devProcessId ?? null,
    dev_process_status: devProcessStatus ?? null,
  };
}

export function deriveIsAgentWorking(
  taskSessionState: string | null,
  isAgentRunning: boolean,
  taskState: string | null,
): boolean {
  if (taskSessionState !== null)
    return taskSessionState === "STARTING" || taskSessionState === "RUNNING";
  return isAgentRunning && (taskState === "IN_PROGRESS" || taskState === "SCHEDULING");
}

export function buildArchivedValue(task: Task | null, repository: Repository | null) {
  const isArchived = !!task?.archived_at;
  return {
    isArchived,
    archivedTaskId: isArchived ? task?.id : undefined,
    archivedTaskTitle: isArchived ? task?.title : undefined,
    archivedTaskRepositoryPath: isArchived ? (repository?.local_path ?? undefined) : undefined,
    archivedTaskUpdatedAt: isArchived ? task?.updated_at : undefined,
  };
}

export function resolveTaskIds(task: Task | null) {
  return {
    taskId: task?.id ?? null,
    workflowId: task?.workflow_id ?? null,
    workspaceId: task?.workspace_id ?? null,
    workflowStepId: task?.workflow_step_id ?? null,
    baseBranch: task?.repositories?.[0]?.base_branch,
    isArchived: !!task?.archived_at,
  };
}

export function resolveTaskProps(task: Task | null, repository: Repository | null) {
  const ids = resolveTaskIds(task);
  return {
    ...ids,
    taskTitle: task?.title,
    taskDescription: task?.description,
    repositoryPath: repository?.local_path ?? null,
    repositoryName: repository?.name ?? null,
  };
}
