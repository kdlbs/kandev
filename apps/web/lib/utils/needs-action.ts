import type { TaskState } from "@/lib/types/http";

/**
 * Determines if a task needs user attention/action.
 * Used for visual highlighting across kanban views.
 */
export function needsAction(task: {
  state?: TaskState;
  reviewStatus?: "pending" | "approved" | "changes_requested" | "rejected" | null;
}): boolean {
  return (
    (task.reviewStatus === "pending" && task.state !== "IN_PROGRESS") ||
    task.reviewStatus === "changes_requested" ||
    task.state === "WAITING_FOR_INPUT"
  );
}
