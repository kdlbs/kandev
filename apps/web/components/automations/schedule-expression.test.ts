import { describe, expect, it } from "vitest";

import {
  buildExpression,
  describeSchedule,
  formatInZone,
  nextRun,
  parseExpression,
  timeZoneAbbreviation,
  timeZoneOptions,
  usesTimezone,
  type ScheduleSpec,
} from "./schedule-expression";

const SG = "Asia/Singapore";
const DAILY_9 = "0 9 * * *";
const CUSTOM_WEEKDAYS = "0 9 * * 1-5";
const HOURLY_15 = "15 * * * *";
const EVERY_15M = "@every 15m";

const spec = (overrides: Partial<ScheduleSpec>): ScheduleSpec => ({
  frequency: "daily",
  minute: 0,
  hour: 9,
  weekday: 1,
  expression: "",
  ...overrides,
});

describe("parseExpression", () => {
  it("recognises the interval presets", () => {
    expect(parseExpression(EVERY_15M).frequency).toBe("every-15m");
    expect(parseExpression("@every 6h").frequency).toBe("every-6h");
  });

  it("expands the named descriptors the backend accepts", () => {
    expect(parseExpression("@hourly")).toMatchObject({ frequency: "hourly", minute: 0 });
    expect(parseExpression("@daily")).toMatchObject({ frequency: "daily", hour: 0, minute: 0 });
    expect(parseExpression("@weekly")).toMatchObject({
      frequency: "weekly",
      hour: 0,
      minute: 0,
      weekday: 0,
    });
  });

  it("reads structured shapes back into controls", () => {
    expect(parseExpression("30 9 * * *")).toMatchObject({
      frequency: "daily",
      hour: 9,
      minute: 30,
    });
    expect(parseExpression("0 9 * * 1")).toMatchObject({
      frequency: "weekly",
      hour: 9,
      weekday: 1,
    });
    expect(parseExpression(HOURLY_15)).toMatchObject({ frequency: "hourly", minute: 15 });
  });

  it("keeps an expression it cannot round-trip as custom, verbatim", () => {
    // A weekday range is valid to the backend but has no structured control.
    // Rewriting it into a narrower shape would silently change the schedule.
    const parsed = parseExpression(CUSTOM_WEEKDAYS);
    expect(parsed.frequency).toBe("custom");
    expect(parsed.expression).toBe(CUSTOM_WEEKDAYS);

    expect(parseExpression("*/10 * * * *").frequency).toBe("custom");
    expect(parseExpression("nonsense").frequency).toBe("custom");
  });
});

describe("buildExpression", () => {
  it("emits the cron each frequency means", () => {
    expect(buildExpression(spec({ frequency: "every-5m" }))).toBe("@every 5m");
    expect(buildExpression(spec({ frequency: "hourly", minute: 15 }))).toBe(HOURLY_15);
    expect(buildExpression(spec({ frequency: "daily", hour: 9, minute: 30 }))).toBe("30 9 * * *");
    expect(buildExpression(spec({ frequency: "weekly", hour: 9, minute: 0, weekday: 3 }))).toBe(
      "0 9 * * 3",
    );
  });

  it("passes a custom expression through untouched", () => {
    expect(buildExpression(spec({ frequency: "custom", expression: CUSTOM_WEEKDAYS }))).toBe(
      CUSTOM_WEEKDAYS,
    );
  });

  it("round-trips every structured frequency", () => {
    for (const built of ["@every 30m", HOURLY_15, "30 9 * * *", "0 9 * * 3"]) {
      expect(buildExpression(parseExpression(built))).toBe(built);
    }
  });
});

describe("usesTimezone", () => {
  it("is true for wall-clock schedules and false for intervals", () => {
    expect(usesTimezone(DAILY_9)).toBe(true);
    expect(usesTimezone("@daily")).toBe(true);
    expect(usesTimezone(EVERY_15M)).toBe(false);
    expect(usesTimezone("")).toBe(false);
  });
});

describe("nextRun", () => {
  // 2026-07-30T14:30:00Z — 22:30 in Singapore, so "09:00 daily" has already
  // passed there today and the next firing is tomorrow morning.
  const from = new Date("2026-07-30T14:30:00Z");

  it("resolves a daily schedule in the given timezone", () => {
    const next = nextRun(DAILY_9, SG, from);
    expect(next?.toISOString()).toBe("2026-07-31T01:00:00.000Z");
  });

  it("puts the same expression at a different instant per timezone", () => {
    // This is the whole point of storing a timezone: the identical cron means
    // two different moments, and the editor has to be able to show which.
    const sg = nextRun(DAILY_9, SG, from);
    const utc = nextRun(DAILY_9, "UTC", from);
    expect(sg).not.toBeNull();
    expect(utc).not.toBeNull();
    expect(utc!.getTime() - sg!.getTime()).toBe(8 * 60 * 60 * 1000);
  });

  it("treats an empty timezone as UTC, matching the backend", () => {
    expect(nextRun(DAILY_9, "", from)?.toISOString()).toBe(
      nextRun(DAILY_9, "UTC", from)?.toISOString(),
    );
  });

  it("resolves weekly to the right weekday", () => {
    // 2026-07-30 is a Thursday; the next Monday in Singapore is 2026-08-03.
    const next = nextRun("0 9 * * 1", SG, from);
    expect(next?.toISOString()).toBe("2026-08-03T01:00:00.000Z");
  });

  it("resolves hourly to the next matching minute", () => {
    expect(nextRun(HOURLY_15, "UTC", from)?.toISOString()).toBe("2026-07-30T15:15:00.000Z");
  });

  it("handles ranges and lists the structured controls cannot express", () => {
    // Thursday 14:30 UTC → next weekday-only 09:00 is Friday.
    expect(nextRun(CUSTOM_WEEKDAYS, "UTC", from)?.toISOString()).toBe("2026-07-31T09:00:00.000Z");
    expect(nextRun("0 9,17 * * *", "UTC", from)?.toISOString()).toBe("2026-07-30T17:00:00.000Z");
  });

  it("crosses a DST boundary at the correct local hour", () => {
    // London goes GMT+1 → GMT+0 on 2026-10-25. A 09:00 local schedule stays at
    // 09:00 local, which is a different UTC instant either side of the change.
    const before = nextRun(DAILY_9, "Europe/London", new Date("2026-10-23T12:00:00Z"));
    const after = nextRun(DAILY_9, "Europe/London", new Date("2026-10-26T12:00:00Z"));
    expect(before?.toISOString()).toBe("2026-10-24T08:00:00.000Z");
    expect(after?.toISOString()).toBe("2026-10-27T09:00:00.000Z");
  });

  it("declines to predict an interval, which the server anchors", () => {
    expect(nextRun(EVERY_15M, "UTC", from)).toBeNull();
  });

  it("returns null rather than guessing at an unparseable expression", () => {
    expect(nextRun("nonsense", "UTC", from)).toBeNull();
    expect(nextRun("", "UTC", from)).toBeNull();
  });
});

