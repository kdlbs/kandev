"use client";

import { IconRefresh } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { cn, formatRelativeTime } from "@/lib/utils";

const ALL_PROJECTS = "__all__";

type ListToolbarProps = {
  title: string;
  count: number;
  loading: boolean;
  lastFetchedAt: Date | null;
  customQuery: string;
  committedQuery: string;
  onCustomQueryChange: (value: string) => void;
  onCommitCustomQuery: () => void;
  projectFilter: string;
  onProjectFilterChange: (value: string) => void;
  projectOptions: string[];
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
        data-testid="gitlab-list-toolbar-refresh"
      >
        <IconRefresh className={cn("h-4 w-4", loading && "animate-spin")} />
      </Button>
    </>
  );
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
  projectFilter,
  onProjectFilterChange,
  projectOptions,
  onRefresh,
}: ListToolbarProps) {
  const selectValue = projectFilter || ALL_PROJECTS;
  const dirty = customQuery !== committedQuery;
  return (
    <div className="px-4 sm:px-6 py-2.5 border-b shrink-0 flex flex-col md:flex-row md:items-center md:flex-wrap gap-2 md:gap-3">
      <div className="flex items-center gap-2 min-w-0">
        <div className="flex items-baseline gap-2 min-w-0 flex-1 md:flex-initial">
          <h2 className="text-sm font-semibold truncate" data-testid="gitlab-list-toolbar-title">
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
      <Select
        value={selectValue}
        onValueChange={(v) => onProjectFilterChange(v === ALL_PROJECTS ? "" : v)}
      >
        <SelectTrigger
          className="w-full md:w-[220px] h-8 cursor-pointer"
          data-testid="gitlab-project-filter-trigger"
        >
          <SelectValue placeholder="All projects" />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={ALL_PROJECTS} className="cursor-pointer">
            All projects
          </SelectItem>
          {projectOptions.map((key) => (
            <SelectItem key={key} value={key} className="cursor-pointer">
              {key}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
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
          placeholder='Custom query — press Enter. e.g. "labels=bug&state=opened"'
          className="h-8 pr-20"
          data-testid="gitlab-list-toolbar-custom-query"
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
