import type { OfficeTask, OfficeTaskStatus } from "@/lib/state/slices/office/types";

/** The task shape received before the Office status contract is applied. */
export type OfficeTaskWire = Omit<OfficeTask, "status" | "rawStatus" | "children"> & {
  status: string;
  rawStatus?: string | null;
  children?: OfficeTaskWire[];
};

/**
 * Normalises a task status from any backend or local source into the
 * canonical OfficeTaskStatus union.
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

/** Converts a wire task into the canonical Office store shape. */
export function normalizeOfficeTask(task: OfficeTaskWire): OfficeTask {
  const rawStatus = task.rawStatus ?? task.status;
  return {
    ...task,
    status: normalizeTaskStatus(rawStatus),
    rawStatus,
    children: task.children?.map(normalizeOfficeTask),
  };
}

// Inverse of STATUS_MAP. It expands canonical values for backend SQL filters.
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
  for (const status of statuses) out.push(...(CANONICAL_TO_BACKEND[status] ?? []));
  return out;
}
