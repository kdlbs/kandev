import { describe, expect, it } from "vitest";
import type { RoutineTrigger } from "@/lib/state/slices/office/types";
import { selectPrimaryCronTrigger } from "./routine-trigger-selection";

const TIMESTAMP = "2026-01-01T00:00:00Z";
const NEXT_RUN_AT = "2026-05-05T00:00:00Z";

function trigger(overrides: Partial<RoutineTrigger>): RoutineTrigger {
  return {
    id: "trigger-a",
    routineId: "routine-1",
    kind: "cron",
    cronExpression: "*/5 * * * *",
    timezone: "UTC",
    enabled: true,
    createdAt: TIMESTAMP,
    updatedAt: TIMESTAMP,
    ...overrides,
  };
}

describe("selectPrimaryCronTrigger", () => {
  it("selects the enabled cron trigger with the earliest resolvable nextRunAt (AC-003.1)", () => {
    const later = trigger({ id: "b", nextRunAt: "2026-05-06T00:00:00Z" });
    const earlier = trigger({ id: "a", nextRunAt: NEXT_RUN_AT });
    const result = selectPrimaryCronTrigger([later, earlier]);
    expect(result).toEqual({ trigger: earlier, isPrimary: true });
  });

  it("tiebreaks equal nextRunAt values by id ascending in byte order (AC-003.1)", () => {
    const b = trigger({ id: "b", nextRunAt: NEXT_RUN_AT });
    const a = trigger({ id: "a", nextRunAt: NEXT_RUN_AT });
    const result = selectPrimaryCronTrigger([b, a]);
    expect(result.trigger?.id).toBe("a");
  });

  it("excludes a disabled cron trigger from primary selection even with a valid nextRunAt", () => {
    const disabled = trigger({ id: "a", enabled: false, nextRunAt: NEXT_RUN_AT });
    const result = selectPrimaryCronTrigger([disabled]);
    expect(result).toEqual({ trigger: disabled, isPrimary: false });
  });

  it("excludes a cron trigger whose nextRunAt does not parse as a valid timestamp", () => {
    const malformed = trigger({ id: "a", nextRunAt: "not-a-date" });
    const result = selectPrimaryCronTrigger([malformed]);
    expect(result).toEqual({ trigger: malformed, isPrimary: false });
  });

  it("excludes a cron trigger whose nextRunAt Date.parse would silently accept, pinning parseTurnTimestamp over Date.parse (AC-003.1)", () => {
    // "2026-02-30" is not a real calendar date, but `Date.parse` normalizes
    // it to March 2 rather than rejecting it. Only `parseTurnTimestamp`
    // rejects this input; a selector using `Date.parse` would wrongly treat
    // this trigger as primary.
    const calendarInvalid = trigger({ id: "a", nextRunAt: "2026-02-30T00:00:00Z" });
    const result = selectPrimaryCronTrigger([calendarInvalid]);
    expect(result).toEqual({ trigger: calendarInvalid, isPrimary: false });
  });

  it("falls back to the lowest-id cron trigger, enabled or not, when no primary exists (AC-003.3)", () => {
    const noPrimaryA = trigger({ id: "b", enabled: false });
    const noPrimaryB = trigger({ id: "a", enabled: true }); // no nextRunAt at all
    const result = selectPrimaryCronTrigger([noPrimaryA, noPrimaryB]);
    expect(result).toEqual({ trigger: noPrimaryB, isPrimary: false });
  });

  it("returns no trigger when the routine has no cron trigger (AC-003.4)", () => {
    const webhook = trigger({ id: "a", kind: "webhook" });
    const result = selectPrimaryCronTrigger([webhook]);
    expect(result).toEqual({ trigger: undefined, isPrimary: false });
  });

  it("returns no trigger for an empty trigger list", () => {
    expect(selectPrimaryCronTrigger([])).toEqual({ trigger: undefined, isPrimary: false });
  });

  it("ignores non-cron triggers when a cron trigger is also present", () => {
    const webhook = trigger({ id: "a", kind: "webhook" });
    const cron = trigger({ id: "b", nextRunAt: NEXT_RUN_AT });
    const result = selectPrimaryCronTrigger([webhook, cron]);
    expect(result).toEqual({ trigger: cron, isPrimary: true });
  });
});
