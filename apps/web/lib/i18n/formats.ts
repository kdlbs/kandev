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
 * Compact relative time without the "ago" suffix — for rows that already
 * carry an elapsed-time affordance (e.g. the prompt history duration
 * hourglass), where "5h ago" would be redundant. Same buckets as
 * `formatRelative`; "just now" is kept as-is.
 */
export function formatRelativeCompact(dateStr: string, now: number = Date.now()): string {
  if (!dateStr) return "";
  const date = new Date(dateStr);
  if (Number.isNaN(date.getTime())) return "";
  const diffSec = Math.floor((now - date.getTime()) / 1000);
  if (diffSec < 60) return t("common:justNow");
  const diffMin = Math.floor(diffSec / 60);
  if (diffMin < 60) return t("common:mShort", { diffMin });
  const diffHr = Math.floor(diffMin / 60);
  if (diffHr < 24) return t("common:hShort", { diffHr });
  const diffDay = Math.floor(diffHr / 24);
  return t("common:dShort", { diffDay });
}

/**
 * Sidebar-only elapsed time with a stable, direction-free visual shape. Unlike
 * `formatRelativeTime`, this deliberately uses floor-based elapsed buckets and
 * catalog-backed unit tokens so task rows stay compact and aligned.
 */
export function formatSidebarElapsedTime(dateStr: string, now: number = Date.now()): string {
  if (!dateStr) return "";
  const date = new Date(dateStr);
  if (Number.isNaN(date.getTime())) return "";

  const elapsedSeconds = Math.max(0, Math.floor((now - date.getTime()) / 1000));
  if (elapsedSeconds < 60) return t("common:sidebarSeconds", { count: elapsedSeconds });

  const elapsedMinutes = Math.floor(elapsedSeconds / 60);
  if (elapsedMinutes < 60) return t("common:sidebarMinutes", { count: elapsedMinutes });

  const elapsedHours = Math.floor(elapsedMinutes / 60);
  if (elapsedHours < 24) return t("common:sidebarHours", { count: elapsedHours });

  const elapsedDays = Math.floor(elapsedHours / 24);
  if (elapsedDays < 7) return t("common:sidebarDays", { count: elapsedDays });

  const elapsedWeeks = Math.floor(elapsedDays / 7);
  if (elapsedDays < 365) return t("common:sidebarWeeks", { count: elapsedWeeks });

  const elapsedYears = Math.floor(elapsedDays / 365);
  return elapsedYears > 99
    ? t("common:sidebarYearsMax")
    : t("common:sidebarYears", { count: elapsedYears });
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

/**
 * Anything closer than this to `now` reads as "now" rather than "3 seconds
 * ago". Compared against raw milliseconds, deliberately: testing it against
 * the already-rounded second count moved the real cutoff to 9.5s, and made it
 * asymmetric, because `Math.round(-9.5)` is `-9` while `Math.round(9.5)` is
 * `10` — so 9.5s in the past read "now" while 9.5s in the future read "in 10
 * seconds".
 */
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
  const diffMs = date.getTime() - now;
  if (Math.abs(diffMs) < JUST_NOW_SECONDS * 1000) return formatter.format(0, "second");

  // Rounded, not truncated, so unit selection lands on the nearest unit —
  // 59.6s reads "1 minute ago", which describes it better than "59 seconds
  // ago". That rounding is intended for the unit choice; it just must not
  // decide the "now" threshold above.
  const diffSec = Math.round(diffMs / 1000);
  const magnitude = Math.abs(diffSec);
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
