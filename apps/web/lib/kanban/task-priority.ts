import type { TaskPriority } from "@/lib/types/http";

/**
 * Shared between the create-task priority control and the card-menu priority
 * action so the two surfaces cannot render the four tokens in different
 * orders. Severity order, matching the spec's enumeration and the Office
 * fallback ordering.
 */
export const KANBAN_PRIORITY_TOKENS: readonly TaskPriority[] = [
  "critical",
  "high",
  "medium",
  "low",
];

export const KANBAN_PRIORITY_LABEL_KEYS: Record<TaskPriority, string> = {
  critical: "kanban:priorityCritical",
  high: "kanban:priorityHigh",
  medium: "kanban:priorityMedium",
  low: "kanban:priorityLow",
};

/** A priority value read from a task is untrusted wire data, not a `TaskPriority`. */
export function isKanbanPriority(value: unknown): value is TaskPriority {
  return typeof value === "string" && (KANBAN_PRIORITY_TOKENS as readonly string[]).includes(value);
}
