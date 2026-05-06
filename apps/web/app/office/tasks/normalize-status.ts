import type { OfficeTaskStatus } from "@/lib/state/slices/office/types";

/**
 * Normalises a task status string from any source (backend uppercase enum
 * like "REVIEW", lowercase office canonical "in_review", or local
 * detail-page TaskStatus) into the canonical OfficeTaskStatus union.
 *
 * Backend `task.state` uses the core task enum (TODO, REVIEW, IN_PROGRESS,
 * COMPLETED, …) while office's display layer wants the lowercase
 * Linear-style names (todo, in_review, done, …). This function bridges
 * them so every status display can look itself up in a single map.
 */
const STATUS_MAP: Record<string, OfficeTaskStatus> = {
  todo: "todo",
  created: "todo",
  scheduling: "todo",
  in_progress: "in_progress",
  waiting_for_input: "in_progress",
  review: "in_review",
  in_review: "in_review",
  blocked: "blocked",
  failed: "blocked",
  completed: "done",
  done: "done",
  cancelled: "cancelled",
  canceled: "cancelled",
  backlog: "backlog",
};

export function normalizeTaskStatus(status: string | undefined | null): OfficeTaskStatus {
  if (!status) return "backlog";
  return STATUS_MAP[status.toLowerCase()] ?? "backlog";
}

// Inverse of STATUS_MAP — maps the canonical office status to the set of
// backend `task.state` enum values that normalise to it. Used when sending
// status filters to the backend's filtered-task-list endpoint, which
// matches against raw task.state strings via SQL `IN (...)`.
const CANONICAL_TO_BACKEND: Record<OfficeTaskStatus, string[]> = {
  backlog: ["BACKLOG"],
  todo: ["TODO", "CREATED", "SCHEDULING"],
  in_progress: ["IN_PROGRESS", "WAITING_FOR_INPUT"],
  in_review: ["REVIEW", "IN_REVIEW"],
  blocked: ["BLOCKED", "FAILED"],
  done: ["COMPLETED", "DONE"],
  cancelled: ["CANCELLED", "CANCELED"],
};

export function canonicalStatusesToBackend(statuses: OfficeTaskStatus[]): string[] {
  const out: string[] = [];
  for (const s of statuses) out.push(...(CANONICAL_TO_BACKEND[s] ?? []));
  return out;
}
