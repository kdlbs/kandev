"use client";

import { useMemo, useState } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Tabs, TabsList, TabsTrigger } from "@kandev/ui/tabs";
import { useTranslation } from "react-i18next";
import type { TokenUsageMetrics, TokenUsageResponse, TokenUsageRow } from "@/lib/types/http";
import {
  aggregateTokenRows,
  aggregateTokenTrendRows,
  costBasisLabel,
  formatCost,
  formatLastUpdated,
  formatRate,
  formatTokenPeriod,
  formatTokens,
  type SortDirection,
  type TokenUsageAggregate,
  type TokenUsageSortKey,
  type TokenUsageTrendView,
  type TokenUsageView,
} from "./token-usage-utils";
import { TokenUsageTables } from "./token-usage-table";

const VIEW_KEYS: TokenUsageView[] = ["models", "daily", "monthly", "tasks", "sessions"];
const UNKNOWN_PROVIDER = "__unknown_provider__";
const UNKNOWN_LABEL_KEY = "common:unknown";

type TokenUsageSummaryProps = {
  metrics: TokenUsageMetrics;
  datedCoverage: boolean;
  undatedCoverage: boolean;
  lastUpdated?: string | null;
};

export function TokenUsageSummary({
  metrics,
  datedCoverage,
  undatedCoverage,
  lastUpdated,
}: TokenUsageSummaryProps) {
  const { t } = useTranslation();
  const unavailable = t("common:unavailable");
  const coverageText = coverageSummary(datedCoverage, undatedCoverage, t);

  return (
    <div className="space-y-3" data-testid="token-usage-summary">
      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <SummaryCard
          label={t("stats:estimatedCost")}
          value={formatCost(metrics.cost_subcents, metrics.currency, unavailable)}
          detail={costBasisLabel(metrics.cost_basis, t)}
        />
        <SummaryCard
          label={t("stats:totalTokens")}
          value={formatTokens(metrics.total_tokens, unavailable)}
          detail={t("stats:tokenUsageAllCategories")}
        />
        <SummaryCard
          label={t("stats:inputOutput")}
          value={`${formatTokens(metrics.input_tokens, unavailable)} / ${formatTokens(metrics.output_tokens, unavailable)}`}
          detail={t("stats:inputOutputTokens")}
        />
        <SummaryCard
          label={t("stats:cacheAndReasoning")}
          value={`${formatTokens(metrics.cache_read_tokens, unavailable)} / ${formatTokens(metrics.cache_write_tokens, unavailable)}`}
          detail={`${t("stats:cacheReadWrite")} · ${t("stats:reasoningTokens")}: ${formatTokens(metrics.reasoning_tokens, unavailable)}`}
        />
      </div>
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
        <span>{coverageText}</span>
        {metrics.effective_cost_per_million !== null && (
          <span>
            {t("stats:effectiveRate")}:{" "}
            {formatRate(metrics.effective_cost_per_million, metrics.currency, unavailable)}
          </span>
        )}
        <span>
          {t("stats:lastUpdated")}: {formatLastUpdated(lastUpdated, unavailable)}
        </span>
      </div>
    </div>
  );
}

function SummaryCard({ label, value, detail }: { label: string; value: string; detail: string }) {
  return (
    <Card className="rounded-sm">
      <CardHeader className="pb-2">
        <CardTitle className="text-xs font-medium text-muted-foreground">{label}</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="text-lg font-semibold tabular-nums" data-testid="token-usage-card-value">
          {value}
        </div>
        <div className="mt-1 text-xs text-muted-foreground">{detail}</div>
      </CardContent>
    </Card>
  );
}

type TokenUsageTrendProps = {
  rows: TokenUsageRow[];
  currency: string;
  datedCoverage: boolean;
};

