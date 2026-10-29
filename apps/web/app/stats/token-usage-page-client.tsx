"use client";

import { useCallback, useMemo, useState, type ReactNode } from "react";
import { Button } from "@kandev/ui/button";
import { PageShell } from "@/components/page-shell";
import { IconChartBar } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { useRouter, useSearchParams } from "@/lib/routing/client-router";
import { usePlugins } from "@/hooks/domains/plugins/use-plugins";
import { useCopyToClipboard } from "@/hooks/use-copy-to-clipboard";
import { browserTimeZone } from "@/components/automations/schedule-expression";
import type { TokenUsageResponse } from "@/lib/types/http";
import type { RangeKey } from "./stats-utils";
import { DEFAULT_RANGE, isRangeKey } from "./stats-utils";
import { RANGE_LABEL_KEYS, StatsNavigation, StatsRangeToggle } from "./stats-navigation";
import { isUsagePreparing, useTokenUsage, type TokenUsageLoadOptions } from "./token-usage-data";
import { buildTokenUsageCopySummary } from "./token-usage-copy";
import { TokenUsageBreakdown, TokenUsageSummary, TokenUsageTrend } from "./token-usage-sections";
import {
  TokenUsageCoverageNotice,
  TokenUsageEmpty,
  TokenUsageError,
  TokenUsageLoading,
  TokenUsagePreparing,
  TokenUsageProjectionNotice,
} from "./token-usage-states";
import {
  type SortDirection,
  type TokenUsageSortKey,
  type TokenUsageView,
} from "./token-usage-utils";

const TOKEN_USAGE_PAGE_SIZE = 200;
const TOKEN_USAGE_SORT_ALIASES: Partial<Record<TokenUsageSortKey, string>> = {
  input_tokens: "input",
  output_tokens: "output",
  cache_read_tokens: "cache_read",
  cache_write_tokens: "cache_write",
  reasoning_tokens: "reasoning",
};

type TokenUsagePageClientProps = {
  workspaceId?: string;
  activeRange?: RangeKey;
  initialError?: string | null;
};

type TokenUsageRequestState = ReturnType<typeof useTokenUsage>;

export function TokenUsagePageClient({
  workspaceId,
  activeRange,
  initialError,
}: TokenUsagePageClientProps) {
  const { t } = useTranslation();
  const router = useRouter();
  const searchParams = useSearchParams();
  const { copied, copy } = useCopyToClipboard();
  const [retryKey, setRetryKey] = useState(0);
  const {
    breakdownView,
    provider,
    sortKey,
    sortDirection,
    offset,
    handleViewChange,
    handleProviderChange,
    handleSort,
    setOffset,
  } = useTokenUsageControls();
  const timezone = useMemo(() => browserTimeZone(), []);
  const rawRange = searchParams.get("range") ?? activeRange;
  const range: RangeKey = isRangeKey(rawRange) ? rawRange : DEFAULT_RANGE;
  const { state, trendState } = useTokenUsagePageRequests({
    workspaceId,
    range,
    retryKey,
    breakdownView,
    provider,
    sortKey,
    sortDirection,
    offset,
    timezone,
  });
  const rangeLabel = t(RANGE_LABEL_KEYS[range]);
  const copyText = useMemo(() => {
    if (state.status !== "ready" || isUsagePreparing(state.data)) return "";
    return buildTokenUsageCopySummary(state.data, rangeLabel, breakdownView, provider, t);
  }, [breakdownView, provider, rangeLabel, state, t]);

  const handleRangeChange = useCallback(
    (nextRange: RangeKey) => {
      const params = new URLSearchParams(searchParams.toString());
      params.set("range", nextRange);
      setOffset(0);
      router.replace(`/stats/token-usage?${params.toString()}`, { scroll: false });
    },
    [router, searchParams, setOffset],
  );

  const handleCopy = useCallback(() => {
    if (copyText) void copy(copyText);
  }, [copy, copyText]);

  return (
    <TokenUsagePageView
      workspaceId={workspaceId}
      initialError={initialError}
      range={range}
      state={state}
      trendState={trendState}
      breakdownView={breakdownView}
      provider={provider}
      sortKey={sortKey}
      sortDirection={sortDirection}
      offset={offset}
      copyText={copyText}
      copied={copied}
      onRetry={() => setRetryKey((key) => key + 1)}
      onRangeChange={handleRangeChange}
      onCopy={handleCopy}
      onViewChange={handleViewChange}
      onProviderChange={handleProviderChange}
      onSort={handleSort}
      onPageChange={setOffset}
    />
  );
}

