export const DEFAULT_WIP_RELEASE_GRACE_MINUTES = 10;
/** Upper bound shared with the backend: one year, far below duration overflow. */
export const MAX_WIP_RELEASE_GRACE_MINUTES = 525600;

export type WipReleaseSaved = {
  enabled: boolean;
  graceMinutes: number;
};

export type WipReleaseUpdates = {
  wip_sessionless_release_enabled?: boolean;
  wip_sessionless_release_grace_minutes?: number;
};

export function wipReleaseSavedFrom(workspace: {
  wip_sessionless_release_enabled?: boolean;
  wip_sessionless_release_grace_minutes?: number;
}): WipReleaseSaved {
  return {
    enabled: workspace.wip_sessionless_release_enabled ?? false,
    graceMinutes:
      workspace.wip_sessionless_release_grace_minutes ?? DEFAULT_WIP_RELEASE_GRACE_MINUTES,
  };
}

export function isValidGraceMinutes(value: string): boolean {
  return /^\d+$/.test(value) && Number(value) > 0 && Number(value) <= MAX_WIP_RELEASE_GRACE_MINUTES;
}

/** Returns only the fields the draft changed relative to the saved policy. */
export function buildWipReleaseUpdates(
  draft: { enabled: boolean; graceMinutes: string },
  saved: WipReleaseSaved,
): WipReleaseUpdates {
  const updates: WipReleaseUpdates = {};
  if (draft.enabled !== saved.enabled) updates.wip_sessionless_release_enabled = draft.enabled;
  const grace = Number(draft.graceMinutes);
  if (isValidGraceMinutes(draft.graceMinutes) && grace !== saved.graceMinutes) {
    updates.wip_sessionless_release_grace_minutes = grace;
  }
  return updates;
}

/**
 * Returns the value a draft field takes after a save: the saved server value
 * when the field was not edited while the save was in flight, otherwise the
 * in-flight edit, which then stays dirty against the new baseline.
 */
export function settleDraftField<T>(current: T, requested: T, saved: T): T {
  return current === requested ? saved : current;
}
