import type { TokenUsageMetrics, TokenUsageMetricValue, TokenUsageRow } from "@/lib/types/http";
import { formatDate, formatDateTime, formatNumber, intlLocale } from "@/lib/i18n/formats";
import {
  MAX_SAFE_USAGE_AMOUNT,
  parseUsageInteger,
  usageAmount,
  type UsageIntegerValue,
} from "@/lib/api/domains/token-usage-amounts";
import { parseTurnTimestamp } from "@/lib/state/slices/session/turn-actions";

export const TOKEN_METRIC_KEYS = [
  "input_tokens",
  "output_tokens",
  "cache_read_tokens",
  "cache_write_tokens",
  "reasoning_tokens",
  "total_tokens",
  "turns",
  "cost_subcents",
] as const;

export type TokenMetricKey = (typeof TOKEN_METRIC_KEYS)[number];
export type TokenUsageView = "models" | "daily" | "monthly" | "tasks" | "sessions";
export type SortDirection = "asc" | "desc";
export type TokenUsageSortKey = TokenMetricKey | "effective_cost_per_million" | "label" | "period";

export type TokenUsageAggregate = TokenUsageMetrics &
  Partial<TokenUsageRow> & {
    key: string;
    label: string;
    secondary: string;
    period: string;
    provider: string;
    coverage: string;
  };
export type TokenUsageTrendView = "daily" | "weekly" | "monthly";

export function emptyTokenMetrics(): TokenUsageMetrics {
  return {
    input_tokens: null,
    output_tokens: null,
    cache_read_tokens: null,
    cache_write_tokens: null,
    reasoning_tokens: null,
    total_tokens: null,
    turns: null,
    cost_subcents: null,
    currency: "",
    cost_basis: "unknown",
    cost_coverage: "missing",
    effective_cost_per_million: null,
  };
}

function mergeDimension(left: string, right: string): string {
  if (!left) return right;
  if (!right || left === right) return left;
  return "mixed";
}

function mergeCoverage(left: string, right: string): string {
  if (!left) return right;
  if (!right || left === right) return left;
  if (left === "missing" || right === "missing") return "missing";
  return "partial";
}

function mergeCostBasis(left: string, right: string): string {
  if (!left) return right;
  if (!right || left === right) return left;
  if (left === "unknown" || right === "unknown") return "unknown";
  return "mixed";
}

type TokenMetricsInput = Pick<
  TokenUsageMetrics,
  | TokenMetricKey
  | "currency"
  | "cost_basis"
  | "cost_coverage"
  | "amounts_decimal"
  | "effective_cost_per_million"
> & {
  coverage?: string;
};

export function sumTokenMetrics(rows: TokenMetricsInput[]): TokenUsageMetrics {
  const result = emptyTokenMetrics();
  result.currency = "";
  result.cost_basis = "";
  result.cost_coverage = "";
  let coverage = "";
  const missing = Object.fromEntries(TOKEN_METRIC_KEYS.map((key) => [key, false])) as Record<
    TokenMetricKey,
    boolean
  >;
  const sums: Partial<Record<TokenMetricKey, bigint>> = {};
  for (const row of rows) {
    for (const key of TOKEN_METRIC_KEYS) {
      const amount = usageAmount(row, key);
      if (amount === null) {
        missing[key] = true;
      } else {
        sums[key] = (sums[key] ?? BigInt(0)) + amount;
      }
    }
    result.currency = mergeDimension(result.currency, row.currency);
    result.cost_basis = mergeCostBasis(result.cost_basis, row.cost_basis);
    result.cost_coverage = mergeCoverage(result.cost_coverage, row.cost_coverage);
    coverage = mergeCoverage(coverage, row.coverage ?? "complete");
  }
  for (const key of TOKEN_METRIC_KEYS) {
    const amount = missing[key] ? undefined : sums[key];
    if (amount === undefined) continue;
    if (amount <= MAX_SAFE_USAGE_AMOUNT) result[key] = Number(amount);
    else {
      result.amounts_decimal ??= {};
      result.amounts_decimal[key] = amount.toString();
    }
  }
  if (result.currency === "mixed") {
    result.cost_subcents = null;
    if (result.amounts_decimal) result.amounts_decimal.cost_subcents = null;
  }
  result.effective_cost_per_million = aggregateEffectiveRate(rows, result, coverage);
  return result;
}

function aggregateEffectiveRate(
  rows: TokenMetricsInput[],
  metrics: TokenUsageMetrics,
  coverage: string,
): number | null {
  // A published null also covers unavailable rate-matched attribution and coverage.
  if (!rows.every((row) => Number.isFinite(row.effective_cost_per_million))) return null;
  return calculateEffectiveRate({
    ...metrics,
    coverage,
    effective_cost_per_million: rows[0]?.effective_cost_per_million ?? null,
  });
}

