import type { TaskPendingAction, TaskState } from "@/lib/types/http";

/**
 * Determines if a task needs user attention/action.
 * Used for visual highlighting across kanban views.
 */
export function needsAction(task: {
  state?: TaskState;
  taskPendingAction?: TaskPendingAction | null;
  statusSummary?: { pending_action?: TaskPendingAction | null } | null;
  reviewStatus?: "pending" | "approved" | "changes_requested" | "rejected" | null;
}): boolean {
  const pendingAction =
    task.statusSummary != null ? task.statusSummary.pending_action : task.taskPendingAction;
  return (
    (task.reviewStatus === "pending" && task.state !== "IN_PROGRESS") ||
    task.reviewStatus === "changes_requested" ||
    pendingAction != null ||
    task.state === "WAITING_FOR_INPUT"
  );
}
