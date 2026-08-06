import {
  describeSchedule,
  nextRun,
  timeZoneAbbreviation,
} from "@/components/automations/schedule-expression";
import type { Automation, AutomationTrigger } from "@/lib/types/automation";

const UTC = "UTC";

export type ScheduleBinding = {
  /** The scheduled trigger, or null when the automation has none. */
  trigger: AutomationTrigger | null;
  /** Trimmed cron/interval expression. Empty when no schedule is set. */
  expression: string;
  timeZone: string;
};

/**
 * The scheduled trigger's expression and zone, read defensively.
 *
 * `config` is an untyped bag on the wire, so a trigger saved before the
 * composer existed — or one whose config was hand-edited — can carry anything.
 * Reading it as "no schedule" is right: the row then says the automation will
 * not fire on its own, which is true, rather than crashing the whole list.
 */
export function scheduleBinding(automation: Automation): ScheduleBinding {
  const trigger = (automation.triggers ?? []).find((entry) => entry.type === "scheduled") ?? null;
  const config = (trigger?.config ?? {}) as { cron_expression?: unknown; timezone?: unknown };
  const expression =
    typeof config.cron_expression === "string" ? config.cron_expression.trim() : "";
  const timeZone = typeof config.timezone === "string" && config.timezone ? config.timezone : UTC;
  return { trigger, expression, timeZone };
}

/** The schedule in plain words, e.g. "Every day at 09:00 GMT+8". */
export function describeAutomationSchedule(automation: Automation): string {
  const { expression, timeZone } = scheduleBinding(automation);
  return describeSchedule(expression, timeZone);
}

/**
 * The next instant this automation's schedule fires, or null when that cannot
 * be stated (no schedule, an `@every` interval anchored server-side, or an
 * expression the parser does not recognise).
 */
export function nextFiringInstant(automation: Automation, from: Date = new Date()): Date | null {
  const { expression, timeZone } = scheduleBinding(automation);
  if (!expression) return null;
  return nextRun(expression, timeZone, from);
}

const MS_PER_DAY = 86_400_000;

/** Calendar day of an instant as seen in a zone, as a day number for differencing. */
function zonedDayNumber(instant: Date, timeZone: string): number {
  const parts = new Intl.DateTimeFormat("en-US", {
    timeZone,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  }).formatToParts(instant);
  const read = (type: string) => Number(parts.find((part) => part.type === type)?.value ?? "0");
  return Date.UTC(read("year"), read("month") - 1, read("day")) / MS_PER_DAY;
}

/**
 * How the reader names the day. Within the week a weekday name is unambiguous
 * and shorter than a date; past that it stops being one, so fall back to a date.
 */
function dayWord(at: Date, timeZone: string, dayDelta: number): string {
  if (dayDelta <= 0) return "today";
  if (dayDelta === 1) return "tomorrow";
  if (dayDelta < 7) {
    return new Intl.DateTimeFormat("en-GB", { timeZone, weekday: "long" }).format(at);
  }
  return new Intl.DateTimeFormat("en-GB", { timeZone, day: "numeric", month: "short" }).format(at);
}

/**
 * The resolved firing, e.g. `~00:00 tomorrow · GMT+8`.
 *
 * The tilde is load-bearing. The scheduler wakes on an interval and fires the
 * first tick at or after the cron instant, so the real firing lands a little
 * after this time. Printing a bare `00:00` would claim a precision the system
 * does not have, and a user who sees the run land at 00:00:47 would read the
 * difference as a bug rather than as how it works.
 */
export function formatNextFiring(at: Date, timeZone: string, now: Date = new Date()): string {
  const zone = timeZone || UTC;
  const time = new Intl.DateTimeFormat("en-GB", {
    timeZone: zone,
    hour: "2-digit",
    minute: "2-digit",
    hourCycle: "h23",
  }).format(at);
  const delta = zonedDayNumber(at, zone) - zonedDayNumber(now, zone);
  return `~${time} ${dayWord(at, zone, delta)} · ${timeZoneAbbreviation(zone, at)}`;
}
