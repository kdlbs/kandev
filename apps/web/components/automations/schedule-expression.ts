/**
 * Schedule expression helpers for the automation editor.
 *
 * The backend accepts two categorically different kinds of schedule and treats
 * them differently (see internal/automation/scheduler.go):
 *
 *   - `@every <duration>` is a pure interval. It is timezone-independent by
 *     nature and its next firing depends on a server-side anchor, so it has no
 *     wall-clock meaning the UI can predict.
 *   - Everything else is a wall-clock cron evaluated in a timezone. An empty
 *     timezone means UTC.
 *
 * This module converts between a stored expression and the structured shape the
 * schedule composer edits, and computes the next firing so the editor can show
 * the resolved instant instead of leaving it implicit.
 */
import { t } from "@/lib/i18n";

export type Frequency =
  | "every-5m"
  | "every-15m"
  | "every-30m"
  | "every-6h"
  | "hourly"
  | "daily"
  | "weekly"
  | "custom";

export type ScheduleSpec = {
  frequency: Frequency;
  /** Minute of the hour. Used by hourly, daily and weekly. */
  minute: number;
  /** Hour of the day, 0-23. Used by daily and weekly. */
  hour: number;
  /** Day of week, 0 = Sunday. Used by weekly. */
  weekday: number;
  /** Verbatim expression. Authoritative when frequency is "custom". */
  expression: string;
};

const INTERVAL_EXPRESSIONS: Record<string, Frequency> = {
  "@every 5m": "every-5m",
  "@every 15m": "every-15m",
  "@every 30m": "every-30m",
  "@every 6h": "every-6h",
};

const FREQUENCY_EXPRESSIONS: Partial<Record<Frequency, string>> = {
  "every-5m": "@every 5m",
  "every-15m": "@every 15m",
  "every-30m": "@every 30m",
  "every-6h": "@every 6h",
};

export const DEFAULT_SPEC: ScheduleSpec = {
  frequency: "daily",
  minute: 0,
  hour: 9,
  weekday: 1,
  expression: "0 9 * * *",
};

/**
 * A schedule only carries a timezone when it is a wall-clock expression. This
 * mirrors the backend check that decides whether to bind a CRON_TZ prefix.
 */
export function usesTimezone(expression: string): boolean {
  const trimmed = expression.trim();
  if (!trimmed) return false;
  return !trimmed.startsWith("@every");
}

/**
 * Expand the named descriptors the backend accepts into 5-field form.
 *
 * The set has to match `cron.Descriptor` exactly (scheduler.go wires that
 * option into its parser). A descriptor missing here fails `toFields`, so a
 * schedule the backend runs happily falls back to the custom editor and the
 * structured controls never load for it.
 */
function expandDescriptor(expression: string): string | null {
  switch (expression) {
    case "@hourly":
      return "0 * * * *";
    case "@daily":
    case "@midnight":
      return "0 0 * * *";
    case "@weekly":
      return "0 0 * * 0";
    case "@monthly":
      return "0 0 1 * *";
    case "@yearly":
    case "@annually":
      return "0 0 1 1 *";
    default:
      return null;
  }
}

type CronFields = { minute: string; hour: string; dom: string; month: string; dow: string };

function toFields(expression: string): CronFields | null {
  const parts = (expandDescriptor(expression) ?? expression).split(/\s+/);
  if (parts.length !== 5) return null;
  const [minute, hour, dom, month, dow] = parts;
  return { minute, hour, dom, month, dow };
}

const PLAIN_NUMBER = /^\d+$/;

/**
 * Only the exact shapes the composer itself produces map back onto structured
 * controls. Anything else stays custom, so a hand-written cron is never
 * silently rewritten into something narrower.
 */
function parseFields(fields: CronFields, expression: string): ScheduleSpec {
  const { minute, hour, dom, month, dow } = fields;
  const everyDay = dom === "*" && month === "*";
  if (!PLAIN_NUMBER.test(minute) || !everyDay) {
    return { ...DEFAULT_SPEC, frequency: "custom", expression };
  }

  if (hour === "*" && dow === "*") {
    return { ...DEFAULT_SPEC, frequency: "hourly", minute: Number(minute), expression };
  }
  if (!PLAIN_NUMBER.test(hour)) {
    return { ...DEFAULT_SPEC, frequency: "custom", expression };
  }

  const base = { ...DEFAULT_SPEC, minute: Number(minute), hour: Number(hour), expression };
  if (dow === "*") return { ...base, frequency: "daily" };
  if (PLAIN_NUMBER.test(dow)) {
    return { ...base, frequency: "weekly", weekday: Number(dow) % 7 };
  }
  return { ...DEFAULT_SPEC, frequency: "custom", expression };
}