export function calculateEffectiveRate(
  metrics: TokenUsageMetrics & { coverage?: string },
): number | null {
  if (
    !Number.isFinite(metrics.effective_cost_per_million) ||
    metrics.cost_subcents === null ||
    metrics.total_tokens === null ||
    metrics.total_tokens <= 0 ||
    metrics.cost_basis === "unknown" ||
    metrics.cost_basis === "mixed" ||
    metrics.cost_coverage !== "complete" ||
    !metrics.currency ||
    metrics.currency === "mixed" ||
    (metrics.coverage !== undefined && metrics.coverage !== "complete")
  ) {
    return null;
  }
  return (metrics.cost_subcents / 10_000 / metrics.total_tokens) * 1_000_000;
}

function groupKey(row: TokenUsageRow, view: TokenUsageView): string {
  switch (view) {
    case "models":
      return `${row.model}\u0000${row.provider}`;
    case "daily":
      return `${row.period}\u0000${row.calendar ?? ""}\u0000${row.currency}`;
    case "monthly":
      return `${row.period.slice(0, 7)}\u0000${row.calendar ?? ""}\u0000${row.currency}`;
    case "tasks":
      return row.task_id;
    case "sessions":
      return row.session_id;
  }
}

function aggregateLabel(
  row: TokenUsageRow,
  view: TokenUsageView,
): { label: string; secondary: string } {
  const model = row.model || "";
  const provider = row.provider || "";
  switch (view) {
    case "models":
      return { label: model, secondary: provider };
    case "daily":
      return { label: row.period, secondary: "" };
    case "monthly":
      return { label: row.period.slice(0, 7), secondary: "" };
    case "tasks":
      return { label: row.task_id, secondary: [model, provider].filter(Boolean).join(" · ") };
    case "sessions":
      return { label: row.session_id, secondary: [model, provider].filter(Boolean).join(" · ") };
  }
}

export function aggregateTokenRows(
  rows: TokenUsageRow[],
  view: TokenUsageView,
): TokenUsageAggregate[] {
  const grouped = new Map<string, TokenUsageAggregate>();
  for (const row of rows) {
    const key = groupKey(row, view);
    const display = aggregateLabel(row, view);
    const existing = grouped.get(key);
    if (!existing) {
      grouped.set(key, {
        ...row,
        key,
        label: display.label,
        secondary: display.secondary,
        period: view === "monthly" ? row.period.slice(0, 7) : row.period,
        effective_cost_per_million: row.effective_cost_per_million,
      });
      continue;
    }

    const merged = sumTokenMetrics([existing, row]);
    const coverage = mergeCoverage(existing.coverage, row.coverage);
    grouped.set(key, {
      ...existing,
      ...merged,
      key,
      label: existing.label,
      secondary: mergeDimension(existing.secondary, display.secondary),
      period: existing.period,
      provider: mergeDimension(existing.provider, row.provider),
      coverage,
      effective_cost_per_million:
        coverage === "complete" ? merged.effective_cost_per_million : null,
    });
  }
  return [...grouped.values()];
}

/** Breakdown rows are already grouped, sorted and paginated by the server. */
export function presentTokenRows(
  rows: TokenUsageRow[],
  view: TokenUsageView,
): TokenUsageAggregate[] {
  return rows.map((row) => ({ ...row, ...aggregateLabel(row, view) }));
}

export function aggregateTokenTrendRows(
  rows: TokenUsageRow[],
  view: TokenUsageTrendView,
): TokenUsageAggregate[] {
  if (view !== "weekly") {
    return aggregateTokenRows(rows, view);
  }

  const grouped = new Map<string, TokenUsageAggregate>();
  for (const point of aggregateTokenRows(rows, "daily")) {
    const period = weekStartPeriod(point.period);
    if (!period) continue;
    const key = `${period}\u0000${point.calendar ?? ""}\u0000${point.currency}`;
    const existing = grouped.get(key);
    if (!existing) {
      grouped.set(key, {
        ...point,
        key,
        label: period,
        secondary: "",
        period,
      });
      continue;
    }

    const merged = sumTokenMetrics([existing, point]);
    const coverage = mergeCoverage(existing.coverage, point.coverage);
    grouped.set(key, {
      ...existing,
      ...merged,
      key,
      label: period,
      secondary: "",
      period,
      provider: mergeDimension(existing.provider, point.provider),
      coverage,
      effective_cost_per_million:
        coverage === "complete" ? merged.effective_cost_per_million : null,
    });
  }
  return [...grouped.values()];
}

