import { DEFAULT_LOCALE, i18n, t } from "./index";

/**
 * Locale-aware formatting helpers. The active locale comes from the shared
 * i18next instance. `pseudo` is a QA locale with no real CLDR data, so
 * Intl-based formatters map it to `en`.
 */
function intlLocale(): string {
  const locale = i18n.language || DEFAULT_LOCALE;
  return locale === "pseudo" ? DEFAULT_LOCALE : locale;
}

/**
 * Compact relative time. Behavior-compatible with the former
 * `lib/utils/time.ts` `timeAgo` under `en`: "" for empty/invalid input,
 * "just now" (<60s), "{n}m ago" (<60m), "{n}h ago" (<24h), "{n}d ago" else.
 * The bucket strings are routed through i18next so they extract and pseudolocalize.
 */
export function formatRelative(dateStr: string, now: number = Date.now()): string {
  if (!dateStr) return "";
  const date = new Date(dateStr);
  if (Number.isNaN(date.getTime())) return "";
  const diffSec = Math.floor((now - date.getTime()) / 1000);
  if (diffSec < 60) return t("common:justNow");
  const diffMin = Math.floor(diffSec / 60);
  if (diffMin < 60) return t("common:mAgo", { diffMin });
  const diffHr = Math.floor(diffMin / 60);
  if (diffHr < 24) return t("common:hAgo", { diffHr });
  const diffDay = Math.floor(diffHr / 24);
  return t("common:dAgo", { diffDay });
}

/**
 * Units `formatRelativeTime` picks from, largest first, with the number of
 * seconds in each. Months and years are the conventional 30/365-day
 * approximations — `Intl.RelativeTimeFormat` only phrases a magnitude, so
 * calendar-exact boundaries are neither available nor expected here.
 */
const RELATIVE_UNITS: readonly (readonly [Intl.RelativeTimeFormatUnit, number])[] = [
  ["year", 365 * 24 * 60 * 60],
  ["month", 30 * 24 * 60 * 60],
  ["week", 7 * 24 * 60 * 60],
  ["day", 24 * 60 * 60],
  ["hour", 60 * 60],
  ["minute", 60],
  ["second", 1],
];

/** Anything closer than this to `now` reads as "now" rather than "3 seconds ago". */
const JUST_NOW_SECONDS = 10;

const relativeFormatters = new Map<string, Intl.RelativeTimeFormat>();

function relativeFormatter(): Intl.RelativeTimeFormat {
  const locale = intlLocale();
  const cached = relativeFormatters.get(locale);
  if (cached) return cached;
  // `numeric: "auto"` lets the CLDR data phrase the small magnitudes
  // idiomatically ("yesterday" rather than "1 day ago"), which is the whole
  // reason to go through Intl instead of interpolating a number.
  const formatter = new Intl.RelativeTimeFormat(locale, { numeric: "auto" });
  relativeFormatters.set(locale, formatter);
  return formatter;
}

/**
 * Locale-aware relative time ("3 hours ago", "in 2 days", "yesterday") via
 * `Intl.RelativeTimeFormat`, in the active i18next locale. Future timestamps
 * come back phrased as future. Returns "" for unparseable input.
 *
 * Distinct from `formatRelative` above, which routes a fixed four-bucket
 * ladder through the message catalog for compact UI chips. This one covers
 * the full range up to years and needs no catalog keys, so it is what
 * `host.utils.formatRelativeTime` exposes to plugins.
 */
export function formatRelativeTime(
  value: string | number | Date,
  now: number = Date.now(),
): string {
  const date = value instanceof Date ? value : new Date(value);
  if (Number.isNaN(date.getTime())) return "";

  const formatter = relativeFormatter();
  const diffSec = Math.round((date.getTime() - now) / 1000);
  const magnitude = Math.abs(diffSec);
  if (magnitude < JUST_NOW_SECONDS) return formatter.format(0, "second");

  for (const [unit, seconds] of RELATIVE_UNITS) {
    if (magnitude >= seconds) return formatter.format(Math.trunc(diffSec / seconds), unit);
  }
  return formatter.format(0, "second");
}

export function formatNumber(value: number, options?: Intl.NumberFormatOptions): string {
  return new Intl.NumberFormat(intlLocale(), options).format(value);
}

export function formatDate(
  value: Date | number | string,
  options: Intl.DateTimeFormatOptions = { dateStyle: "medium" },
): string {
  return new Intl.DateTimeFormat(intlLocale(), options).format(new Date(value));
}

export function formatTime(
  value: Date | number | string,
  options: Intl.DateTimeFormatOptions = { timeStyle: "short" },
): string {
  return new Intl.DateTimeFormat(intlLocale(), options).format(new Date(value));
}

export function formatDateTime(
  value: Date | number | string,
  options: Intl.DateTimeFormatOptions = { dateStyle: "medium", timeStyle: "short" },
): string {
  return new Intl.DateTimeFormat(intlLocale(), options).format(new Date(value));
}
