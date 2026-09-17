import type {
  Routine,
  ScheduleState,
  UnarmedCronTrigger,
  UnarmedReason,
} from "@/lib/state/slices/office/types";

/**
 * ScheduleStateGroup collapses the nine wire values of `ScheduleState` into
 * the five label groups AC-OFFICE-ROUTINE-ARMING-002.10 requires: two
 * routines in different groups never render the same label.
 */
export type ScheduleStateGroup = "armed" | "broken" | "event_only" | "no_schedule" | "unknown";

const GROUP_BY_STATE: Record<ScheduleState, ScheduleStateGroup> = {
  armed: "armed",
  trigger_invalid: "broken",
  trigger_unscheduled: "broken",
  trigger_disabled: "broken",
  event_only: "event_only",
  unscheduled_manual_only: "no_schedule",
  unscheduled_no_trigger: "no_schedule",
  unknown: "unknown",
};

export function scheduleStateGroup(state: ScheduleState | undefined): ScheduleStateGroup {
  return state ? (GROUP_BY_STATE[state] ?? "unknown") : "unknown";
}

// readScheduleState and readUnarmedCronTriggers tolerate the raw snake_case
// wire shape (`schedule_state`, `unarmed_cron_triggers`, `trigger_id`)
// because fetchJson does not case-convert responses; see routine-row.tsx's
// assigneeAgentProfileId fallback for the established pattern.
export function readScheduleState(routine: Routine): ScheduleState | undefined {
  const raw = routine as unknown as Record<string, unknown>;
  return routine.scheduleState ?? (raw.schedule_state as ScheduleState | undefined);
}

export function readUnarmedCronTriggers(routine: Routine): UnarmedCronTrigger[] {
  const raw = routine as unknown as Record<string, unknown>;
  const list = routine.unarmedCronTriggers ?? (raw.unarmed_cron_triggers as unknown[] | undefined);
  if (!Array.isArray(list)) return [];
  return list.map((entry) => {
    const e = entry as Record<string, unknown>;
    return {
      triggerId: (e.triggerId ?? e.trigger_id) as string,
      reasons: (e.reasons ?? []) as UnarmedReason[],
    };
  });
}

// isSchedulableEntry reports whether this unarmed cron trigger could still
// fire once re-armed (enabled and/or its next occurrence recomputed), as
// opposed to one whose cron expression or timezone needs editing first.
function isSchedulableEntry(entry: UnarmedCronTrigger): boolean {
  return !entry.reasons.includes("not_schedulable");
}

/**
 * hasSchedulableUnarmedEntry implements AC-OFFICE-ROUTINE-ARMING-002.4's
 * distinction: at least one entry that only needs re-arming, versus none.
 * Meaningless over an empty list; the caller suppresses rendering entirely
 * in that case rather than asking the question.
 */
export function hasSchedulableUnarmedEntry(unarmed: UnarmedCronTrigger[]): boolean {
  return unarmed.some(isSchedulableEntry);
}
