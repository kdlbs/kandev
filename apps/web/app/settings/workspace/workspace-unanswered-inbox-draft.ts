export const DEFAULT_UNANSWERED_RETENTION_DAYS = 14;
export const MIN_UNANSWERED_RETENTION_DAYS = 1;
export const MAX_UNANSWERED_RETENTION_DAYS = 90;

export type UnansweredInboxSaved = {
  enabled: boolean;
  retentionDays: number;
};

export type UnansweredInboxUpdates = {
  clarification_unanswered_inbox_enabled?: boolean;
  clarification_unanswered_retention_days?: number;
};

type WorkspaceSettingsSource = {
  clarification_unanswered_inbox_enabled?: boolean;
  clarification_unanswered_retention_days?: number;
};

export function unansweredInboxSavedFrom(workspace: WorkspaceSettingsSource): UnansweredInboxSaved {
  return {
    enabled: workspace.clarification_unanswered_inbox_enabled ?? false,
    retentionDays:
      workspace.clarification_unanswered_retention_days ?? DEFAULT_UNANSWERED_RETENTION_DAYS,
  };
}

/** A retention draft is valid when it is a whole number of days in range. */
export function isValidRetentionDays(value: string): boolean {
  if (!/^\d+$/.test(value)) return false;
  const days = Number(value);
  return days >= MIN_UNANSWERED_RETENTION_DAYS && days <= MAX_UNANSWERED_RETENTION_DAYS;
}

/** Only changed fields are sent, so a partial update keeps the other stored value. */
export function buildUnansweredInboxUpdates(
  draft: { enabled: boolean; retentionDays: string },
  saved: UnansweredInboxSaved,
): UnansweredInboxUpdates {
  const updates: UnansweredInboxUpdates = {};
  if (draft.enabled !== saved.enabled) {
    updates.clarification_unanswered_inbox_enabled = draft.enabled;
  }
  if (isValidRetentionDays(draft.retentionDays)) {
    const days = Number(draft.retentionDays);
    if (days !== saved.retentionDays) updates.clarification_unanswered_retention_days = days;
  }
  return updates;
}
