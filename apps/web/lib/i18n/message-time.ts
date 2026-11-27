import { formatRelativeTime as formatCounterpartRelative, intlLocale } from "@/lib/i18n/formats";
import { formatRelativeTime as formatCompactRelative } from "@/lib/utils";
import type { MessageTimeDisplay } from "@/lib/types/http-user-settings";
import { parseStrictRfc3339Timestamp } from "@/lib/utils/strict-timestamp";

const WEEK_MS = 7 * 86_400_000;
const NANOSECONDS_PER_MILLISECOND = BigInt(1_000_000);
const ABSOLUTE_SHORT_OPTIONS: Intl.DateTimeFormatOptions = {
  dateStyle: "short",
  timeStyle: "short",
};
const ABSOLUTE_LONG_OPTIONS: Intl.DateTimeFormatOptions = {
  dateStyle: "long",
  timeStyle: "medium",
};
const RELATIVE_DATE_OPTIONS: Intl.DateTimeFormatOptions = {
  year: "numeric",
  month: "numeric",
  day: "numeric",
};
const dateFormatters = new Map<string, Intl.DateTimeFormat>();

function dateFormatter(locale: string, style: "short" | "long" | "relative-date") {
  const key = `${locale}:${style}`;
  const cached = dateFormatters.get(key);
  if (cached) return cached;
  let options: Intl.DateTimeFormatOptions;
  if (style === "short") {
    options = ABSOLUTE_SHORT_OPTIONS;
  } else if (style === "long") {
    options = ABSOLUTE_LONG_OPTIONS;
  } else {
    options = RELATIVE_DATE_OPTIONS;
  }
  const formatter = new Intl.DateTimeFormat(locale, options);
  dateFormatters.set(key, formatter);
  return formatter;
}

/** Resolves regional date conventions without changing the interface language. */
export function resolveMessageTimeLocale(): string {
  const locale = intlLocale();
  let parsed: Intl.Locale;
  try {
    parsed = new Intl.Locale(locale);
  } catch {
    return locale;
  }
  if (parsed.region) return locale;
  const languages = typeof navigator === "undefined" ? [] : navigator.languages;
  for (const language of languages) {
    try {
      const candidate = new Intl.Locale(language);
      if (candidate.language === parsed.language && candidate.region) return language;
    } catch {
      continue;
    }
  }
  return locale;
}

export function formatMessageTime(
  createdAt: string | undefined,
  display: MessageTimeDisplay,
  now: number = Date.now(),
): { label: string; counterpart: string } | null {
  const timestampNs = parseStrictRfc3339Timestamp(createdAt);
  if (timestampNs === null) return null;
  const date = new Date(Number(timestampNs / NANOSECONDS_PER_MILLISECOND));
  const locale = resolveMessageTimeLocale();
  const short = dateFormatter(locale, "short").format(date);
  const relative = formatCounterpartRelative(date, now);
  let label: string;
  if (display === "relative") {
    label =
      now - date.getTime() >= WEEK_MS
        ? dateFormatter(locale, "relative-date").format(date)
        : formatCompactRelative(date, now);
    return { label, counterpart: short };
  }
  label = display === "absolute_long" ? dateFormatter(locale, "long").format(date) : short;
  return { label, counterpart: relative };
}
