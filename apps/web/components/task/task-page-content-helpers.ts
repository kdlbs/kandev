import {
  taskId as toTaskId,
  workflowId as toWorkflowId,
  workspaceId as toWorkspaceId,
  type Repository,
  type Task,
} from "@/lib/types/http";
import type { KanbanState } from "@/lib/state/slices";
import { issueFieldsFromMetadata } from "@/lib/metadata-utils";
import { repositorySlug } from "@/lib/repository-slug";

const EMPTY_REPOSITORIES: Repository[] = [];

export function selectWorkspaceRepositories(
  itemsByWorkspaceId: Record<string, Repository[]>,
  workspaceId: string | null | undefined,
): Repository[] {
  return (workspaceId && itemsByWorkspaceId[workspaceId]) || EMPTY_REPOSITORIES;
}

type ACPDebugInfo = {
  sessionId: unknown;
  updatedAt: unknown;
  meta: unknown;
};

function readACPDebugInfo(metadata: Record<string, unknown> | null | undefined): ACPDebugInfo {
  const acp = metadata?.acp;
  const acpObject =
    acp && typeof acp === "object" && !Array.isArray(acp) ? (acp as Record<string, unknown>) : {};
  return {
    sessionId: acpObject.session_id ?? null,
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

/**
 * Resolve the task the detail view should render, layering the freshest of
 * three sources: the one-shot `fetchTask` details, the SSR/`initialTask`, and
 * the live kanban entry. The base (details/initial) carries fields the kanban
 * doesn't (repositories, timestamps); the kanban carries live board state.
 */
export function resolveEffectiveTask(
  taskDetails: Task | null,
  initialTask: Task | null,
  kanbanTask: KanbanState["tasks"][number] | null,
  effectiveTaskId: string | null,
): Task | null {
  const matchingTaskDetails = taskDetails?.id === effectiveTaskId ? taskDetails : null;
  const matchingInitialTask = initialTask?.id === effectiveTaskId ? initialTask : null;
  const baseTask = matchingTaskDetails ?? matchingInitialTask;

  if (!baseTask && !kanbanTask) return null;
  if (baseTask) return mergeBaseWithKanban(baseTask, kanbanTask);
  if (kanbanTask) return buildTaskFromKanban(kanbanTask);
  return null;
}

export function mergeBaseWithKanban(
  baseTask: Task,
  kanbanTask: KanbanState["tasks"][number] | null,
): Task {
  if (!kanbanTask) return baseTask;
  const kanbanUpdatedAt = Date.parse(kanbanTask.updatedAt ?? "");
  const baseUpdatedAt = Date.parse(baseTask.updated_at ?? "");
  const hasNewerKanbanState =
    Boolean(baseTask.archived_at) &&
    Number.isFinite(kanbanUpdatedAt) &&
    Number.isFinite(baseUpdatedAt) &&
    kanbanUpdatedAt > baseUpdatedAt;
  return {
    ...baseTask,
    title: kanbanTask.title ?? baseTask.title,
    description: kanbanTask.description ?? baseTask.description,
    workflow_step_id:
      (kanbanTask.workflowStepId as string | undefined) ?? baseTask.workflow_step_id,
    position: kanbanTask.position ?? baseTask.position,
    state: (kanbanTask.state as Task["state"] | undefined) ?? baseTask.state,
    repositories: baseTask.repositories,
    metadata: kanbanTask.metadata !== undefined ? kanbanTask.metadata : baseTask.metadata,
    archived_at: hasNewerKanbanState ? null : baseTask.archived_at,
  };
}

export function buildTaskFromKanban(kanbanTask: KanbanState["tasks"][number]): Task {
  return {
    id: toTaskId(kanbanTask.id),
    title: kanbanTask.title,
    description: kanbanTask.description ?? "",
    workflow_step_id: kanbanTask.workflowStepId,
    position: kanbanTask.position,
    state: kanbanTask.state ?? "CREATED",
    workspace_id: toWorkspaceId(""),
    workflow_id: toWorkflowId(""),
    priority: kanbanTask.priority ?? "medium",
    repositories: [],
    created_at: "",
    updated_at: kanbanTask.updatedAt ?? "",
    metadata: kanbanTask.metadata,
  };
}

export function buildArchivedValue(task: Task | null, repository: Repository | null) {
  const isArchived = !!task?.archived_at;
  return {
    isArchived,
    archivedTaskId: isArchived ? task?.id : undefined,
    archivedTaskTitle: isArchived ? task?.title : undefined,
    archivedTaskRepositoryLabel: isArchived && repository ? repositorySlug(repository) : undefined,
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
}) {
  if (!params.effectiveTaskId) return false;
  return (
    params.taskDetailsId === params.effectiveTaskId ||
    params.initialTaskId === params.effectiveTaskId
  );
}

export function syncActiveTaskSession(params: {
  initialTaskId: string | undefined;
  fallbackTaskId: string | null | undefined;
  initialSessionId: string | null;
  activeTaskId: string | null;
  previousRouteTaskId: string | null | undefined;
  setActiveSessionAuto: (taskId: string, sessionId: string) => void;
  setActiveTask: (taskId: string) => void;
}): boolean {
  const taskId = params.initialTaskId ?? params.fallbackTaskId;
  if (!taskId) return false;
  const routeChanged = params.previousRouteTaskId !== taskId;
  if (!routeChanged && params.activeTaskId !== taskId) return false;
  if (params.initialSessionId) params.setActiveSessionAuto(taskId, params.initialSessionId);
  else params.setActiveTask(taskId);
  return true;
}

export function resolveTaskIds(task: Task | null) {
  const taskValues = task ?? ({} as Task);
  const primaryRepository = taskValues.repositories?.[0];
  return {
    taskId: taskValues.id ?? null,
    workflowId: taskValues.workflow_id ?? null,
    workspaceId: taskValues.workspace_id ?? null,
    projectId: taskValues.project_id ?? null,
    workflowStepId: taskValues.workflow_step_id ?? null,
    baseBranch: primaryRepository?.base_branch,
    pullRequestTarget: primaryRepository?.branch_policy_pull_request_target || undefined,
    isArchived: !!taskValues.archived_at,
  };
}

function buildPullRequestTargetsByRepository(
  task: Pick<Task, "repositories"> | null,
  repositories: Repository[],
): Record<string, string> {
  const targets: Record<string, string> = {};
  for (const taskRepository of task?.repositories ?? []) {
    const target = taskRepository.branch_policy_pull_request_target?.trim();
    if (!target) continue;
    targets[taskRepository.repository_id] = target;
    const workspaceRepository = repositories.find(
      (candidate) => candidate.id === taskRepository.repository_id,
    );
    if (!workspaceRepository) continue;
    targets[workspaceRepository.id] = target;
    if (workspaceRepository.name) targets[workspaceRepository.name] = target;
    const slug = repositorySlug(workspaceRepository);
    if (slug) targets[slug] = target;
  }
  return targets;
}

export function resolveTaskPullRequestProps(
  task: Pick<Task, "title" | "repositories"> | null,
  repositories: Repository[] = [],
) {
  const primaryRepository = task?.repositories?.[0];
  return {
    baseBranch: primaryRepository?.base_branch,
    pullRequestTarget: primaryRepository?.branch_policy_pull_request_target || undefined,
    pullRequestTargetsByRepository: buildPullRequestTargetsByRepository(task, repositories),
    taskTitle: task?.title,
  };
}

export function resolveTaskProps(
  task: Task | null,
  repository: Repository | null,
  repositories: Repository[] = [],
) {
  const ids = resolveTaskIds(task);
  const issue = issueFieldsFromMetadata(task?.metadata);
  const pullRequestProps = resolveTaskPullRequestProps(task, repositories);
  return {
    ...ids,
    taskDescription: task?.description,
    issueUrl: issue.issueUrl,
    issueNumber: issue.issueNumber,
    repositoryPath: repository?.local_path ?? null,
    repositoryName: repository?.name ?? null,
    /**
     * What the top bar shows so a user can tell which project an open task
     * belongs to: the same `owner/repo` identity the sidebar rows and the
     * repository filter use, never the local clone path.
     */
    repositoryLabel: repository ? repositorySlug(repository) : null,
    ...pullRequestProps,
    /**
     * Total number of repositories linked to the task. Used by the top-bar
     * breadcrumb to render a "+N" chip next to the primary repo name when
     * the task is multi-repo. 0 / 1 means single-repo (no chip).
     */
    repositoryCount: task?.repositories?.length ?? 0,
  };
}
