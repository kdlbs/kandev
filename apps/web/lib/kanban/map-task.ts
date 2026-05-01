import {
  isPRReviewFromMetadata,
  isIssueWatchFromMetadata,
  issueFieldsFromMetadata,
} from "@/lib/metadata-utils";
import type { KanbanState } from "@/lib/state/slices/kanban/types";
import type { TaskState, TaskSessionState } from "@/lib/types/http";

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
  repository_id?: string;
  primary_session_id?: string | null;
  primary_session_state?: TaskSessionState | string | null;
  session_count?: number | null;
  review_status?: "pending" | "approved" | "changes_requested" | "rejected" | null;
  primary_executor_id?: string | null;
  primary_executor_type?: string | null;
  primary_executor_name?: string | null;
  is_remote_executor?: boolean;
  parent_id?: string | null;
  updated_at?: string;
  created_at?: string;
  metadata?: Record<string, unknown> | null;
};

function pickRepositoryId(source: TaskLike): string | undefined {
  return source.repository_id ?? source.repositories?.[0]?.repository_id ?? undefined;
}

function pickId(source: TaskLike): string {
  return (source.id ?? source.task_id ?? "") as string;
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

/**
 * Build a canonical {@link KanbanTask} from either an HTTP DTO or a WebSocket
 * payload. Both paths share this helper so a single publisher change can never
 * leave them out of sync again (cf. sidebar filter regressions where the HTTP
 * snapshot derived `isPRReview` but the WS handler didn't).
 */
export function toKanbanTask(source: TaskLike): KanbanTask {
  return {
    id: pickId(source),
    workflowStepId: source.workflow_step_id ?? "",
    title: source.title ?? "",
    description: source.description ?? undefined,
    position: source.position ?? 0,
    state: source.state,
    repositoryId: pickRepositoryId(source),
    repositories: pickRepositories(source),
    primarySessionId: source.primary_session_id ?? undefined,
    primarySessionState: source.primary_session_state ?? undefined,
    sessionCount: source.session_count ?? undefined,
    reviewStatus: source.review_status ?? undefined,
    primaryExecutorId: source.primary_executor_id ?? undefined,
    primaryExecutorType: source.primary_executor_type ?? undefined,
    primaryExecutorName: source.primary_executor_name ?? undefined,
    isRemoteExecutor: source.is_remote_executor ?? false,
    parentTaskId: source.parent_id ?? undefined,
    updatedAt: source.updated_at,
    createdAt: source.created_at,
    isPRReview: isPRReviewFromMetadata(source.metadata),
    isIssueWatch: isIssueWatchFromMetadata(source.metadata),
    ...issueFieldsFromMetadata(source.metadata),
  } as KanbanTask;
}
