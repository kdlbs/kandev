import { ApiError, fetchJson, type ApiRequestOptions } from "@/lib/api/client";

// Mirrors internal/coordinator/policy.go's Action values plus "unknown".
export type ActivityClass =
  | "create_task"
  | "start_agent"
  | "message"
  | "move"
  | "resume"
  | "stop"
  | "improvement"
  | "unknown";

export const ACTIVITY_CLASSES: readonly ActivityClass[] = [
  "create_task",
  "start_agent",
  "message",
  "move",
  "resume",
  "stop",
  "improvement",
  "unknown",
];

export function isActivityClass(value: string | null): value is ActivityClass {
  return value !== null && (ACTIVITY_CLASSES as readonly string[]).includes(value);
}

// Mirrors internal/coordinator/activity.go's ActivityOutcome values.
export type ActivityOutcome =
  | "proposed"
  | "approved"
  | "rejected"
  | "failed"
  | "refused"
  | "undone"
  | "returned";

// Mirrors internal/coordinator/activity.go's ActivityAuthorization values.
export type ActivityAuthorization = "requires_approval" | "denied" | "automatic";

// Mirrors internal/coordinator/activity_service.go's ActivityItem: the row plus
// the read-time fields. The server sends ids only; names are resolved by the
// client.
export type ActivityItem = {
  id: string;
  coordinator_id: string;
  workspace_id: string;
  action_class: ActivityClass | (string & {});
  outcome: ActivityOutcome | (string & {});
  authorization: ActivityAuthorization | (string & {});
  target_task_id: string | null;
  proposal_id: string | null;
  actor_user_id: string | null;
  reason_code: string | null;
  detail: string;
  edited: boolean;
  refusal_count: number;
  undone_at: string | null;
  undone_by: string | null;
  undo_of_id: string | null;
  created_at: string;
  updated_at: string;
  undoable: boolean;
  target_task_identifier: string | null;
  from_step_id: string | null;
  // Present only while features.coordinatorPhase3 is effective.
  unattended_turn_id?: string | null;
};

export type ActivityPage = {
  rows: ActivityItem[];
  next_cursor: string | null;
};

export type UndoConflictBody = {
  code: "undo_conflict" | "already_undone" | "not_undoable";
  reason?: string;
};

function activityPath(workspaceId: string, coordinatorId: string, suffix: string): string {
  return `/api/v1/workspaces/${encodeURIComponent(workspaceId)}/coordinators/${encodeURIComponent(coordinatorId)}/activity${suffix}`;
}

/** One page of the coordinator's activity, newest first. The client never sends `limit`. */
export function listActivity(
  workspaceId: string,
  coordinatorId: string,
  params: { class?: ActivityClass; before?: string },
  options?: ApiRequestOptions,
): Promise<ActivityPage> {
  const query = new URLSearchParams();
  if (params.class) query.set("class", params.class);
  if (params.before) query.set("before", params.before);
  const qs = query.toString();
  return fetchJson<ActivityPage>(
    activityPath(workspaceId, coordinatorId, qs ? `?${qs}` : ""),
    options,
  );
}

export function undoActivity(
  workspaceId: string,
  coordinatorId: string,
  rowId: string,
  options?: ApiRequestOptions,
): Promise<ActivityItem> {
  return fetchJson<ActivityItem>(
    activityPath(workspaceId, coordinatorId, `/${encodeURIComponent(rowId)}/undo`),
    { ...options, init: { ...(options?.init ?? {}), method: "POST" } },
  );
}

const UNDO_CODES: ReadonlySet<string> = new Set([
  "undo_conflict",
  "already_undone",
  "not_undoable",
]);

/** The undo 409 body, or null for any other error and for a 409 with a code the client does not know. */
export function getUndoConflict(error: unknown): UndoConflictBody | null {
  if (!(error instanceof ApiError) || error.status !== 409) return null;
  const body = error.body;
  if (!body || typeof body !== "object") return null;
  const { code, reason } = body as { code?: unknown; reason?: unknown };
  if (typeof code !== "string" || !UNDO_CODES.has(code)) return null;
  return {
    code: code as UndoConflictBody["code"],
    ...(typeof reason === "string" ? { reason } : {}),
  };
}

export type ClassSummary = { approved: number; rejected: number };

export type ActivitySummary = {
  days: number;
  classes: Partial<Record<ActivityClass, ClassSummary>>;
};

/** The approved and rejected counts per class over the last `days` days. */
export function getActivitySummary(
  workspaceId: string,
  coordinatorId: string,
  days: number,
  options?: ApiRequestOptions,
): Promise<ActivitySummary> {
  return fetchJson<ActivitySummary>(
    activityPath(workspaceId, coordinatorId, `/summary?days=${days}`),
    options,
  );
}