export function sortTokenRows(
  rows: TokenUsageAggregate[],
  sortKey: TokenUsageSortKey,
  direction: SortDirection,
): TokenUsageAggregate[] {
  const multiplier = direction === "asc" ? 1 : -1;
  return [...rows].sort((left, right) => {
    if (sortKey === "label" || sortKey === "period") {
      return left[sortKey].localeCompare(right[sortKey]) * multiplier;
    }
    const leftValue =
      sortKey === "effective_cost_per_million" ? left[sortKey] : usageAmount(left, sortKey);
    const rightValue =
      sortKey === "effective_cost_per_million" ? right[sortKey] : usageAmount(right, sortKey);
    if (leftValue === null || rightValue === null) {
      if (leftValue === null && rightValue === null) return 0;
      return leftValue === null ? 1 : -1;
    }
    if (leftValue === rightValue) return 0;
    return (leftValue < rightValue ? -1 : 1) * multiplier;
  });
}

function weekStartPeriod(value: string): string | null {
  const date = validDateParts(value, 10);
  if (!date) return null;
  const day = date.getUTCDay();
  const daysSinceMonday = day === 0 ? 6 : day - 1;
  date.setUTCDate(date.getUTCDate() - daysSinceMonday);
  return date.toISOString().slice(0, 10);
}

function validDateParts(value: string, expectedLength: number): Date | null {
  const pattern = expectedLength === 10 ? /^(\d{4})-(\d{2})-(\d{2})$/ : /^(\d{4})-(\d{2})$/;
  const match = value.match(pattern);
  if (!match) return null;
  const year = Number(match[1]);
  const month = Number(match[2]);
  const day = expectedLength === 10 ? Number(match[3]) : 1;
  const date = new Date(Date.UTC(year, month - 1, day));
  if (
    date.getUTCFullYear() !== year ||
    date.getUTCMonth() !== month - 1 ||
    date.getUTCDate() !== day
  ) {
    return null;
  }
  return date;
}

export function formatTokenPeriod(value: string, view: TokenUsageView): string {
  const date = validDateParts(value, view === "monthly" ? 7 : 10);
  if (!date) return value;
  return formatDate(
    date,
    view === "monthly"
      ? { month: "long", year: "numeric", timeZone: "UTC" }
      : { dateStyle: "medium", timeZone: "UTC" },
  );
}

export function formatTokens(value: UsageIntegerValue, unavailable: string): string {
  const amount = parseUsageInteger(value);
  return amount === null ? unavailable : new Intl.NumberFormat(intlLocale()).format(amount);
}

export function formatCost(
  value: UsageIntegerValue,
  currency: string,
  unavailable: string,
): string {
  const amount = parseUsageInteger(value);
  if (amount === null || !/^[A-Z]{3}$/.test(currency)) return unavailable;
  const whole = amount / BigInt(10_000);
  const fraction = (amount % BigInt(10_000))
    .toString()
    .padStart(4, "0")
    .replace(/0{1,2}$/, "");
  const digits = formatNumber(Number(fraction), {
    useGrouping: false,
    minimumIntegerDigits: fraction.length,
  });
  return new Intl.NumberFormat(intlLocale(), {
    style: "currency",
    currency,
    minimumFractionDigits: 2,
    maximumFractionDigits: 4,
  })
    .formatToParts(whole)
    .map((part) => (part.type === "fraction" ? digits : part.value))
    .join("");
}

export function formatRate(
  value: TokenUsageMetricValue,
  currency: string,
  unavailable: string,
): string {
  if (value === null || !Number.isFinite(value) || !/^[A-Z]{3}$/.test(currency)) return unavailable;
  return formatCurrencyAmount(value, currency);
}

function formatCurrencyAmount(amount: number, currency: string): string {
  try {
    return formatNumber(amount, {
      style: "currency",
      currency,
      maximumSignificantDigits: 15,
    });
  } catch {
    return `${currency || "USD"} ${formatNumber(amount, { maximumFractionDigits: 2 })}`;
  }
}

export function coverageLabelKey(coverage: string): string {
  switch (coverage) {
    case "complete":
      return "stats:coverageComplete";
    case "partial":
      return "stats:coveragePartial";
    case "stale":
      return "stats:coverageStale";
    case "missing":
      return "stats:coverageMissing";
    default:
      return "stats:coverageUnknown";
  }
}

export function costBasisLabel(value: string, t: (key: string) => string): string {
  switch (value) {
    case "estimated":
      return t("stats:costBasisEstimated");
    case "reported":
      return t("stats:costBasisReported");
    case "mixed":
      return t("stats:costBasisMixed");
    default:
      return t("stats:costBasisUnknown");
  }
}

export function formatLastUpdated(value: string | null | undefined, unavailable: string): string {
  if (!value || parseTurnTimestamp(value) === null) return unavailable;
  return formatDateTime(value);
}
