import type { ForegroundActivity, TaskSession, TaskSessionState } from "@/lib/types/http";
import type { AgentProfileOption } from "@/lib/state/slices";
import { t } from "@/lib/i18n";

export type SessionRow = {
  id: string;
  agentName: string | null;
  agentLabel: string;
  repositoryLabel: string | null;
  state: TaskSessionState | null;
  foregroundActivity: ForegroundActivity | null;
  /** True when the session is waiting on the operator to notice, not to act —
   *  a settled session with a positively-sampled background process still
   *  live (spec: docs/specs/disambiguate-waiting/spec.md). */
  parkedOnBackgroundWork: boolean;
  isPrimary: boolean;
  isHistory: boolean;
  isArchived: boolean;
  index: number;
  startedAt: string;
};

function sessionAgentLabel(session: TaskSession, profile: AgentProfileOption | undefined): string {
  const labelParts = profile?.label.split(" • ") ?? [];
  const snapshotLabel = session.agent_profile_snapshot?.label;
  return (
    session.name ||
    labelParts[1] ||
    labelParts[0] ||
    profile?.label ||
    (typeof snapshotLabel === "string" ? snapshotLabel : null) ||
    t("task:agent")
  );
}

export function buildSessionRows(
  sessions: TaskSession[],
  agentProfiles: AgentProfileOption[],
  primarySessionId: string | null | undefined,
  repositoryLabelsById: ReadonlyMap<string, string>,
): SessionRow[] {
  const sorted = [...sessions].sort(
    (a, b) => new Date(a.started_at).getTime() - new Date(b.started_at).getTime(),
  );
  const knownPrimarySessionId =
    primarySessionId && sessions.some((session) => session.id === primarySessionId)
      ? primarySessionId
      : null;
  return sorted.map((s, idx) => {
    const profile = agentProfiles.find((p) => p.id === s.agent_profile_id);
    const isPrimary = knownPrimarySessionId ? s.id === knownPrimarySessionId : !!s.is_primary;
    return {
      id: s.id,
      agentName: profile?.agent_name ?? null,
      // User-supplied session name wins over the derived profile label,
      // matching the desktop session tab title precedence.
      agentLabel: sessionAgentLabel(s, profile),
      repositoryLabel: s.repository_id ? (repositoryLabelsById.get(s.repository_id) ?? null) : null,
      state: (s.state as TaskSessionState | undefined) ?? null,
      foregroundActivity: s.foreground_activity ?? null,
      parkedOnBackgroundWork: !!s.parked_on_background_work,
      isPrimary,
      isArchived: !!s.archived_at,
      isHistory:
        !!s.archived_at || (!isPrimary && ["COMPLETED", "FAILED", "CANCELLED"].includes(s.state)),
      index: idx + 1,
      startedAt: s.started_at,
    };
  });
}

export function visibleMobileSessionRows(
  rows: SessionRow[],
  showHistory: boolean,
  activeSessionId: string | null,
): SessionRow[] {
  return rows
    .filter((row) => showHistory || row.isPrimary || !row.isHistory || row.id === activeSessionId)
    .map((row, index) => ({ ...row, index: index + 1 }));
}
