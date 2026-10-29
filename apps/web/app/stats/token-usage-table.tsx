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
import { usageAmount } from "@/lib/api/domains/token-usage-amounts";
import { UsageDetails, UsageDetailsDisclosure, UsageRowIdentity } from "./token-usage-details";
import { usageProviderLabel, usageRowLabel } from "./token-usage-metadata";
import {
  coverageLabelKey,
  costBasisLabel,
  formatCost,
  formatRate,
  formatTokens,
  type SortDirection,
  type TokenMetricKey,
  type TokenUsageAggregate,
  type TokenUsageSortKey,
  type TokenUsageView,
} from "./token-usage-utils";

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
        {t("stats:usageSort")}
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
        className="h-11 w-11 cursor-pointer md:h-7 md:w-7 [@media(pointer:coarse)]:h-11 [@media(pointer:coarse)]:w-11"
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
        className="h-11 w-11 cursor-pointer md:h-7 md:w-7 [@media(pointer:coarse)]:h-11 [@media(pointer:coarse)]:w-11"
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
        className="inline-flex min-h-7 cursor-pointer items-center gap-1 [@media(pointer:coarse)]:min-h-11"
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
        <div
          className="flex max-w-48 items-center gap-2 font-medium"
          title={usageRowLabel(row, view, t)}
        >
          <UsageRowIdentity row={row} view={view} />
          {view === "monthly" && row.coverage === "partial" && (
            <CoverageBadge coverage={row.coverage} />
          )}
        </div>
        {row.secondary && (
          <div className="max-w-48 truncate text-[11px] text-muted-foreground">{row.secondary}</div>
        )}
        <UsageDetailsDisclosure row={row} view={view} />
      </TableCell>
      <TableCell className="text-muted-foreground">{usageProviderLabel(row.provider, t)}</TableCell>
      <MetricCells metrics={row} unavailable={unavailable} />
      <TableCell className="text-right tabular-nums">
        {formatRate(row.effective_cost_per_million, row.currency, unavailable)}
      </TableCell>
      <TableCell>
        <CoverageBadge coverage={row.stale ? "stale" : row.coverage} />
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
        {formatTokens(usageAmount(metrics, "input_tokens"), unavailable)}
      </TableCell>
      <TableCell className="text-right tabular-nums">
        {formatTokens(usageAmount(metrics, "output_tokens"), unavailable)}
      </TableCell>
      <TableCell className="text-right tabular-nums">
        {formatTokens(usageAmount(metrics, "cache_read_tokens"), unavailable)}
      </TableCell>
      <TableCell className="text-right tabular-nums">
        {formatTokens(usageAmount(metrics, "cache_write_tokens"), unavailable)}
      </TableCell>
      <TableCell className="text-right tabular-nums">
        {formatTokens(usageAmount(metrics, "reasoning_tokens"), unavailable)}
      </TableCell>
      <TableCell className="text-right tabular-nums">
        {formatTokens(usageAmount(metrics, "total_tokens"), unavailable)}
      </TableCell>
      <TableCell className="text-right tabular-nums">
        {formatCost(usageAmount(metrics, "cost_subcents"), metrics.currency, unavailable)}
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
    <details className="min-w-0 rounded-sm border" data-testid="token-usage-mobile-row">
      <summary
        className="flex min-h-11 cursor-pointer list-none flex-wrap items-center justify-between gap-3 p-3 [&::-webkit-details-marker]:hidden"
        aria-label={t("stats:usageDetailsFor", { label: usageRowLabel(row, view, t) })}
      >
        <span className="min-w-0 flex-1">
          <span className="block text-sm font-medium">
            <UsageRowIdentity row={row} view={view} link={false} />
          </span>
          {row.secondary && (
            <span className="block truncate text-xs text-muted-foreground">{row.secondary}</span>
          )}
          <span className="block text-xs text-muted-foreground">{t("stats:usageDetails")}</span>
        </span>
        <span className="ml-auto min-w-0 max-w-full text-right text-xs tabular-nums [overflow-wrap:anywhere]">
          <span className="block font-medium">
            {formatTokens(usageAmount(row, "total_tokens"), unavailable)}
          </span>
          <span className="block text-muted-foreground">
            {formatCost(usageAmount(row, "cost_subcents"), row.currency, unavailable)}
          </span>
          <span className="block text-muted-foreground">{costBasisLabel(row.cost_basis, t)}</span>
        </span>
      </summary>
      <div className="min-w-0 border-t p-3">
        <UsageDetails row={row} view={view} showLink />
      </div>
    </details>
  );
}

function MobileTotals({ row, unavailable }: { row: TokenUsageMetrics; unavailable: string }) {
  const { t } = useTranslation();
  return (
    <div className="flex items-center justify-between gap-3 border-t px-1 pt-3 text-sm">
      <span className="font-medium">{t("stats:allFilteredRecords")}</span>
      <span className="text-right tabular-nums">
        <span className="block font-medium">
          {formatTokens(usageAmount(row, "total_tokens"), unavailable)}
        </span>
        <span className="text-xs text-muted-foreground">
          {formatCost(usageAmount(row, "cost_subcents"), row.currency, unavailable)}
        </span>
      </span>
    </div>
  );
}

function CoverageBadge({ coverage }: { coverage: string }) {
  const { t } = useTranslation();
  return <Badge variant="outline">{t(coverageLabelKey(coverage))}</Badge>;
}
