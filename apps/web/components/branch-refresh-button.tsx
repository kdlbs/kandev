"use client";

import { IconRefresh } from "@tabler/icons-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";

type BranchRefreshButtonProps = {
  onRefresh: () => void;
  refreshing?: boolean;
  fetchedAt?: string;
};

export function BranchRefreshButton({
  onRefresh,
  refreshing,
  fetchedAt,
}: BranchRefreshButtonProps) {
  const tooltip = formatRefreshTooltip(fetchedAt, refreshing);
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <button
          type="button"
          aria-label="Refresh branches"
          data-testid="branch-refresh-button"
          onClick={(e) => {
            e.preventDefault();
            e.stopPropagation();
            onRefresh();
          }}
          disabled={refreshing}
          className={`inline-flex h-6 w-6 items-center justify-center rounded-md text-muted-foreground hover:bg-muted/40 hover:text-foreground ${refreshing ? "cursor-not-allowed opacity-50" : "cursor-pointer"}`}
        >
          <IconRefresh className={`h-3.5 w-3.5 ${refreshing ? "animate-spin" : ""}`} />
        </button>
      </TooltipTrigger>
      <TooltipContent>{tooltip}</TooltipContent>
    </Tooltip>
  );
}

function formatRefreshTooltip(fetchedAt: string | undefined, refreshing: boolean | undefined) {
  if (refreshing) return "Refreshing branches...";
  if (!fetchedAt) return "Refresh branches (git fetch)";
  const date = new Date(fetchedAt);
  if (Number.isNaN(date.getTime())) return "Refresh branches (git fetch)";
  return `Refresh branches (last fetched ${date.toLocaleTimeString()})`;
}
