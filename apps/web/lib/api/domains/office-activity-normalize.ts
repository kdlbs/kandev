import type { ActivityEntry } from "@/lib/state/slices/office/types";

export type RawActivityEntry = ActivityEntry & {
  workspace_id?: string;
  actor_type?: ActivityEntry["actorType"];
  actor_id?: string;
  target_type?: string;
  target_id?: string;
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

export function normalizeActivityEntry(raw: RawActivityEntry): ActivityEntry {
  return {
    id: raw.id ?? "",
    workspaceId: raw.workspaceId ?? raw.workspace_id ?? "",
    actorType: raw.actorType ?? raw.actor_type ?? "system",
    actorId: raw.actorId ?? raw.actor_id ?? "",
    action: raw.action ?? "",
    targetType: raw.targetType ?? raw.target_type,
    targetId: raw.targetId ?? raw.target_id,
    details: parseDetails(raw.details),
    runId: raw.runId ?? raw.run_id,
    sessionId: raw.sessionId ?? raw.session_id,
    createdAt: raw.createdAt ?? raw.created_at ?? "",
  };
}