function useTokenUsageControls() {
  const [breakdownView, setBreakdownView] = useState<TokenUsageView>("models");
  const [provider, setProvider] = useState("all");
  const [sortKey, setSortKey] = useState<TokenUsageSortKey>("cost_subcents");
  const [sortDirection, setSortDirection] = useState<SortDirection>("desc");
  const [offset, setOffset] = useState(0);

  const handleViewChange = useCallback((nextView: TokenUsageView) => {
    setBreakdownView(nextView);
    setOffset(0);
    if (nextView === "daily" || nextView === "monthly") {
      setSortKey("period");
      setSortDirection("desc");
    } else {
      setSortKey("cost_subcents");
      setSortDirection("desc");
    }
  }, []);

  const handleProviderChange = useCallback((nextProvider: string) => {
    setProvider(nextProvider);
    setOffset(0);
  }, []);

  const handleSort = useCallback(
    (nextKey: TokenUsageSortKey, requestedDirection?: SortDirection) => {
      if (requestedDirection) {
        setSortKey(nextKey);
        setSortDirection(requestedDirection);
        setOffset(0);
        return;
      }
      if (sortKey === nextKey) {
        setSortDirection((direction) => (direction === "asc" ? "desc" : "asc"));
      } else {
        setSortKey(nextKey);
        setSortDirection(nextKey === "label" || nextKey === "period" ? "asc" : "desc");
      }
      setOffset(0);
    },
    [sortKey],
  );

  return {
    breakdownView,
    provider,
    sortKey,
    sortDirection,
    offset,
    handleViewChange,
    handleProviderChange,
    handleSort,
    setOffset,
  };
}

function useTokenUsagePageRequests({
  workspaceId,
  range,
  retryKey,
  breakdownView,
  provider,
  sortKey,
  sortDirection,
  offset,
  timezone,
}: {
  workspaceId?: string;
  range: RangeKey;
  retryKey: number;
  breakdownView: TokenUsageView;
  provider: string;
  sortKey: TokenUsageSortKey;
  sortDirection: SortDirection;
  offset: number;
  timezone: string;
}): { state: TokenUsageRequestState; trendState: TokenUsageRequestState } {
  const breakdownQuery = useMemo<TokenUsageLoadOptions>(
    () => ({
      provider: provider === "all" ? undefined : provider,
      timezone,
      sortBy: apiSortKey(breakdownView, sortKey),
      sortDirection,
      limit: TOKEN_USAGE_PAGE_SIZE,
      offset,
    }),
    [breakdownView, offset, provider, sortDirection, sortKey, timezone],
  );
  const state = useTokenUsage(
    workspaceId,
    range,
    retryKey,
    usageGroupForView(breakdownView),
    breakdownQuery,
  );
  // The breakdown endpoint is grouped for the selected table. Keep a separate
  // daily query for the chart so switching to Models or Tasks does not discard
  // the dated points needed by the trend.
  const trendQuery = useMemo<TokenUsageLoadOptions>(
    () => ({
      provider: provider === "all" ? undefined : provider,
      timezone,
      sortBy: "period",
      sortDirection: "asc",
      limit: 1000,
      offset: 0,
      allPages: true,
    }),
    [provider, timezone],
  );
  const trendState = useTokenUsage(workspaceId, range, retryKey, "day", trendQuery);
  return { state, trendState };
}

type TokenUsagePageViewProps = {
  workspaceId?: string;
  initialError?: string | null;
  range: RangeKey;
  state: TokenUsageRequestState;
  trendState: TokenUsageRequestState;
  breakdownView: TokenUsageView;
  provider: string;
  sortKey: TokenUsageSortKey;
  sortDirection: SortDirection;
  offset: number;
  copyText: string;
  copied: boolean;
  onRetry: () => void;
  onRangeChange: (range: RangeKey) => void;
  onCopy: () => void;
  onViewChange: (view: TokenUsageView) => void;
  onProviderChange: (provider: string) => void;
  onSort: (key: TokenUsageSortKey, direction?: SortDirection) => void;
  onPageChange: (offset: number) => void;
};