export function TokenUsageTrend({ rows, currency, datedCoverage }: TokenUsageTrendProps) {
  const { t } = useTranslation();
  const [metric, setMetric] = useState<"cost" | "tokens">("cost");
  const [granularity, setGranularity] = useState<TokenUsageTrendView>("daily");
  const unavailable = t("common:unavailable");
  const points = useMemo(
    () =>
      datedCoverage
        ? aggregateTokenTrendRows(rows, granularity)
            .filter((point) =>
              granularity === "monthly"
                ? /^\d{4}-\d{2}$/.test(point.period)
                : /^\d{4}-\d{2}-\d{2}$/.test(point.period),
            )
            .sort((a, b) => a.period.localeCompare(b.period))
            .slice(-14)
        : [],
    [datedCoverage, granularity, rows],
  );
  const values = points.map((point) => {
    const value =
      metric === "cost" && point.currency !== "mixed" ? point.cost_subcents : point.total_tokens;
    if (metric === "cost" && point.currency === "mixed") return null;
    if (value === null || !Number.isFinite(value)) return null;
    return metric === "cost" ? value / 10_000 : value;
  });
  const knownValues = values.filter((value): value is number => value !== null);
  const maxValue = Math.max(...knownValues, 1);

  return (
    <Card className="rounded-sm" data-testid="token-usage-trend">
      <CardHeader className="flex flex-row items-center justify-between gap-2 pb-2">
        <CardTitle className="text-sm font-medium text-muted-foreground">
          {t("stats:usageOverTime")}
        </CardTitle>
        <div className="flex flex-wrap items-center justify-end gap-2">
          <Tabs value={metric} onValueChange={(value) => setMetric(value as "cost" | "tokens")}>
            <TabsList className="h-11 md:h-7" variant="line">
              <TabsTrigger value="cost" className="h-11 cursor-pointer px-3 text-xs md:h-6 md:px-2">
                {t("stats:cost")}
              </TabsTrigger>
              <TabsTrigger
                value="tokens"
                className="h-11 cursor-pointer px-3 text-xs md:h-6 md:px-2"
              >
                {t("stats:tokens")}
              </TabsTrigger>
            </TabsList>
          </Tabs>
          <Tabs
            value={granularity}
            onValueChange={(value) => setGranularity(value as TokenUsageTrendView)}
          >
            <TabsList className="h-11 md:h-7" variant="line">
              <TabsTrigger
                value="daily"
                className="h-11 cursor-pointer px-3 text-xs md:h-6 md:px-2"
              >
                {t("stats:daily")}
              </TabsTrigger>
              <TabsTrigger
                value="weekly"
                className="h-11 cursor-pointer px-3 text-xs md:h-6 md:px-2"
              >
                {t("stats:weekly")}
              </TabsTrigger>
              <TabsTrigger
                value="monthly"
                className="h-11 cursor-pointer px-3 text-xs md:h-6 md:px-2"
              >
                {t("stats:monthly")}
              </TabsTrigger>
            </TabsList>
          </Tabs>
        </div>
      </CardHeader>
      <CardContent>
        {points.length === 0 ? (
          <p className="py-8 text-sm text-muted-foreground">
            {datedCoverage ? t("stats:noTokenUsageInRange") : t("stats:undatedCoverageOnly")}
          </p>
        ) : (
          <TrendBars
            points={points}
            values={values}
            maxValue={maxValue}
            metric={metric}
            currency={currency}
            unavailable={unavailable}
            granularity={granularity}
            label={t("stats:usageOverTime")}
          />
        )}
      </CardContent>
    </Card>
  );
}

type TrendBarsProps = {
  points: TokenUsageAggregate[];
  values: Array<number | null>;
  maxValue: number;
  metric: "cost" | "tokens";
  currency: string;
  unavailable: string;
  granularity: TokenUsageTrendView;
  label: string;
};

function TrendBars({
  points,
  values,
  maxValue,
  metric,
  currency,
  unavailable,
  granularity,
  label,
}: TrendBarsProps) {
  return (
    <div className="flex h-36 items-end gap-1.5" role="img" aria-label={label}>
      {points.map((point, index) => {
        const value = values[index];
        const height = value === null ? 0 : Math.max(6, Math.round((value / maxValue) * 100));
        const formattedValue =
          metric === "cost"
            ? formatCost(
                point.currency === "mixed" ? null : point.cost_subcents,
                point.currency || currency,
                unavailable,
              )
            : formatTokens(point.total_tokens, unavailable);
        return (
          <div
            key={`${point.period}-${index}`}
            className="flex min-w-0 flex-1 flex-col items-center gap-1"
          >
            <span className="max-w-full truncate text-[10px] text-muted-foreground">
              {formattedValue}
            </span>
            <div className="flex h-24 w-full items-end">
              <div
                className="w-full rounded-t-sm bg-primary/70"
                style={{ height: `${height}%` }}
                title={formattedValue}
              />
            </div>
            <span className="max-w-full truncate text-[10px] text-muted-foreground">
              {formatTokenPeriod(point.period, granularity === "monthly" ? "monthly" : "daily")}
            </span>
          </div>
        );
      })}
    </div>
  );
}

type TokenUsageBreakdownProps = {
  response: TokenUsageResponse;
  view: TokenUsageView;
  onViewChange: (view: TokenUsageView) => void;
  provider: string;
  onProviderChange: (provider: string) => void;
  providers: string[];
  onSort: (key: TokenUsageSortKey, direction?: SortDirection) => void;
  sortKey: TokenUsageSortKey;
  sortDirection: SortDirection;
  offset: number;
  limit: number;
  onPageChange: (offset: number) => void;
};

