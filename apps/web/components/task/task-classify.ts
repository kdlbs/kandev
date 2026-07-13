import type { TaskState, TaskSessionState } from "@/lib/types/http";

// Single source of truth for bucketing a task into review / in_progress /
// backlog. Used by the sidebar sort in task-switcher and the per-item
// spinner icon in task-item so the two views always agree.

const REVIEW_STATES = new Set<TaskSessionState>([
  "WAITING_FOR_INPUT",
  "COMPLETED",
  "FAILED",
  "CANCELLED",
]);
const IN_PROGRESS_STATES = new Set<TaskSessionState>(["RUNNING"]);
const TASK_STATE_REVIEW = new Set<TaskState | undefined>(["REVIEW", "COMPLETED"]);
const TASK_STATE_IN_PROGRESS = new Set<TaskState | undefined>(["IN_PROGRESS", "SCHEDULING"]);

export type TaskBucket = "review" | "in_progress" | "backlog";

export function classifyTask(
  sessionState: TaskSessionState | undefined,
  taskState?: TaskState,
): TaskBucket {
  if (!sessionState) {
    if (TASK_STATE_REVIEW.has(taskState)) return "review";
    if (TASK_STATE_IN_PROGRESS.has(taskState)) return "in_progress";
    return "backlog";
  }
  // CREATED/STARTING are transient session states while the agent process is
  // booting or reattaching. Defer to the workflow task state so an
  // IN_PROGRESS task keeps showing activity, while a REVIEW task does not
  // flicker out of the review bucket during auto-resume.
  if ((sessionState === "CREATED" || sessionState === "STARTING") && taskState) {
    if (TASK_STATE_REVIEW.has(taskState)) return "review";
    if (TASK_STATE_IN_PROGRESS.has(taskState)) return "in_progress";
    return "backlog";
  }
  if (REVIEW_STATES.has(sessionState)) return "review";
  if (IN_PROGRESS_STATES.has(sessionState)) return "in_progress";
  return "backlog";
}
