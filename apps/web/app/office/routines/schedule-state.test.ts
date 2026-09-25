import { describe, expect, it } from "vitest";
import type { Routine, ScheduleState, UnarmedCronTrigger } from "@/lib/state/slices/office/types";
import {
  hasSchedulableUnarmedEntry,
  readScheduleState,
  readUnarmedCronTriggers,
  scheduleStateGroup,
} from "./schedule-state";

// AC-OFFICE-ROUTINE-ARMING-002.10: every one of the nine wire values maps to
// exactly one of the five label groups, and two states in different groups
// never collapse to the same group.
describe("scheduleStateGroup", () => {
  it.each<[ScheduleState, string]>([
    ["armed", "armed"],
    ["trigger_invalid", "broken"],
    ["trigger_unscheduled", "broken"],
    ["trigger_disabled", "broken"],
    ["event_only", "event_only"],
    ["unscheduled_manual_only", "no_schedule"],
    ["unscheduled_no_trigger", "no_schedule"],
    ["unknown", "unknown"],
  ])("maps %s to group %s", (state, group) => {
    expect(scheduleStateGroup(state)).toBe(group);
  });

  it("treats a missing state as unknown rather than throwing", () => {
    expect(scheduleStateGroup(undefined)).toBe("unknown");
  });
});

function routineWithRaw(raw: Record<string, unknown>): Routine {
  return raw as unknown as Routine;
}

describe("readScheduleState / readUnarmedCronTriggers", () => {
  it("reads the camelCase field when a mapping layer already provided it", () => {
    const routine = routineWithRaw({ scheduleState: "armed" });
    expect(readScheduleState(routine)).toBe("armed");
  });

  it("returns undefined when the mapping layer has no schedule state", () => {
    expect(readScheduleState(routineWithRaw({}))).toBeUndefined();
  });

  it("reads normalized unarmed cron triggers", () => {
    const routine = routineWithRaw({
      unarmedCronTriggers: [{ triggerId: "trig-1", reasons: ["disabled"] }],
    });
    expect(readUnarmedCronTriggers(routine)).toEqual([
      { triggerId: "trig-1", reasons: ["disabled"] },
    ]);
  });

  it("returns an empty list when the field is absent", () => {
    expect(readUnarmedCronTriggers(routineWithRaw({}))).toEqual([]);
  });
});

describe("hasSchedulableUnarmedEntry", () => {
  it("is true when at least one unarmed entry only needs re-arming", () => {
    const unarmed: UnarmedCronTrigger[] = [
      { triggerId: "t1", reasons: ["not_schedulable"] },
      { triggerId: "t2", reasons: ["disabled"] },
    ];
    expect(hasSchedulableUnarmedEntry(unarmed)).toBe(true);
  });

  it("is false when every unarmed entry needs its expression or timezone edited", () => {
    const unarmed: UnarmedCronTrigger[] = [{ triggerId: "t1", reasons: ["not_schedulable"] }];
    expect(hasSchedulableUnarmedEntry(unarmed)).toBe(false);
  });

  it("is false over an empty list", () => {
    expect(hasSchedulableUnarmedEntry([])).toBe(false);
  });
});
