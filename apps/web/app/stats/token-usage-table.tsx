"use client";

import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import {
  Table,
  TableBody,
  TableCell,
  TableFooter,
  TableHead,
  TableHeader,
  TableRow,
} from "@kandev/ui/table";
import {
  IconChevronDown,
  IconChevronLeft,
  IconChevronRight,
  IconChevronUp,
} from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import type { TokenUsageMetrics } from "@/lib/types/http";
import {
  coverageLabelKey,
  costBasisLabel,
  formatCost,
  formatRate,
  formatTokenPeriod,
  formatTokens,
  type SortDirection,
  type TokenMetricKey,
  type TokenUsageAggregate,
  type TokenUsageSortKey,
  type TokenUsageView,
} from "./token-usage-utils";

const UNKNOWN_LABEL_KEY = "common:unknown";
const METRIC_LABEL_KEYS: Record<TokenMetricKey, string> = {
  input_tokens: "stats:inputTokens",
  output_tokens: "stats:outputTokens",
  cache_read_tokens: "stats:cacheReadTokens",
  cache_write_tokens: "stats:cacheWriteTokens",
  reasoning_tokens: "stats:reasoningTokens",
  total_tokens: "stats:totalTokens",
  turns: "stats:totalTurns",
  cost_subcents: "stats:cost",
};

export type TokenUsageTablesProps = {
  rows: TokenUsageAggregate[];
  totals: TokenUsageMetrics;
  view: TokenUsageView;
  unavailable: string;
  onSort: (key: TokenUsageSortKey, direction?: SortDirection) => void;
  sortKey: TokenUsageSortKey;
  sortDirection: SortDirection;
  viewLabels: Record<TokenUsageView, string>;
  hasMore: boolean;
  totalRows: number;
  offset: number;
  limit: number;
  onPageChange: (offset: number) => void;
};

export function TokenUsageTables({
  rows,
  totals,
  view,
  unavailable,
  onSort,
  sortKey,
  sortDirection,
  viewLabels,
  hasMore,
  totalRows,
  offset,
  limit,
  onPageChange,
}: TokenUsageTablesProps) {
  return (
    <>
      <div className="hidden md:block">
        <DesktopUsageTable
          rows={rows}
          totals={totals}
          view={view}
          unavailable={unavailable}
          onSort={onSort}
          sortKey={sortKey}
          sortDirection={sortDirection}
          viewLabels={viewLabels}
        />
      </div>
      <div className="space-y-2 md:hidden">
        <MobileSortControl
          view={view}
          sortKey={sortKey}
          sortDirection={sortDirection}
          onSort={onSort}
        />
        {rows.map((row) => (
          <MobileUsageRow key={row.key} row={row} view={view} unavailable={unavailable} />
        ))}
        <MobileTotals row={totals} unavailable={unavailable} />
      </div>
      <UsagePagination
        hasMore={hasMore}
        totalRows={totalRows}
        offset={offset}
        limit={limit}
        onPageChange={onPageChange}
      />
    </>
  );
}

function MobileSortControl({
  view,
  sortKey,
  sortDirection,
  onSort,
}: {
  view: TokenUsageView;
  sortKey: TokenUsageSortKey;
  sortDirection: SortDirection;
  onSort: (key: TokenUsageSortKey, direction?: SortDirection) => void;
}) {
  const { t } = useTranslation();
  const viewLabel = t(`stats:${view}`);
  const options: Array<{ key: TokenUsageSortKey; label: string }> = [
    {
      key: view === "daily" || view === "monthly" ? "period" : "label",
      label: viewLabel,
    },
    { key: "total_tokens", label: t("stats:totalTokens") },
    { key: "cost_subcents", label: t("stats:cost") },
    { key: "effective_cost_per_million", label: t("stats:effectiveRate") },
  ];
  const value = `${sortKey}:${sortDirection}`;
  return (
    <div className="flex items-center justify-end gap-2">
      <label htmlFor="token-usage-mobile-sort" className="text-xs text-muted-foreground">
        {t("stats:view")}
      </label>
      <select
        id="token-usage-mobile-sort"
        className="h-11 rounded-sm border bg-background px-2 text-xs md:h-7"
        value={value}
        onChange={(event) => {
          const [key, direction] = event.target.value.split(":") as [
            TokenUsageSortKey,
            SortDirection,
          ];
          onSort(key, direction);
        }}
      >
        {options.map((option) => (
          <option key={option.key} value={`${option.key}:desc`}>
            {option.label} ↓
          </option>
        ))}
        {options.map((option) => (
          <option key={`${option.key}:asc`} value={`${option.key}:asc`}>
            {option.label} ↑
          </option>
        ))}
      </select>
    </div>
  );
}

