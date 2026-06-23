"use client";

import { useMemo } from "react";
import { IconRefresh } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Combobox, type ComboboxOption } from "@/components/combobox";
import { cn } from "@/lib/utils";
import { formatRelativeTime } from "@/lib/utils";

const ALL_REPOS = "__all__";

type ListToolbarProps = {
  title: string;
  count: number;
  loading: boolean;
  lastFetchedAt: Date | null;
  customQuery: string;
  committedQuery: string;
  onCustomQueryChange: (value: string) => void;
  onCommitCustomQuery: () => void;
  repoFilter: string;
  onRepoFilterChange: (value: string) => void;
  repoOptions: string[];
  onRefresh: () => void;
};

function RefreshControls({
  loading,
  lastFetchedAt,
  onRefresh,
  showUpdatedPrefix,
}: {
  loading: boolean;
  lastFetchedAt: Date | null;
  onRefresh: () => void;
  showUpdatedPrefix: boolean;
}) {
  return (
    <>
      {lastFetchedAt && !loading && (
        <span className="text-xs text-muted-foreground whitespace-nowrap">
          {showUpdatedPrefix ? "Updated " : ""}
          {formatRelativeTime(lastFetchedAt.toISOString())}
        </span>
      )}
      <Button
        variant="ghost"
        size="icon"
        className="h-8 w-8 cursor-pointer"
        onClick={onRefresh}
        disabled={loading}
        title="Refresh"
      >
        <IconRefresh className={cn("h-4 w-4", loading && "animate-spin")} />
      </Button>
    </>
  );
}

function buildRepoFilterOptions(repoOptions: string[]): ComboboxOption[] {
  return [
    { value: ALL_REPOS, label: "All repos", keywords: ["all", "repositories", "repos"] },
    ...repoOptions.map((key) => ({ value: key, label: key, keywords: [key] })),
  ];
}

export function ListToolbar({
  title,
  count,
  loading,
  lastFetchedAt,
  customQuery,
  committedQuery,
  onCustomQueryChange,
  onCommitCustomQuery,
  repoFilter,
  onRepoFilterChange,
  repoOptions,
  onRefresh,
}: ListToolbarProps) {
  const selectValue = repoFilter || ALL_REPOS;
  const dirty = customQuery !== committedQuery;
  const repoFilterOptions = useMemo(() => buildRepoFilterOptions(repoOptions), [repoOptions]);
  return (
    <div className="px-4 sm:px-6 py-2.5 border-b shrink-0 flex flex-col md:flex-row md:items-center md:flex-wrap gap-2 md:gap-3">
      <div className="flex items-center gap-2 min-w-0">
        <div className="flex items-baseline gap-2 min-w-0 flex-1 md:flex-initial">
          <h2 className="text-sm font-semibold truncate" data-testid="github-list-toolbar-title">
            {title}
          </h2>
          <span className="text-xs text-muted-foreground tabular-nums">
            {loading ? "…" : count}
          </span>
        </div>
        <div className="flex items-center gap-2 md:hidden">
          <RefreshControls
            loading={loading}
            lastFetchedAt={lastFetchedAt}
            onRefresh={onRefresh}
            showUpdatedPrefix={false}
          />
        </div>
      </div>
      <Combobox
        value={selectValue}
        onValueChange={(v) => {
          // Combobox signals "toggle off" with ""; All repos is the explicit clear path.
          if (!v) return;
          onRepoFilterChange(v === ALL_REPOS ? "" : v);
        }}
        options={repoFilterOptions}
        placeholder="All repos"
        searchPlaceholder="Filter repositories..."
        emptyMessage="No repositories found."
        triggerClassName="w-full md:w-[220px] h-8 border border-input bg-background hover:bg-secondary/50 px-2 py-1.5 text-xs/relaxed"
        className="md:min-w-[360px]"
        testId="github-repo-filter-trigger"
        dropdownTestId="github-repo-filter-dropdown"
      />
      <div className="w-full md:flex-1 md:min-w-[240px] relative">
        <Input
          value={customQuery}
          onChange={(e) => onCustomQueryChange(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              onCommitCustomQuery();
            }
          }}
          onBlur={() => {
            if (dirty) onCommitCustomQuery();
          }}
          placeholder='Custom query — press Enter. e.g. "is:open review-requested:@me"'
          className="h-8 pr-20"
        />
        {dirty && (
          <span className="pointer-events-none absolute right-2 top-1/2 -translate-y-1/2 text-[10px] uppercase tracking-wider text-muted-foreground hidden sm:inline">
            Press Enter
          </span>
        )}
      </div>
      <div className="hidden md:flex items-center gap-2 md:ml-auto">
        <RefreshControls
          loading={loading}
          lastFetchedAt={lastFetchedAt}
          onRefresh={onRefresh}
          showUpdatedPrefix
        />
      </div>
    </div>
  );
}