export function parseExpression(expression: string): ScheduleSpec {
  const trimmed = expression.trim();
  if (!trimmed) return { ...DEFAULT_SPEC };

  const interval = INTERVAL_EXPRESSIONS[trimmed];
  if (interval) return { ...DEFAULT_SPEC, frequency: interval, expression: trimmed };

  const fields = toFields(trimmed);
  if (!fields) return { ...DEFAULT_SPEC, frequency: "custom", expression: trimmed };
  return parseFields(fields, trimmed);
}

export function buildExpression(spec: ScheduleSpec): string {
  const interval = FREQUENCY_EXPRESSIONS[spec.frequency];
  if (interval) return interval;
  switch (spec.frequency) {
    case "hourly":
      return `${spec.minute} * * * *`;
    case "daily":
      return `${spec.minute} ${spec.hour} * * *`;
    case "weekly":
      return `${spec.minute} ${spec.hour} * * ${spec.weekday}`;
    default:
      return spec.expression.trim();
  }
}

/* -------------------------------------------------------------------------
 * Cron field matching
 * ---------------------------------------------------------------------- */

type FieldRange = { from: number; to: number; step: number };

/**
 * Spelled month and weekday values, as the scheduler's parser accepts them.
 *
 * The editor validates "0 9 * * MON-FRI" as a schedule the backend will run,
 * so the preview has to be able to read it too. Parsing names as numbers
 * yielded NaN, which failed every day of the search and left the next-firing
 * line silently blank for an expression that fires perfectly well.
 */
const CRON_NAME_VALUES: Record<string, number> = {
  jan: 1,
  feb: 2,
  mar: 3,
  apr: 4,
  may: 5,
  jun: 6,
  jul: 7,
  aug: 8,
  sep: 9,
  oct: 10,
  nov: 11,
  dec: 12,
  sun: 0,
  mon: 1,
  tue: 2,
  wed: 3,
  thu: 4,
  fri: 5,
  sat: 6,
};

/** A cron field value: a number, or the name the scheduler accepts for it. */
function cronValue(text: string): number {
  const named = CRON_NAME_VALUES[text.trim().toLowerCase()];
  return named === undefined ? Number(text) : named;
}

/** One comma-separated term: a star, a value, or a range, optionally stepped. */
function parseTerm(term: string, min: number, max: number): FieldRange | null {
  const [spec, stepText] = term.split("/");
  const step = stepText === undefined ? 1 : Number(stepText);
  if (!Number.isFinite(step) || step < 1) return null;
  // "?" is the scheduler's alias for "*" in the two day fields.
  if (spec === "*" || spec === "" || spec === "?") return { from: min, to: max, step };

  const [startText, endText] = spec.split("-");
  const from = cronValue(startText);
  if (!Number.isFinite(from)) return null;

  if (endText !== undefined) {
    const to = cronValue(endText);
    return Number.isFinite(to) ? { from, to, step } : null;
  }
  // A bare number carrying a step means "from here onwards", e.g. 5/10.
  return { from, to: stepText === undefined ? from : max, step };
}

function fieldMatches(field: string, value: number, min: number, max: number): boolean {
  return field.split(",").some((term) => {
    const range = parseTerm(term, min, max);
    if (!range) return false;
    if (value < range.from || value > range.to) return false;
    return (value - range.from) % range.step === 0;
  });
}

/**
 * Whether a day field narrows the schedule at all.
 *
 * The wildcard spellings must be the same set `parseTerm` accepts, or the two
 * disagree about what a field means. `?` was the case that bit: `parseTerm`
 * reads it as the full range, but this read it as a restriction, so
 * `0 9 ? * MON` counted both day fields as restricted and took the OR branch —
 * where the `?` side matches every date — and the preview claimed 09:00 daily
 * instead of Mondays.
 */
