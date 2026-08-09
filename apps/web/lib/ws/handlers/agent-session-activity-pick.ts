import type { TaskSession } from "@/lib/types/http";

// The parked triple rides on session.activity_changed alongside
// foreground_activity (Review round 2, F3: the spec's carrier is
// session.activity_changed, not a dedicated event). A frame that omits the
// three fields — any activity_changed publish triggered by something other
// than a parked transition — must leave them untouched rather than reset to
// false/undefined, mirroring agent-session.ts's own
// pickActiveSubagentCount/pickSupportsSteering pattern. Split into this file
// to keep agent-session.ts under its line-count limit.

export function pickParkedOnBackgroundWork(
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  payload: any,
  existing: TaskSession,
): boolean | undefined {
  return payload.parked_on_background_work !== undefined
    ? payload.parked_on_background_work
    : existing.parked_on_background_work;
}

export function pickParkedEpoch(
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  payload: any,
  existing: TaskSession,
): number | undefined {
  return payload.parked_epoch !== undefined ? payload.parked_epoch : existing.parked_epoch;
}

export function pickParkedRevision(
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  payload: any,
  existing: TaskSession,
): number | undefined {
  return payload.parked_revision !== undefined ? payload.parked_revision : existing.parked_revision;
}
