"use client";

import type { ReactNode } from "react";
import { IconRefresh } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { cn, formatRelativeTime } from "@/lib/utils";

export type IntegrationListToolbarProps = {
  title: string;
  count: number;
  loading: boolean;
  lastFetchedAt: Date | null;
  customQuery: string;
  committedQuery: string;
  onCustomQueryChange: (value: string) => void;
  onCommitCustomQuery: () => void;
  onRefresh: () => void;
  filter: ReactNode;
  queryPlaceholder: string;
  titleTestId?: string;
  queryTestId?: string;
  refreshTestId?: string;
};

type RefreshControlsProps = Pick<
  IntegrationListToolbarProps,
  "loading" | "lastFetchedAt" | "onRefresh" | "refreshTestId"
> & { showUpdatedPrefix: boolean };

function RefreshControls({
  loading,
  lastFetchedAt,
  onRefresh,
  refreshTestId,
  showUpdatedPrefix,
}: RefreshControlsProps) {
  return (
    <>
      {lastFetchedAt && !loading ? (
        <span className="whitespace-nowrap text-xs text-muted-foreground">
          {showUpdatedPrefix ? "Updated " : ""}
          {formatRelativeTime(lastFetchedAt.toISOString())}
        </span>
      ) : null}
      <Button
        variant="ghost"
        size="icon"
        className="h-8 w-8 cursor-pointer"
        onClick={onRefresh}
        disabled={loading}
        title="Refresh"
        data-testid={refreshTestId}
      >
        <IconRefresh className={cn("h-4 w-4", loading && "animate-spin")} />
      </Button>
    </>
  );
}

export function IntegrationListToolbar({
  title,
  count,
  loading,
  lastFetchedAt,
  customQuery,
  committedQuery,
  onCustomQueryChange,
  onCommitCustomQuery,
  onRefresh,
  filter,
  queryPlaceholder,
  titleTestId,
  queryTestId,
  refreshTestId,
}: IntegrationListToolbarProps) {
  const dirty = customQuery !== committedQuery;
  return (
    <div className="flex shrink-0 flex-col gap-2 border-b px-4 py-2.5 sm:px-6 md:flex-row md:flex-wrap md:items-center md:gap-3">
      <div className="flex min-w-0 items-center gap-2">
        <div className="flex min-w-0 flex-1 items-baseline gap-2 md:flex-initial">
          <h2 className="truncate text-sm font-semibold" data-testid={titleTestId}>
            {title}
          </h2>
          <span className="text-xs tabular-nums text-muted-foreground">
            {loading ? "…" : count}
          </span>
        </div>
        <div className="flex items-center gap-2 md:hidden">
          <RefreshControls
            loading={loading}
            lastFetchedAt={lastFetchedAt}
            onRefresh={onRefresh}
            refreshTestId={refreshTestId}
            showUpdatedPrefix={false}
          />
        </div>
      </div>
      {filter}
      <div className="relative w-full md:min-w-[240px] md:flex-1">
        <Input
          value={customQuery}
          onChange={(event) => onCustomQueryChange(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === "Enter") {
              event.preventDefault();
              onCommitCustomQuery();
            }
          }}
          onBlur={() => {
            if (dirty) onCommitCustomQuery();
          }}
          placeholder={queryPlaceholder}
          className="h-8 pr-20"
          data-testid={queryTestId}
        />
        {dirty ? (
          <span className="pointer-events-none absolute right-2 top-1/2 hidden -translate-y-1/2 text-[10px] uppercase tracking-wider text-muted-foreground sm:inline">
            Press Enter
          </span>
        ) : null}
      </div>
      <div className="ml-auto hidden items-center gap-2 md:flex">
        <RefreshControls
          loading={loading}
          lastFetchedAt={lastFetchedAt}
          onRefresh={onRefresh}
          refreshTestId={refreshTestId}
          showUpdatedPrefix
        />
      </div>
    </div>
  );
}