function TokenUsagePageView({
  workspaceId,
  initialError,
  range,
  state,
  trendState,
  breakdownView,
  provider,
  sortKey,
  sortDirection,
  offset,
  copyText,
  copied,
  onRetry,
  onRangeChange,
  onCopy,
  onViewChange,
  onProviderChange,
  onSort,
  onPageChange,
}: TokenUsagePageViewProps) {
  const { t } = useTranslation();
  return (
    <PageShell
      title={t("stats:statistics")}
      icon={<IconChartBar className="h-4 w-4" />}
      subtitle={t("stats:tokenUsageSubtitle")}
      scroll="none"
      topbarTestId="token-usage-topbar"
      freeWidth="actions"
      actionsClassName="min-w-0 max-w-full flex-1 !shrink overflow-x-auto"
      actions={
        <div
          className="flex w-full min-w-0 max-w-full items-center gap-2 overflow-x-auto"
          data-testid="token-usage-topbar-actions"
        >
          <StatsNavigation />
          <StatsRangeToggle range={range} onChange={onRangeChange} />
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="h-11 cursor-pointer px-2 text-xs md:h-7"
            onClick={onCopy}
            disabled={!copyText}
          >
            {copied ? t("stats:copied") : t("stats:copyStats")}
          </Button>
        </div>
      }
    >
      <div
        className="min-h-0 flex-1 overflow-y-auto bg-background"
        data-testid="token-usage-scroll-body"
      >
        <div className="mx-auto min-w-0 max-w-7xl space-y-4 p-4 pb-[max(1rem,env(safe-area-inset-bottom))] md:p-6">
          <TokenUsagePageState
            workspaceId={workspaceId}
            initialError={initialError}
            state={state}
            trendState={trendState}
            breakdownView={breakdownView}
            provider={provider}
            sortKey={sortKey}
            sortDirection={sortDirection}
            offset={offset}
            onRetry={onRetry}
            onRangeChange={onRangeChange}
            onViewChange={onViewChange}
            onProviderChange={onProviderChange}
            onSort={onSort}
            onPageChange={onPageChange}
          />
        </div>
      </div>
    </PageShell>
  );
}

function TokenUsagePageState({
  workspaceId,
  initialError,
  state,
  trendState,
  breakdownView,
  provider,
  sortKey,
  sortDirection,
  offset,
  onRetry,
  onRangeChange,
  onViewChange,
  onProviderChange,
  onSort,
  onPageChange,
}: Omit<TokenUsagePageViewProps, "range" | "copyText" | "copied" | "onCopy">) {
  const { items: plugins } = usePlugins();
  const sessionCost = plugins.find(
    (plugin) => plugin.id === "kandev-session-cost" && plugin.status !== "uninstalled",
  );
  const pluginSettingsHref = sessionCost
    ? `/settings/plugins/${encodeURIComponent(sessionCost.id)}`
    : "/settings/plugins";
  if (initialError) return <TokenUsageError message={initialError} onRetry={onRetry} />;
  if (!workspaceId || state.status === "no-workspace") {
    return (
      <TokenUsageEmpty
        hasHistory={false}
        onAllTime={() => undefined}
        pluginSettingsHref={pluginSettingsHref}
      />
    );
  }
  if (state.status === "loading") return <TokenUsageLoading />;
  if (state.status === "error")
    return <TokenUsageError message={state.message} onRetry={onRetry} />;
  if (isUsagePreparing(state.data))
    return <TokenUsagePreparing onRetry={onRetry} refreshing={state.refreshing} />;
  if (!state.data.has_history) {
    return (
      <TokenUsageSavedValues state={state} onRetry={onRetry}>
        <TokenUsageEmpty
          hasHistory={false}
          onAllTime={() => onRangeChange("all")}
          pluginSettingsHref={pluginSettingsHref}
        />
      </TokenUsageSavedValues>
    );
  }
  if (!state.data.has_range_data && !state.data.undated_coverage) {
    return (
      <TokenUsageSavedValues state={state} onRetry={onRetry}>
        <TokenUsageEmpty hasHistory onAllTime={() => onRangeChange("all")} />
      </TokenUsageSavedValues>
    );
  }
  return (
    <TokenUsageSavedValues state={state} onRetry={onRetry}>
      <TokenUsageContent
        data={state.data}
        trendState={trendState}
        onRetry={onRetry}
        view={breakdownView}
        onViewChange={onViewChange}
        provider={provider}
        onProviderChange={onProviderChange}
        providers={state.data.providers}
        onSort={onSort}
        sortKey={sortKey}
        sortDirection={sortDirection}
        offset={offset}
        limit={TOKEN_USAGE_PAGE_SIZE}
        onPageChange={onPageChange}
      />
    </TokenUsageSavedValues>
  );
}