function isRestricted(field: string): boolean {
  return !field.split(",").some((term) => {
    const spec = term.split("/")[0];
    return spec === "*" || spec === "" || spec === "?";
  });
}

/**
 * Standard cron day rule, matching the backend: when day-of-month and
 * day-of-week are both restricted the schedule fires when EITHER matches;
 * otherwise both must match.
 */
function dayMatches(fields: CronFields, day: Date): boolean {
  if (!fieldMatches(fields.month, day.getUTCMonth() + 1, 1, 12)) return false;
  const domOk = fieldMatches(fields.dom, day.getUTCDate(), 1, 31);
  // Sunday is spelled 0 or 7; getUTCDay only ever says 0, so the 7 form has to
  // be offered explicitly or a schedule written that way never matches.
  const weekday = day.getUTCDay();
  const dowOk =
    fieldMatches(fields.dow, weekday, 0, 7) || (weekday === 0 && fieldMatches(fields.dow, 7, 0, 7));
  if (isRestricted(fields.dom) && isRestricted(fields.dow)) return domOk || dowOk;
  return domOk && dowOk;
}

/** Earliest matching time on a day, strictly after the given hour:minute. */
function firstTimeAfter(
  fields: CronFields,
  afterHour: number,
  afterMinute: number,
): { hour: number; minute: number } | null {
  for (let hour = 0; hour < 24; hour++) {
    if (!fieldMatches(fields.hour, hour, 0, 23)) continue;
    for (let minute = 0; minute < 60; minute++) {
      if (!fieldMatches(fields.minute, minute, 0, 59)) continue;
      if (hour < afterHour) continue;
      if (hour === afterHour && minute <= afterMinute) continue;
      return { hour, minute };
    }
  }
  return null;
}

/* -------------------------------------------------------------------------
 * Timezone arithmetic
 * ---------------------------------------------------------------------- */

type ZonedParts = { year: number; month: number; day: number; hour: number; minute: number };

function zonedParts(instant: Date, timeZone: string): ZonedParts {
  const parts = new Intl.DateTimeFormat("en-US", {
    timeZone,
    hourCycle: "h23",
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).formatToParts(instant);

  const read = (type: string) => Number(parts.find((part) => part.type === type)?.value ?? "0");
  return {
    year: read("year"),
    month: read("month"),
    day: read("day"),
    hour: read("hour"),
    minute: read("minute"),
  };
}

/**
 * Convert a wall-clock reading in `timeZone` back to the instant it names.
 * Two correction passes settle the offset, which is enough to land on the
 * right side of a DST transition.
 */
function wallClockToInstant(parts: ZonedParts, timeZone: string): Date {
  const target = Date.UTC(parts.year, parts.month - 1, parts.day, parts.hour, parts.minute);
  let instant = target;
  for (let pass = 0; pass < 2; pass++) {
    const seen = zonedParts(new Date(instant), timeZone);
    const seenAsUtc = Date.UTC(seen.year, seen.month - 1, seen.day, seen.hour, seen.minute);
    instant = target - (seenAsUtc - instant);
  }
  return new Date(instant);
}

/**
 * The next instant the expression fires, or null when that cannot be stated:
 * an `@every` interval (anchored server-side), an unparseable expression, or
 * nothing within a year.
 */
export function nextRun(
  expression: string,
  timeZone: string,
  from: Date = new Date(),
): Date | null {
  const trimmed = expression.trim();
  if (!trimmed || trimmed.startsWith("@every")) return null;

  const fields = toFields(trimmed);
  if (!fields) return null;

  const zone = timeZone || "UTC";
  let start: ZonedParts;
  try {
    start = zonedParts(from, zone);
  } catch {
    return null;
  }

  for (let offset = 0; offset <= 366; offset++) {
    const day = new Date(Date.UTC(start.year, start.month - 1, start.day + offset));
    if (!dayMatches(fields, day)) continue;
    const today = offset === 0;
    const time = firstTimeAfter(fields, today ? start.hour : -1, today ? start.minute : -1);
    if (!time) continue;
    return wallClockToInstant(
      {
        year: day.getUTCFullYear(),
        month: day.getUTCMonth() + 1,
        day: day.getUTCDate(),
        ...time,
      },
      zone,
    );
  }
  return null;
}

/* -------------------------------------------------------------------------
 * Presentation
 * ---------------------------------------------------------------------- */

export function browserTimeZone(): string {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC";
  } catch {
    return "UTC";
  }
}