export function TokenUsageBreakdown({
  response,
  view,
  onViewChange,
  provider,
  onProviderChange,
  providers,
  onSort,
  sortKey,
  sortDirection,
  offset,
  limit,
  onPageChange,
}: TokenUsageBreakdownProps) {
  const { t } = useTranslation();
  const unavailable = t("common:unavailable");
  const rows = useMemo(() => aggregateTokenRows(response.rows, view), [response.rows, view]);
  const viewLabels = viewLabelMap(t);

  return (
    <Card className="rounded-sm" data-testid="token-usage-breakdown">
      <CardHeader className="gap-3 pb-2">
        <BreakdownControls
          view={view}
          onViewChange={onViewChange}
          provider={provider}
          onProviderChange={onProviderChange}
          providers={providers}
          viewLabels={viewLabels}
        />
      </CardHeader>
      <CardContent className="pt-0">
        {rows.length === 0 ? (
          <p className="py-8 text-sm text-muted-foreground">{t("stats:noTokenUsageInRange")}</p>
        ) : (
          <TokenUsageTables
            rows={rows}
            totals={response.totals}
            view={view}
            unavailable={unavailable}
            onSort={onSort}
            sortKey={sortKey}
            sortDirection={sortDirection}
            viewLabels={viewLabels}
            hasMore={response.has_more}
            totalRows={response.total_rows}
            offset={offset}
            limit={limit}
            onPageChange={onPageChange}
          />
        )}
      </CardContent>
    </Card>
  );
}

function viewLabelMap(t: (key: string) => string): Record<TokenUsageView, string> {
  return {
    models: t("stats:models"),
    daily: t("stats:daily"),
    monthly: t("stats:monthly"),
    tasks: t("stats:tasks"),
    sessions: t("stats:sessions"),
  };
}

function BreakdownControls({
  view,
  onViewChange,
  provider,
  onProviderChange,
  providers,
  viewLabels,
}: {
  view: TokenUsageView;
  onViewChange: (view: TokenUsageView) => void;
  provider: string;
  onProviderChange: (provider: string) => void;
  providers: string[];
  viewLabels: Record<TokenUsageView, string>;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-wrap items-center justify-between gap-2">
      <CardTitle className="text-sm font-medium text-muted-foreground">
        {t("stats:tokenUsageBreakdown")}
      </CardTitle>
      <div className="flex items-center gap-2">
        <ViewSelector view={view} onChange={onViewChange} viewLabels={viewLabels} />
        <ProviderSelector provider={provider} onChange={onProviderChange} providers={providers} />
      </div>
    </div>
  );
}

function ViewSelector({
  view,
  onChange,
  viewLabels,
}: {
  view: TokenUsageView;
  onChange: (view: TokenUsageView) => void;
  viewLabels: Record<TokenUsageView, string>;
}) {
  const { t } = useTranslation();
  return (
    <>
      <div className="hidden md:block">
        <Tabs value={view} onValueChange={(value) => onChange(value as TokenUsageView)}>
          <TabsList className="h-7" variant="line">
            {VIEW_KEYS.map((key) => (
              <TabsTrigger key={key} value={key} className="h-6 cursor-pointer px-2 text-xs">
                {viewLabels[key]}
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>
      </div>
      <div className="md:hidden">
        <Select value={view} onValueChange={(value) => onChange(value as TokenUsageView)}>
          <SelectTrigger className="min-h-11 w-32" aria-label={t("stats:view")}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {VIEW_KEYS.map((key) => (
              <SelectItem key={key} value={key}>
                {viewLabels[key]}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
    </>
  );
}

function ProviderSelector({
  provider,
  onChange,
  providers,
}: {
  provider: string;
  onChange: (provider: string) => void;
  providers: string[];
}) {
  const { t } = useTranslation();
  return (
    <Select value={provider} onValueChange={onChange}>
      <SelectTrigger className="min-h-11 w-32 md:min-h-0" aria-label={t("stats:provider")}>
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value="all">{t("stats:allProviders")}</SelectItem>
        {providers.map((value) => (
          <SelectItem key={value} value={value}>
            {value === UNKNOWN_PROVIDER ? t(UNKNOWN_LABEL_KEY) : value}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

function coverageSummary(
  datedCoverage: boolean,
  undatedCoverage: boolean,
  t: (key: string) => string,
): string {
  if (datedCoverage && undatedCoverage) return t("stats:datedAndUndatedCoverage");
  if (datedCoverage) return t("stats:datedCoverageAvailable");
  if (undatedCoverage) return t("stats:undatedCoverageOnly");
  return t("stats:coverageUnavailable");
}