function UsagePagination({
  hasMore,
  totalRows,
  offset,
  limit,
  onPageChange,
}: Pick<TokenUsageTablesProps, "hasMore" | "totalRows" | "offset" | "limit" | "onPageChange">) {
  const { t } = useTranslation();
  const first = totalRows === 0 ? 0 : offset + 1;
  const last = Math.min(offset + limit, totalRows);
  return (
    <div className="flex items-center justify-end gap-2 text-xs text-muted-foreground">
      <span aria-live="polite">
        {t("stats:tokenUsagePageRange", { first, last, total: totalRows })}
      </span>
      <Button
        type="button"
        variant="outline"
        size="icon"
        aria-label={t("stats:previousPage")}
        onClick={() => onPageChange(Math.max(0, offset - limit))}
        disabled={offset <= 0}
      >
        <IconChevronLeft />
      </Button>
      <Button
        type="button"
        variant="outline"
        size="icon"
        aria-label={t("stats:nextPage")}
        onClick={() => onPageChange(offset + limit)}
        disabled={!hasMore}
      >
        <IconChevronRight />
      </Button>
    </div>
  );
}

function DesktopUsageTable({
  rows,
  totals,
  view,
  unavailable,
  onSort,
  sortKey,
  sortDirection,
  viewLabels,
}: Omit<TokenUsageTablesProps, "hasMore" | "totalRows" | "offset" | "limit" | "onPageChange">) {
  const { t } = useTranslation();
  return (
    <Table data-testid="token-usage-table">
      <TableHeader>
        <TableRow>
          <SortableHead
            label={viewLabels[view]}
            sortKey={view === "daily" || view === "monthly" ? "period" : "label"}
            activeKey={sortKey}
            direction={sortDirection}
            onSort={onSort}
          />
          <TableHead>{t("stats:provider")}</TableHead>
          {(
            [
              "input_tokens",
              "output_tokens",
              "cache_read_tokens",
              "cache_write_tokens",
              "reasoning_tokens",
              "total_tokens",
              "cost_subcents",
            ] as TokenMetricKey[]
          ).map((key) => (
            <SortableHead
              key={key}
              label={t(METRIC_LABEL_KEYS[key])}
              sortKey={key}
              activeKey={sortKey}
              direction={sortDirection}
              onSort={onSort}
              numeric
            />
          ))}
          <SortableHead
            label={t("stats:effectiveRate")}
            sortKey="effective_cost_per_million"
            activeKey={sortKey}
            direction={sortDirection}
            onSort={onSort}
            numeric
          />
          <TableHead>{t("stats:coverage")}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {rows.map((row) => (
          <DesktopUsageRow key={row.key} row={row} view={view} unavailable={unavailable} />
        ))}
      </TableBody>
      <TableFooter>
        <TableRow data-testid="token-usage-total-row">
          <TableCell className="font-medium">{t("stats:allFilteredRecords")}</TableCell>
          <TableCell />
          <MetricCells metrics={totals} unavailable={unavailable} />
          <TableCell className="text-right font-medium">
            {formatRate(totals.effective_cost_per_million, totals.currency, unavailable)}
          </TableCell>
          <TableCell>{costBasisLabel(totals.cost_basis, t)}</TableCell>
        </TableRow>
      </TableFooter>
    </Table>
  );
}

function SortableHead({
  label,
  sortKey,
  activeKey,
  direction,
  onSort,
  numeric = false,
}: {
  label: string;
  sortKey: TokenUsageSortKey;
  activeKey: TokenUsageSortKey;
  direction: SortDirection;
  onSort: (key: TokenUsageSortKey, direction?: SortDirection) => void;
  numeric?: boolean;
}) {
  const active = activeKey === sortKey;
  return (
    <TableHead
      className={numeric ? "text-right" : undefined}
      aria-sort={active ? `${direction}ending` : "none"}
    >
      <button
        type="button"
        className="inline-flex cursor-pointer items-center gap-1"
        onClick={() => onSort(sortKey)}
      >
        {label}
        {active &&
          (direction === "asc" ? (
            <IconChevronUp className="size-3" />
          ) : (
            <IconChevronDown className="size-3" />
          ))}
      </button>
    </TableHead>
  );
}

function DesktopUsageRow({
  row,
  view,
  unavailable,
}: {
  row: TokenUsageAggregate;
  view: TokenUsageView;
  unavailable: string;
}) {
  const { t } = useTranslation();
  return (
    <TableRow data-testid="token-usage-row">
      <TableCell>
        <div className="flex max-w-48 items-center gap-2 truncate font-medium" title={row.label}>
          <span className="truncate">
            {view === "daily" || view === "monthly"
              ? periodLabel(row.label, view, t)
              : row.label || t(UNKNOWN_LABEL_KEY)}
          </span>
          {view === "monthly" && row.coverage === "partial" && (
            <CoverageBadge coverage={row.coverage} />
          )}
        </div>
        {row.secondary && (
          <div className="max-w-48 truncate text-[11px] text-muted-foreground">{row.secondary}</div>
        )}
      </TableCell>
      <TableCell className="text-muted-foreground">
        {row.provider || t(UNKNOWN_LABEL_KEY)}
      </TableCell>
      <MetricCells metrics={row} unavailable={unavailable} />
      <TableCell className="text-right tabular-nums">
        {formatRate(row.effective_cost_per_million, row.currency, unavailable)}
      </TableCell>
      <TableCell>
        <CoverageBadge coverage={row.coverage} />
      </TableCell>
    </TableRow>
  );
}