export function timeZoneOptions(): string[] {
  const supported = (Intl as unknown as { supportedValuesOf?: (key: string) => string[] })
    .supportedValuesOf;
  let zones: string[] = [];
  try {
    zones = supported?.("timeZone") ?? [];
  } catch {
    zones = [];
  }
  if (!zones.length) zones = [browserTimeZone()];
  // Intl omits UTC from supportedValuesOf — 418 IANA zones, none of them "UTC" —
  // while an empty timezone means UTC everywhere else in this feature. Returning
  // that list unaltered let a schedule be moved off UTC with no way back to it.
  return ["UTC", ...zones].filter((zone, index, all) => all.indexOf(zone) === index);
}

/**
 * Short zone label for the preview line, e.g. "GMT+8".
 *
 * The zero offset is normalised because ICU does not agree with itself about
 * it: a full-ICU Node renders "GMT", the slimmer build in CI renders "GMT+0".
 * Leaving that through would make the label depend on which binary happened to
 * render it, and "GMT+0" is the worse of the two readings anyway — an offset of
 * zero is the one case where naming the offset adds nothing.
 */
export function timeZoneAbbreviation(timeZone: string, at: Date = new Date()): string {
  try {
    const parts = new Intl.DateTimeFormat("en-US", {
      timeZone,
      timeZoneName: "shortOffset",
    }).formatToParts(at);
    const label = parts.find((part) => part.type === "timeZoneName")?.value ?? timeZone;
    return /^GMT[+-]0+$/.test(label) ? "GMT" : label;
  } catch {
    return timeZone;
  }
}

// Catalog KEYS, not copy: this array is built once at module load, so resolving
// it here would freeze the weekday names at the boot locale (docs/i18n.md,
// "no module-scope t()"). `describeSchedule` resolves them per call instead.
const WEEKDAY_KEYS = [
  "common:weekdaySunday",
  "common:weekdayMonday",
  "common:weekdayTuesday",
  "common:weekdayWednesday",
  "common:weekdayThursday",
  "common:weekdayFriday",
  "common:weekdaySaturday",
];

const INTERVAL_LABEL_KEYS: Partial<Record<Frequency, string>> = {
  "every-5m": "automations:scheduleIntervalEvery5Minutes",
  "every-15m": "automations:scheduleIntervalEvery15Minutes",
  "every-30m": "automations:scheduleIntervalEvery30Minutes",
  "every-6h": "automations:scheduleIntervalEvery6Hours",
};

const pad = (value: number) => String(value).padStart(2, "0");

/**
 * One-line summary for lists. Always states the time and zone for a wall-clock
 * schedule — "Every day" alone is true but tells nobody when it runs.
 */
export function describeSchedule(expression: string, timeZone: string): string {
  const trimmed = expression.trim();
  if (!trimmed) return t("automations:scheduleNone");

  const spec = parseExpression(trimmed);
  const intervalKey = INTERVAL_LABEL_KEYS[spec.frequency];
  if (intervalKey) return t(intervalKey);

  const zone = timeZone || "UTC";
  const zoneLabel = timeZoneAbbreviation(zone);
  const at = `${pad(spec.hour)}:${pad(spec.minute)}`;

  switch (spec.frequency) {
    case "hourly":
      return t("automations:scheduleEveryHourAt", { minute: pad(spec.minute) });
    case "daily":
      return t("automations:scheduleEveryDayAt", { time: at, zone: zoneLabel });
    case "weekly":
      return t("automations:scheduleEveryWeekdayAt", {
        weekday: t(WEEKDAY_KEYS[spec.weekday]),
        time: at,
        zone: zoneLabel,
      });
    default:
      return `${trimmed} · ${zoneLabel}`;
  }
}

export function formatInZone(instant: Date, timeZone: string): string {
  return new Intl.DateTimeFormat("en-GB", {
    timeZone,
    weekday: "short",
    day: "numeric",
    month: "short",
    hour: "2-digit",
    minute: "2-digit",
    hourCycle: "h23",
  }).format(instant);
}
