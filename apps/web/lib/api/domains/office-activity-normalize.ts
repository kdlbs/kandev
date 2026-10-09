import type { ActivityEntry } from "@/lib/state/slices/office/types";

export type RawActivityEntry = ActivityEntry & {
  workspace_id?: string;
  actor_type?: ActivityEntry["actorType"];
  actor_id?: string;
  actor_name?: string;
  target_type?: string;
  target_id?: string;
  target_name?: string;
  target_identifier?: string;
  run_id?: string;
  session_id?: string;
  created_at?: string;
};

function parseDetails(details: unknown): Record<string, unknown> | undefined {
  if (!details) return undefined;
  if (typeof details === "object") return details as Record<string, unknown>;
  if (typeof details !== "string") return undefined;
  try {
    const parsed = JSON.parse(details) as unknown;
    return typeof parsed === "object" && parsed !== null
      ? (parsed as Record<string, unknown>)
      : undefined;
  } catch {
    return undefined;
  }
}

function pick<T>(fallback: T, ...values: Array<T | undefined>): T {
  for (const value of values) {
    if (value !== undefined) return value;
  }
  return fallback;
}

export function normalizeActivityEntry(raw: RawActivityEntry): ActivityEntry {
  return {
    id: pick("", raw.id),
    workspaceId: pick("", raw.workspaceId, raw.workspace_id),
    actorType: pick("system", raw.actorType, raw.actor_type),
    actorId: pick("", raw.actorId, raw.actor_id),
    actorName: pick<string | undefined>(undefined, raw.actorName, raw.actor_name),
    action: pick("", raw.action),
    targetType: pick<string | undefined>(undefined, raw.targetType, raw.target_type),
    targetId: pick<string | undefined>(undefined, raw.targetId, raw.target_id),
    targetName: pick<string | undefined>(undefined, raw.targetName, raw.target_name),
    targetIdentifier: pick<string | undefined>(
      undefined,
      raw.targetIdentifier,
      raw.target_identifier,
    ),
    details: parseDetails(raw.details),
    runId: pick<string | undefined>(undefined, raw.runId, raw.run_id),
    sessionId: pick<string | undefined>(undefined, raw.sessionId, raw.session_id),
    createdAt: pick("", raw.createdAt, raw.created_at),
  };
}