function MetricCells({
  metrics,
  unavailable,
}: {
  metrics: TokenUsageMetrics;
  unavailable: string;
}) {
  return (
    <>
      <TableCell className="text-right tabular-nums">
        {formatTokens(metrics.input_tokens, unavailable)}
      </TableCell>
      <TableCell className="text-right tabular-nums">
        {formatTokens(metrics.output_tokens, unavailable)}
      </TableCell>
      <TableCell className="text-right tabular-nums">
        {formatTokens(metrics.cache_read_tokens, unavailable)}
      </TableCell>
      <TableCell className="text-right tabular-nums">
        {formatTokens(metrics.cache_write_tokens, unavailable)}
      </TableCell>
      <TableCell className="text-right tabular-nums">
        {formatTokens(metrics.reasoning_tokens, unavailable)}
      </TableCell>
      <TableCell className="text-right tabular-nums">
        {formatTokens(metrics.total_tokens, unavailable)}
      </TableCell>
      <TableCell className="text-right tabular-nums">
        {formatCost(metrics.cost_subcents, metrics.currency, unavailable)}
      </TableCell>
    </>
  );
}

function MobileUsageRow({
  row,
  view,
  unavailable,
}: {
  row: TokenUsageAggregate;
  view: TokenUsageView;
  unavailable: string;
}) {
  const { t } = useTranslation();
  return (
    <details className="rounded-sm border" data-testid="token-usage-mobile-row">
      <summary className="flex min-h-11 cursor-pointer list-none items-center justify-between gap-3 p-3 [&::-webkit-details-marker]:hidden">
        <span className="min-w-0">
          <span className="flex truncate text-sm font-medium">
            <span className="truncate">
              {view === "daily" || view === "monthly"
                ? periodLabel(row.label, view, t)
                : row.label || t(UNKNOWN_LABEL_KEY)}
            </span>
            {view === "monthly" && row.coverage === "partial" && (
              <span className="ml-2 shrink-0">
                <CoverageBadge coverage={row.coverage} />
              </span>
            )}
          </span>
          {row.secondary && (
            <span className="block truncate text-xs text-muted-foreground">{row.secondary}</span>
          )}
        </span>
        <span className="shrink-0 text-right text-xs tabular-nums">
          <span className="block font-medium">{formatTokens(row.total_tokens, unavailable)}</span>
          <span className="text-muted-foreground">
            {formatCost(row.cost_subcents, row.currency, unavailable)}
          </span>
        </span>
      </summary>
      <div className="grid grid-cols-2 gap-3 border-t p-3">
        <MobileMetric
          label={t("stats:inputTokens")}
          value={formatTokens(row.input_tokens, unavailable)}
        />
        <MobileMetric
          label={t("stats:outputTokens")}
          value={formatTokens(row.output_tokens, unavailable)}
        />
        <MobileMetric
          label={t("stats:cacheReadTokens")}
          value={formatTokens(row.cache_read_tokens, unavailable)}
        />
        <MobileMetric
          label={t("stats:cacheWriteTokens")}
          value={formatTokens(row.cache_write_tokens, unavailable)}
        />
        <MobileMetric
          label={t("stats:reasoningTokens")}
          value={formatTokens(row.reasoning_tokens, unavailable)}
        />
        <MobileMetric
          label={t("stats:effectiveRate")}
          value={formatRate(row.effective_cost_per_million, row.currency, unavailable)}
        />
        <div className="col-span-2 flex items-center justify-between gap-2 text-xs">
          <span className="text-muted-foreground">{t("stats:coverage")}</span>
          <CoverageBadge coverage={row.coverage} />
        </div>
      </div>
    </details>
  );
}

function MobileMetric({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <div className="text-xs text-muted-foreground">{label}</div>
      <div className="text-sm tabular-nums">{value}</div>
    </div>
  );
}

function MobileTotals({ row, unavailable }: { row: TokenUsageMetrics; unavailable: string }) {
  const { t } = useTranslation();
  return (
    <div className="flex items-center justify-between gap-3 border-t px-1 pt-3 text-sm">
      <span className="font-medium">{t("stats:allFilteredRecords")}</span>
      <span className="text-right tabular-nums">
        <span className="block font-medium">{formatTokens(row.total_tokens, unavailable)}</span>
        <span className="text-xs text-muted-foreground">
          {formatCost(row.cost_subcents, row.currency, unavailable)}
        </span>
      </span>
    </div>
  );
}

function CoverageBadge({ coverage }: { coverage: string }) {
  const { t } = useTranslation();
  return <Badge variant="outline">{t(coverageLabelKey(coverage))}</Badge>;
}

function periodLabel(value: string, view: TokenUsageView, t: (key: string) => string): string {
  return value ? formatTokenPeriod(value, view) : t("stats:undatedCoverageOnly");
}