// The editor accepts named weekday and month fields because the scheduler runs
// them. The preview has to read the same expressions, or it promises nothing
// for a schedule that fires — the failure is silent, which is the worst kind
// here: the line simply does not appear.
describe("nextRun with the named and aliased cron forms", () => {
  // A Thursday, 22:30 in Singapore.
  const from = new Date("2026-07-30T14:30:00Z");

  it("resolves a named weekday range the same as its numeric form", () => {
    const named = nextRun("0 9 * * MON-FRI", SG, from);
    const numeric = nextRun("0 9 * * 1-5", SG, from);
    expect(named).not.toBeNull();
    expect(named?.toISOString()).toBe(numeric?.toISOString());
  });

  it("resolves lowercase weekday lists", () => {
    expect(nextRun("0 9 * * mon,wed,fri", SG, from)).not.toBeNull();
  });

  it("resolves a named month", () => {
    const named = nextRun("0 0 1 JAN *", SG, from);
    const numeric = nextRun("0 0 1 1 *", SG, from);
    expect(named).not.toBeNull();
    expect(named?.toISOString()).toBe(numeric?.toISOString());
  });

  it("treats ? as the wildcard the scheduler treats it as", () => {
    const anyDay = nextRun("0 9 ? * *", SG, from);
    const star = nextRun("0 9 * * *", SG, from);
    expect(anyDay).not.toBeNull();
    expect(anyDay?.toISOString()).toBe(star?.toISOString());
  });

  it("matches Sunday whether it is spelled 0 or 7", () => {
    const zero = nextRun("0 9 * * 0", SG, from);
    const seven = nextRun("0 9 * * 7", SG, from);
    expect(zero).not.toBeNull();
    expect(seven?.toISOString()).toBe(zero?.toISOString());
  });
});

describe("describeSchedule", () => {
  it("always states the time and zone for a wall-clock schedule", () => {
    // "Every day" on its own is true and tells nobody when the thing runs.
    expect(describeSchedule(DAILY_9, SG)).toBe("Every day at 09:00 GMT+8");
    expect(describeSchedule("30 9 * * 1", SG)).toBe("Every Monday at 09:30 GMT+8");
  });

  it("omits the zone where it carries no meaning", () => {
    expect(describeSchedule(EVERY_15M, SG)).toBe("Every 15 minutes");
    expect(describeSchedule(HOURLY_15, SG)).toBe("Every hour at :15");
  });

  it("shows a custom cron verbatim, with its zone", () => {
    expect(describeSchedule(CUSTOM_WEEKDAYS, SG)).toBe("0 9 * * 1-5 · GMT+8");
  });

  it("falls back to UTC when no zone is stored", () => {
    expect(describeSchedule(DAILY_9, "")).toBe("Every day at 09:00 GMT");
  });

  it("says when there is no schedule", () => {
    expect(describeSchedule("", "UTC")).toBe("No schedule");
  });
});

describe("presentation helpers", () => {
  it("labels a zone by its current offset", () => {
    expect(timeZoneAbbreviation(SG, new Date("2026-07-30T00:00:00Z"))).toBe("GMT+8");
  });

  // ICU disagrees with itself about the zero offset: a full-ICU build renders
  // "GMT", the slimmer one in CI renders "GMT+0". Without normalising, the
  // label a user reads would depend on which binary rendered it — and this
  // assertion would pass on a developer machine and fail in CI, which is
  // exactly what it did.
  it("renders the zero offset the same way on every ICU build", () => {
    for (const zone of ["UTC", "Etc/UTC", "Etc/GMT"]) {
      expect(timeZoneAbbreviation(zone, new Date("2026-07-30T00:00:00Z")), zone).toBe("GMT");
    }
  });

  it("offers UTC, which Intl itself omits", () => {
    // supportedValuesOf returns 418 IANA zones and none of them is "UTC", while
    // an empty timezone means UTC everywhere else here. Returning that list
    // unaltered let a schedule move off UTC with no way back.
    expect(timeZoneOptions()).toContain("UTC");
    expect(timeZoneOptions().filter((z) => z === "UTC")).toHaveLength(1);
  });

  it("formats an instant in the target zone", () => {
    expect(formatInZone(new Date("2026-07-31T01:00:00Z"), SG)).toContain("09:00");
  });
});
