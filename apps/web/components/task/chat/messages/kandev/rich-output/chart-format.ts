const MAX_CATEGORY_TICK_CHARACTERS = 16;
const ISO_DATE_OR_TIMESTAMP =
  /^\d{4}-\d{2}-\d{2}(?:T\d{2}:\d{2}(?::\d{2}(?:\.\d{1,9})?)?(?:Z|[+-]\d{2}:\d{2}))?$/;

type YAxisFormatters = {
  compact: Intl.NumberFormat;
  significant: Intl.NumberFormat;
  fixed: Intl.NumberFormat;
};

const yAxisFormatterCache = new Map<string, YAxisFormatters>();

function resolveIntlLocale(locale: string): string {
  try {
    return Intl.NumberFormat.supportedLocalesOf([locale])[0] ?? "en-US";
  } catch {
    return "en-US";
  }
}

function clipCategoryTick(value: string): string {
  const characters = Array.from(value);
  if (characters.length <= MAX_CATEGORY_TICK_CHARACTERS) return value;
  return `${characters.slice(0, MAX_CATEGORY_TICK_CHARACTERS - 1).join("")}…`;
}

function yAxisFormatters(locale: string): YAxisFormatters {
  const supportedLocale = resolveIntlLocale(locale);
  const cached = yAxisFormatterCache.get(supportedLocale);
  if (cached) return cached;
  const formatters = {
    compact: new Intl.NumberFormat(supportedLocale, {
      notation: "compact",
      maximumFractionDigits: 1,
    }),
    significant: new Intl.NumberFormat(supportedLocale, { maximumSignificantDigits: 2 }),
    fixed: new Intl.NumberFormat(supportedLocale, { maximumFractionDigits: 2 }),
  };
  yAxisFormatterCache.set(supportedLocale, formatters);
  return formatters;
}

function parseISODate(value: string): Date | null {
  if (!ISO_DATE_OR_TIMESTAMP.test(value)) return null;
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? null : date;
}

export function createXAxisTickFormatter(
  labels: string[],
  locale: string,
): (value: unknown) => string {
  const dates = labels.map(parseISODate);
  const allDates = dates.every((date): date is Date => date !== null);

  if (!allDates) {
    return (value) => clipCategoryTick(String(value ?? ""));
  }

  const supportedLocale = resolveIntlLocale(locale);
  const calendarDays = new Set(dates.map((date) => date.toISOString().slice(0, 10)));
  const includeTime = calendarDays.size < dates.length;
  const formatter = new Intl.DateTimeFormat(supportedLocale, {
    month: "short",
    day: "numeric",
    ...(includeTime ? { hour: "2-digit", minute: "2-digit", hourCycle: "h23" } : {}),
    timeZone: "UTC",
  });

  return (value) => {
    const rawValue = String(value ?? "");
    const date = parseISODate(rawValue);
    return date ? formatter.format(date) : clipCategoryTick(rawValue);
  };
}

export function formatYAxisTick(value: unknown, locale: string): string {
  if (typeof value !== "number" || !Number.isFinite(value)) {
    return clipCategoryTick(String(value ?? ""));
  }

  const absoluteValue = Math.abs(value);
  const formatters = yAxisFormatters(locale);
  if (absoluteValue >= 1_000) {
    return formatters.compact.format(value);
  }

  if (absoluteValue > 0 && absoluteValue < 0.01) {
    return formatters.significant.format(value);
  }

  return formatters.fixed.format(value);
}
