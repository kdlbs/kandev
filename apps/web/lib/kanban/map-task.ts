import {
  isPRReviewFromMetadata,
  isIssueWatchFromMetadata,
  issueFieldsFromMetadata,
} from "@/lib/metadata-utils";
import type { KanbanState } from "@/lib/state/slices/kanban/types";
import type {
  ForegroundActivity,
  TaskPendingAction,
  TaskState,
  TaskSessionState,
} from "@/lib/types/http";
import type { TaskStatusSummary } from "@/lib/types/task-status-summary";

type KanbanTask = KanbanState["tasks"][number];

/**
 * Shape accepted by {@link toKanbanTask}. Satisfied by both the HTTP Task DTO
 * (which nests repositories) and the backend's `task.updated` WebSocket
 * payload (which flattens `repository_id` and uses `task_id`).
 *
 * The single publisher / single mapper design relies on this shape being the
 * contract; anyone changing what the backend emits needs to keep it in sync,
 * and the parity test in `map-task.test.ts` will catch drift.
 */
export type TaskLike = {
  id?: string;
  task_id?: string;
  workspace_id?: string;
  workflow_id?: string;
  workflow_step_id?: string;
  title?: string;
  description?: string | null;
  position?: number;
  state?: TaskState;
  repositories?: Array<{
    id?: string;
    repository_id: string;
    base_branch?: string;
    checkout_branch?: string;
    position?: number;
  }>;
  workspace_folders?: Array<{
    id: string;
    local_path: string;
    display_name: string;
    position: number;
  }>;
  repository_id?: string;
  primary_session_id?: string | null;
  primary_session_state?: TaskSessionState | string | null;
  primary_session_pending_action?: TaskPendingAction | null;
  task_pending_action?: TaskPendingAction | null;
  /** True when the task's session was mid-turn when the backend died. */
  interrupted?: boolean;
  foreground_activity?: ForegroundActivity | null;
  active_subagent_count?: number;
  session_count?: number | null;
  review_status?: "pending" | "approved" | "changes_requested" | "rejected" | null;
  primary_executor_id?: string | null;
  primary_executor_type?: string | null;
  primary_executor_name?: string | null;
  is_remote_executor?: boolean;
  parent_id?: string | null;
  updated_at?: string;
  created_at?: string;
  wip_admitted?: boolean;
  queued_for_step_id?: string | null;
  queued_at?: string | null;
  metadata?: Record<string, unknown> | null;
  archived_at?: string | null;
  status_summary?: TaskStatusSummary | null;
};

export type WorkspaceMode = "inherit_parent" | "new_workspace" | "shared_group";

export function workspaceModeFromMetadata(
  metadata: TaskLike["metadata"],
): WorkspaceMode | undefined {
  const workspace = metadata?.workspace;
  if (!workspace || typeof workspace !== "object") return undefined;
  const mode = (workspace as Record<string, unknown>).mode;
  if (mode === "inherit_parent" || mode === "new_workspace" || mode === "shared_group") {
    return mode;
  }
  return undefined;
}

function pickRepositoryId(source: TaskLike): string | undefined {
  return source.repository_id ?? source.repositories?.[0]?.repository_id ?? undefined;
}

function pickId(source: TaskLike): string {
  return (source.id ?? source.task_id ?? "") as string;
}

export function pickPendingAction(action: unknown): TaskPendingAction | null | undefined {
  if (action === null) return null;
  if (action === "clarification" || action === "permission") {
    return action;
  }
  return undefined;
}

function pickForegroundActivity(
  activity: TaskLike["foreground_activity"],
): ForegroundActivity | null | undefined {
  return activity === null ? null : (activity ?? undefined);
}

type KanbanTaskRepository = NonNullable<KanbanTask["repositories"]>[number];

function pickRepositories(source: TaskLike): KanbanTaskRepository[] | undefined {
  if (!source.repositories) return undefined;
  return source.repositories.map((r, idx) => ({
    id: r.id ?? "",
    repository_id: r.repository_id,
    base_branch: r.base_branch ?? "",
    checkout_branch: r.checkout_branch,
    position: r.position ?? idx,
  }));
}

function pickWorkspaceFolders(source: TaskLike): KanbanTask["workspaceFolders"] | undefined {
  return source.workspace_folders?.map((folder) => ({ ...folder }));
}

/**
 * Build a canonical {@link KanbanTask} from either an HTTP DTO or a WebSocket
 * payload. Both paths share this helper so a single publisher change can never
 * leave them out of sync again (cf. sidebar filter regressions where the HTTP
 * snapshot derived `isPRReview` but the WS handler didn't).
 */
export function toKanbanTask(source: TaskLike): KanbanTask {
  return {
    id: pickId(source),
    workspaceId: source.workspace_id,
    workflowId: source.workflow_id,
    workflowStepId: source.workflow_step_id ?? "",
    title: source.title ?? "",
    description: source.description ?? undefined,
    position: source.position ?? 0,
    state: source.state,
    repositoryId: pickRepositoryId(source),
    repositories: pickRepositories(source),
    workspaceFolders: pickWorkspaceFolders(source),
    primarySessionId: source.primary_session_id ?? undefined,
    primarySessionState: source.primary_session_state ?? undefined,
    primarySessionPendingAction: pickPendingAction(source.primary_session_pending_action),
    taskPendingAction: pickPendingAction(source.task_pending_action),
    interrupted: source.interrupted,
    foregroundActivity: pickForegroundActivity(source.foreground_activity),
    activeSubagentCount: source.active_subagent_count ?? undefined,
    sessionCount: source.session_count ?? undefined,
    reviewStatus: source.review_status ?? undefined,
    primaryExecutorId: source.primary_executor_id ?? undefined,
    primaryExecutorType: source.primary_executor_type ?? undefined,
    primaryExecutorName: source.primary_executor_name ?? undefined,
    isRemoteExecutor: source.is_remote_executor ?? false,
    parentTaskId: source.parent_id ?? undefined,
    workspaceMode: workspaceModeFromMetadata(source.metadata),
    updatedAt: source.updated_at,
    createdAt: source.created_at,
    wipAdmitted: source.wip_admitted,
    queuedForStepId: source.queued_for_step_id,
    queuedAt: source.queued_at,
    statusSummary: source.status_summary,
    metadata: source.metadata,
    isArchived: source.archived_at != null,
    isPRReview: isPRReviewFromMetadata(source.metadata),
    isIssueWatch: isIssueWatchFromMetadata(source.metadata),
    ...issueFieldsFromMetadata(source.metadata),
  } as KanbanTask;
}
