import type { TaskSession, TaskSessionState } from "@/lib/types/http";
import type { AgentProfileOption } from "@/lib/state/slices";

const STATUS_ORDER: Record<TaskSessionState, number> = {
  RUNNING: 1,
  STARTING: 1,
  WAITING_FOR_INPUT: 2,
  // Office sessions sit IDLE between turns — order them with WAITING_FOR_INPUT
  // (parked, conversation alive, just not currently running).
  IDLE: 2,
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

/**
 * Orders sessions by the workflow-step flow (left → right): the position of the
 * step a session belongs to, ascending, then by creation time (oldest first)
 * within a step. Sessions with no known step (quick-chat, legacy rows, or a
 * step missing from the map) sort last, after all step-linked sessions, ordered
 * by creation time.
 *
 * This is the single source of truth for the session-tab strip order AND the
 * per-tab number badge, so the two can never disagree (the badge is just the
 * 1-based index into this result).
 */
export function sortSessionsByStepFlow(
  sessions: readonly TaskSession[],
  stepPositionById: Record<string, number>,
): TaskSession[] {
  const UNKNOWN = Number.MAX_SAFE_INTEGER;
  const positionOf = (s: TaskSession): number => {
    const stepId = s.workflow_step_id;
    if (!stepId) return UNKNOWN;
    const pos = stepPositionById[stepId];
    return pos === undefined ? UNKNOWN : pos;
  };
  return [...sessions].sort((a, b) => {
    const posDiff = positionOf(a) - positionOf(b);
    if (posDiff !== 0) return posDiff;
    return new Date(a.started_at).getTime() - new Date(b.started_at).getTime();
  });
}

/**
 * Minimal shape of the app store needed to resolve workflow-step metadata for a
 * session. Merges the single active board (`kanban.steps`) with every
 * multi-board snapshot; step ids are globally unique, so a flat map is safe.
 */
type StepAwareState = {
  kanban?: { steps?: readonly { id: string; position: number; title?: string | null }[] };
  kanbanMulti?: {
    snapshots?: Record<
      string,
      { steps?: readonly { id: string; position: number; title?: string | null }[] }
    >;
  };
};

/** Builds a { stepId → position } map from all loaded workflow step metadata. */
export function buildStepPositionById(state: StepAwareState): Record<string, number> {
  const map: Record<string, number> = {};
  const snapshots = state.kanbanMulti?.snapshots;
  if (snapshots) {
    for (const snapshot of Object.values(snapshots)) {
      for (const step of snapshot.steps ?? []) map[step.id] = step.position;
    }
  }
  for (const step of state.kanban?.steps ?? []) map[step.id] = step.position;
  return map;
}

/** Resolves a workflow step's display title from loaded board metadata.
 * Precedence matches `buildStepTitleById`: the active board (`kanban.steps`)
 * wins over any multi-board snapshot for the same step id, so a tab title and
 * a dropdown/menu label can never disagree for a step that (against the
 * "step ids are globally unique" assumption) appears in both. */
export function resolveWorkflowStepTitle(
  state: StepAwareState,
  stepId: string | null | undefined,
): string | null {
  if (!stepId) return null;
  const active = state.kanban?.steps?.find((step) => step.id === stepId);
  if (active?.title) return active.title;
  const snapshots = state.kanbanMulti?.snapshots;
  if (snapshots) {
    for (const snapshot of Object.values(snapshots)) {
      const found = snapshot.steps?.find((step) => step.id === stepId);
      if (found?.title) return found.title;
    }
  }
  return null;
}

/** Builds a { stepId → title } map from all loaded workflow step metadata. */
export function buildStepTitleById(state: StepAwareState): Record<string, string> {
  const map: Record<string, string> = {};
  const snapshots = state.kanbanMulti?.snapshots;
  if (snapshots) {
    for (const snapshot of Object.values(snapshots)) {
      for (const step of snapshot.steps ?? []) {
        if (step.title) map[step.id] = step.title;
      }
    }
  }
  for (const step of state.kanban?.steps ?? []) {
    if (step.title) map[step.id] = step.title;
  }
  return map;
}

export function buildAgentLabelsById(
  agentProfiles: readonly AgentProfileOption[],
): Record<string, string> {
  return Object.fromEntries(agentProfiles.map((p) => [p.id, p.label]));
}

/**
 * Splits an agent profile's "provider • model" label and picks the more
 * specific second segment, falling back to the first segment or the raw
 * label. Shared by the tab title, mobile pill, and reopen-menu surfaces
 * (each of which previously reimplemented this split independently) so the
 * split convention only needs to change in one place. Returns `null` when
 * there's no profile, letting callers decide their own final fallback
 * (e.g. the tab title's model-derived fallback, vs. the mobile pill's plain
 * "Agent" string) rather than baking one in here.
 */
export function splitAgentProfileLabel(
  profile: { label: string } | null | undefined,
): string | null {
  if (!profile) return null;
  const parts = profile.label.split(" \u2022 ");
  return parts[1] || parts[0] || profile.label;
}

/**
 * Resolves the display label for a session's agent.
 *
 * Store first so that renaming an agent profile is reflected everywhere that
 * calls this (matches the long-standing dropdown behavior). Falls back to the
 * snapshot label only when the profile is no longer in the store — that keeps
 * tabs/rows for sessions whose profile was deleted from rendering as
 * "Unknown agent".
 */
export function resolveAgentLabelFor(
  session: TaskSession,
  agentLabelsById: Record<string, string>,
): string {
  const storeLabel = session.agent_profile_id
    ? (agentLabelsById[session.agent_profile_id] ?? null)
    : null;
  if (storeLabel) return storeLabel;
  const snapshotLabel = (session.agent_profile_snapshot?.label as string | undefined) ?? null;
  if (snapshotLabel) return snapshotLabel;
  return "Unknown agent";
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

export function isSessionActive(state: TaskSessionState | null | undefined): boolean {
  return state === "RUNNING" || state === "STARTING";
}
