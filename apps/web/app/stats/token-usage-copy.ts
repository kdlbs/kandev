import type { TokenUsageResponse } from "@/lib/types/http";
import { usageAmount } from "@/lib/api/domains/token-usage-amounts";
import {
  actorAttributionLabel,
  isUsageUnallocated,
  usageCalendarLabel,
  usageCurrencyLabel,
  usageProviderLabel,
  type UsageTranslate,
} from "./token-usage-metadata";
import {
  costBasisLabel,
  coverageLabelKey,
  formatCost,
  formatLastUpdated,
  formatRate,
  formatTokens,
  type TokenUsageView,
} from "./token-usage-utils";

export function buildTokenUsageCopySummary(
  data: TokenUsageResponse,
  rangeLabel: string,
  view: TokenUsageView,
  provider: string,
  t: UsageTranslate,
): string {
  const metrics = data.totals;
  const unavailable = t("common:unavailable");
  const amounts = [
    ["totalTokens", "total_tokens"],
    ["inputTokens", "input_tokens"],
    ["outputTokens", "output_tokens"],
    ["cacheReadTokens", "cache_read_tokens"],
    ["cacheWriteTokens", "cache_write_tokens"],
    ["reasoningTokens", "reasoning_tokens"],
    ["sourceTotalTokens", "source_total_tokens"],
    ["totalTurns", "turns"],
  ] as const;
  const coverageKey = copyCoverageKey(data);
  return [
    t("stats:tokenUsageSummaryHeading", { range: rangeLabel }),
    `${t("stats:view")}: ${t(`stats:${view}`)}`,
    ...(data.rows.some((row) => isUsageUnallocated(row, view))
      ? [t("stats:unallocatedUsageNotice", { label: t("stats:unallocatedUsage") })]
      : []),
    `${t("stats:provider")}: ${provider === "all" ? t("stats:allProviders") : usageProviderLabel(provider, t)}`,
    `${t("stats:cost")}: ${formatCost(usageAmount(metrics, "cost_subcents"), metrics.currency, unavailable)}`,
    `${t("stats:costBasis")}: ${costBasisLabel(metrics.cost_basis, t)}`,
    `${t("stats:currency")}: ${usageCurrencyLabel(metrics.currency, t)}`,
    ...amounts.map(
      ([label, key]) =>
        `${t(`stats:${label}`)}: ${formatTokens(usageAmount(metrics, key), unavailable)}`,
    ),
    `${t("stats:effectiveRate")}: ${formatRate(metrics.effective_cost_per_million, metrics.currency, unavailable)}`,
    `${t("stats:costCoverage")}: ${t(coverageLabelKey(metrics.cost_coverage))}`,
    `${t("stats:coverage")}: ${t(coverageKey)}`,
    `${t("stats:calendar")}: ${usageCalendarLabel(metrics, t)}`,
    ...(metrics.calendar_basis === "source" ? [t("stats:calendarSourceNotice")] : []),
    `${t("stats:tokenUserAttribution")}: ${actorAttributionLabel(metrics.token_source, t)}`,
    `${t("stats:costUserAttribution")}: ${actorAttributionLabel(metrics.cost_source, t)}`,
    `${t("stats:lastUpdated")}: ${formatLastUpdated(data.last_updated, unavailable)}`,
    ...(data.projection_pending ? [t("stats:updatingUsageHistory")] : []),
    ...(data.projection_status === "stale" && !data.projection_pending
      ? [t("stats:usageHistoryStale")]
      : []),
    ...(data.history_gap ? [t("stats:historyGapNotice")] : []),
  ].join("\n");
}

function copyCoverageKey(data: TokenUsageResponse): string {
  if (data.dated_coverage && data.undated_coverage) return "stats:datedAndUndatedCoverage";
  if (data.dated_coverage) return "stats:datedCoverageAvailable";
  return data.undated_coverage ? "stats:undatedCoverageOnly" : "stats:coverageUnavailable";
}
