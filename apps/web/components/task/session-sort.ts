import type { TaskSession, TaskSessionState } from "@/lib/types/http";
import type { AgentProfileOption } from "@/lib/state/slices";

const STATUS_ORDER: Record<TaskSessionState, number> = {
  RUNNING: 1,
  STARTING: 1,
  WAITING_FOR_INPUT: 2,
  CREATED: 3,
  COMPLETED: 4,
  FAILED: 5,
  CANCELLED: 6,
};

export function sortSessions(sessions: readonly TaskSession[]): TaskSession[] {
  return [...sessions].sort((a, b) => {
    const d = (STATUS_ORDER[a.state] ?? 99) - (STATUS_ORDER[b.state] ?? 99);
    return d !== 0 ? d : new Date(b.started_at).getTime() - new Date(a.started_at).getTime();
  });
}

export function buildAgentLabelsById(
  agentProfiles: readonly AgentProfileOption[],
): Record<string, string> {
  return Object.fromEntries(agentProfiles.map((p) => [p.id, p.label]));
}

export function resolveAgentLabelFor(
  session: TaskSession,
  agentLabelsById: Record<string, string>,
): string {
  const snapshotLabel = (session.agent_profile_snapshot?.label as string | undefined) ?? null;
  if (snapshotLabel) return snapshotLabel;
  return (session.agent_profile_id && agentLabelsById[session.agent_profile_id]) || "Unknown agent";
}

export function pickActiveSessionId(
  sessions: readonly TaskSession[],
  preferredSessionId: string | null | undefined,
): string | null {
  if (sessions.length === 0) return null;
  if (preferredSessionId && sessions.some((s) => s.id === preferredSessionId)) {
    return preferredSessionId;
  }
  const primary = sessions.find((s) => s.is_primary);
  return (primary ?? sessions[0]).id;
}