function TokenUsageSavedValues({
  state,
  onRetry,
  children,
}: {
  state: Extract<TokenUsageRequestState, { status: "ready" }>;
  onRetry: () => void;
  children: ReactNode;
}) {
  return (
    <>
      <TokenUsageProjectionNotice
        response={state.data}
        refreshing={state.refreshing}
        onRetry={onRetry}
      />
      {state.error && <TokenUsageError message={state.error} onRetry={onRetry} />}
      {children}
    </>
  );
}

function usageGroupForView(view: TokenUsageView): "model" | "day" | "month" | "task" | "session" {
  switch (view) {
    case "daily":
      return "day";
    case "monthly":
      return "month";
    case "tasks":
      return "task";
    case "sessions":
      return "session";
    default:
      return "model";
  }
}

function apiSortKey(view: TokenUsageView, key: TokenUsageSortKey): string {
  if (key === "period") return "period";
  if (key === "label") {
    switch (view) {
      case "daily":
      case "monthly":
        return "period";
      case "tasks":
        return "task";
      case "sessions":
        return "session";
      default:
        return "model";
    }
  }
  if (key === "effective_cost_per_million") return "rate";
  return TOKEN_USAGE_SORT_ALIASES[key] ?? key;
}

function TokenUsageContent({
  data,
  trendState,
  onRetry,
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
}: {
  data: TokenUsageResponse;
  trendState: TokenUsageRequestState;
  onRetry: () => void;
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
}) {
  return (
    <>
      <TokenUsageSummary
        metrics={data.totals}
        datedCoverage={data.dated_coverage}
        undatedCoverage={data.undated_coverage}
        lastUpdated={data.last_updated}
      />
      <TokenUsageCoverageNotice response={data} />
      <TokenUsageTrendState state={trendState} fallback={data} onRetry={onRetry} />
      <TokenUsageBreakdown
        response={data}
        view={view}
        onViewChange={onViewChange}
        provider={provider}
        onProviderChange={onProviderChange}
        providers={providers}
        onSort={onSort}
        sortKey={sortKey}
        sortDirection={sortDirection}
        offset={offset}
        limit={limit}
        onPageChange={onPageChange}
      />
    </>
  );
}

function TokenUsageTrendState({
  state,
  fallback,
  onRetry,
}: {
  state: TokenUsageRequestState;
  fallback: TokenUsageResponse;
  onRetry: () => void;
}) {
  const { t } = useTranslation();
  if (state.status === "loading") return <TokenUsageLoading />;
  if (state.status === "error") {
    return <TokenUsageError message={state.message} onRetry={onRetry} />;
  }
  if (state.status !== "ready") return <TokenUsageLoading />;
  const data = state.data;
  if (isUsagePreparing(data))
    return <TokenUsagePreparing onRetry={onRetry} refreshing={state.refreshing} />;
  if (data.projection_generation !== fallback.projection_generation)
    return <TokenUsageError message={t("stats:usageGenerationChanged")} onRetry={onRetry} />;
  return (
    <>
      {state.error && <TokenUsageError message={state.error} onRetry={onRetry} />}
      <TokenUsageTrend
        rows={data.rows}
        currency={data.summary.currency}
        datedCoverage={data.dated_coverage}
      />
    </>
  );
}
