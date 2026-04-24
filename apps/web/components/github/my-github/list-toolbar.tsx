"use client";

import { IconRefresh } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
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
  return (
    <div className="px-6 py-2.5 border-b shrink-0 flex items-center gap-3 flex-wrap">
      <div className="flex items-baseline gap-2 min-w-0">
        <h2 className="text-sm font-semibold truncate">{title}</h2>
        <span className="text-xs text-muted-foreground tabular-nums">{loading ? "…" : count}</span>
      </div>
      <Select
        value={selectValue}
        onValueChange={(v) => onRepoFilterChange(v === ALL_REPOS ? "" : v)}
      >
        <SelectTrigger className="w-[220px] h-8 cursor-pointer">
          <SelectValue placeholder="All repos" />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={ALL_REPOS} className="cursor-pointer">
            All repos
          </SelectItem>
          {repoOptions.map((key) => (
            <SelectItem key={key} value={key} className="cursor-pointer">
              {key}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <div className="flex-1 min-w-[240px] relative">
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
          <span className="pointer-events-none absolute right-2 top-1/2 -translate-y-1/2 text-[10px] uppercase tracking-wider text-muted-foreground">
            Press Enter
          </span>
        )}
      </div>
      <div className="flex items-center gap-2 ml-auto">
        {lastFetchedAt && !loading && (
          <span className="text-xs text-muted-foreground whitespace-nowrap">
            Updated {formatRelativeTime(lastFetchedAt.toISOString())}
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
      </div>
    </div>
  );
}
