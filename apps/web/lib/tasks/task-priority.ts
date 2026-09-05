import type { TaskPriority } from "@/lib/types/http";

/** Severity order shared by every task priority control. */
export const TASK_PRIORITY_TOKENS: readonly TaskPriority[] = ["critical", "high", "medium", "low"];

export const TASK_PRIORITY_LABEL_KEYS: Record<TaskPriority, string> = {
  critical: "kanban:priorityCritical",
  high: "kanban:priorityHigh",
  medium: "kanban:priorityMedium",
  low: "kanban:priorityLow",
};

/** A priority value read from a task is untrusted wire data, not a `TaskPriority`. */
export function isTaskPriority(value: unknown): value is TaskPriority {
  return typeof value === "string" && (TASK_PRIORITY_TOKENS as readonly string[]).includes(value);
}
