import type { TaskSession } from "@/app/office/tasks/[id]/types";

/**
 * One entry in the unified chat timeline / per-agent tabs.
 *
 * Office sessions (`agentProfileId` set) collapse into one group per agent.
 * Kanban / quick-chat sessions (no `agentProfileId`) get one group each —
 * the legacy per-launch model.
 */
export type SessionGroup = {
  /** The id of the representative session — drives entry id, collapse persistence, and DOM ref. */
  id: string;
  representative: TaskSession;
  group: TaskSession[];
  roleChip: string | null;
};

function sessionSortKey(session: TaskSession): string {
  // Sessions without `startedAt` sort to the end of their tier — fall
  // back to `updatedAt` so they at least cluster in a stable place.
  return session.startedAt || session.updatedAt || "";
}

export function groupSortKey(group: SessionGroup): string {
  // Position the entry by its most-recent session start.
  return sessionSortKey(group.representative);
}

function deriveRoleChip(
  agentProfileId: string | undefined,
  reviewers: string[],
  approvers: string[],
): string | null {
  if (!agentProfileId) return null;
  if (approvers.includes(agentProfileId)) return "Approver";
  if (reviewers.includes(agentProfileId)) return "Reviewer";
  return null;
}

/**
 * Group sessions for the unified timeline.
 *
 * - Office sessions (`agentProfileId` set): one group per agent. The
 *   representative is the most-recent session for that agent.
 * - Kanban sessions (no `agentProfileId`): one group per session — the
 *   pre-spec per-launch model.
 *
 * [REACTIVITY] Callers should memoize on `sessions` since this is O(n).
 */
export function groupSessionsForTimeline(
  sessions: TaskSession[],
  reviewers: string[],
  approvers: string[],
): SessionGroup[] {
  const officeBuckets = new Map<string, TaskSession[]>();
  const kanbanGroups: SessionGroup[] = [];
  for (const session of sessions) {
    if (session.agentProfileId) {
      const list = officeBuckets.get(session.agentProfileId);
      if (list) list.push(session);
      else officeBuckets.set(session.agentProfileId, [session]);
    } else {
      kanbanGroups.push({
        id: session.id,
        representative: session,
        group: [session],
        roleChip: null,
      });
    }
  }
  const officeGroups: SessionGroup[] = [];
  for (const [agentProfileId, group] of officeBuckets) {
    const sorted = [...group].sort((a, b) => sessionSortKey(a).localeCompare(sessionSortKey(b)));
    const representative = sorted[sorted.length - 1];
    officeGroups.push({
      id: representative.id,
      representative,
      group: sorted,
      roleChip: deriveRoleChip(agentProfileId, reviewers, approvers),
    });
  }
  return [...kanbanGroups, ...officeGroups];
}

/** True if the group represents an office (per-agent) session. */
export function isOfficeGroup(g: SessionGroup): boolean {
  return Boolean(g.representative.agentProfileId);
}

/** True if any session in the group is currently RUNNING. */
export function isGroupLive(g: SessionGroup): boolean {
  return g.group.some((s) => s.state === "RUNNING");
}
