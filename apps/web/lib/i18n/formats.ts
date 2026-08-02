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
