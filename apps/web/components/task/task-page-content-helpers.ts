import type { Repository, Task } from "@/lib/types/http";
import { issueFieldsFromMetadata } from "@/lib/metadata-utils";

type ACPDebugInfo = {
  sessionId: unknown;
  title: unknown;
  updatedAt: unknown;
  meta: unknown;
};

function readACPDebugInfo(metadata: Record<string, unknown> | null | undefined): ACPDebugInfo {
  const acp = metadata?.acp;
  const acpObject =
    acp && typeof acp === "object" && !Array.isArray(acp) ? (acp as Record<string, unknown>) : {};
  return {
    sessionId: acpObject.session_id ?? null,
    title: acpObject.title ?? null,
    updatedAt: acpObject.updated_at ?? null,
    meta: acpObject.meta ?? null,
  };
}

export function buildDebugEntries(params: {
  connectionStatus: string;
  task: Task | null;
  effectiveSessionId: string | null | undefined;
  activeSessionMetadata?: Record<string, unknown> | null;
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
    activeSessionMetadata,
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
  const acp = readACPDebugInfo(activeSessionMetadata);
  return {
    ws_status: connectionStatus,
    task_id: task?.id ?? null,
    session_id: effectiveSessionId ?? null,
    acp_session_id: acp.sessionId,
    acp_session_title: acp.title,
    acp_session_updated_at: acp.updatedAt,
    acp_meta: acp.meta,
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

export function resolveTaskContentState(params: {
  isMounted: boolean;
  hasTask: boolean;
  hasTaskLoadError: boolean;
}) {
  if (!params.isMounted) return "loading";
  if (params.hasTaskLoadError) return "error";
  if (params.hasTask) return "ready";
  return "loading";
}

export function hasResolvedTaskDetails(params: {
  effectiveTaskId: string | null;
  taskDetailsId?: string | null;
  initialTaskId?: string | null;
  snapshotTaskId?: string | null;
}) {
  if (!params.effectiveTaskId) return false;
  return (
    params.taskDetailsId === params.effectiveTaskId ||
    params.initialTaskId === params.effectiveTaskId ||
    params.snapshotTaskId === params.effectiveTaskId
  );
}

export function syncActiveTaskSession(params: {
  initialTaskId: string | undefined;
  fallbackTaskId: string | null | undefined;
  initialSessionId: string | null;
  setActiveSessionAuto: (taskId: string, sessionId: string) => void;
  setActiveTask: (taskId: string) => void;
}) {
  const taskId = params.initialTaskId ?? params.fallbackTaskId;
  if (!taskId) return;
  if (params.initialSessionId) params.setActiveSessionAuto(taskId, params.initialSessionId);
  else params.setActiveTask(taskId);
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
  const issue = issueFieldsFromMetadata(task?.metadata);
  return {
    ...ids,
    taskTitle: task?.title,
    taskDescription: task?.description,
    issueUrl: issue.issueUrl,
    issueNumber: issue.issueNumber,
    repositoryPath: repository?.local_path ?? null,
    repositoryName: repository?.name ?? null,
    /**
     * Total number of repositories linked to the task. Used by the top-bar
     * breadcrumb to render a "+N" chip next to the primary repo name when
     * the task is multi-repo. 0 / 1 means single-repo (no chip).
     */
    repositoryCount: task?.repositories?.length ?? 0,
  };
}
