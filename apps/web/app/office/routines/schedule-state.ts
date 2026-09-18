import type { Routine, ScheduleState, UnarmedCronTrigger } from "@/lib/state/slices/office/types";

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

// The API normalizer translates the wire fields before a routine reaches the
// UI. These readers keep the component code independent of that wire shape.
export function readScheduleState(routine: Routine): ScheduleState | undefined {
  return routine.scheduleState;
}

export function readUnarmedCronTriggers(routine: Routine): UnarmedCronTrigger[] {
  return routine.unarmedCronTriggers ?? [];
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
