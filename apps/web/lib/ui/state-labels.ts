import type { TaskState, TaskSessionState } from "@/lib/types/http";

const TASK_STATE_LABELS: Record<TaskState, string> = {
  CREATED: "Created",
  SCHEDULING: "Scheduling",
  TODO: "To do",
  IN_PROGRESS: "In progress",
  REVIEW: "Review",
  BLOCKED: "Blocked",
  WAITING_FOR_INPUT: "Waiting for input",
  COMPLETED: "Completed",
  FAILED: "Failed",
  CANCELLED: "Cancelled",
};

const TASK_SESSION_STATE_LABELS: Record<TaskSessionState, string> = {
  CREATED: "Created",
  STARTING: "Starting",
  RUNNING: "Running",
  WAITING_FOR_INPUT: "Waiting for input",
  COMPLETED: "Completed",
  FAILED: "Failed",
  CANCELLED: "Cancelled",
};

export function formatTaskStateLabel(state: TaskState | null | undefined): string {
  if (!state) return "Not started";
  return TASK_STATE_LABELS[state] ?? state;
}

export function formatTaskSessionStateLabel(state: TaskSessionState | null | undefined): string {
  if (!state) return "";
  return TASK_SESSION_STATE_LABELS[state] ?? state;
}
