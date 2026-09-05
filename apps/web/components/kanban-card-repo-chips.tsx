"use client";

import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import type { RepositoryChip } from "@/components/kanban-card";

const REPO_CHIPS_VISIBLE = 2;

function RepoChip({ chip }: { chip: RepositoryChip }) {
  const badge = (
    <span
      title={chip.path}
      className="shrink-0 rounded-sm bg-muted/60 px-1 py-px text-[9px] font-medium text-muted-foreground leading-tight max-w-[8rem] truncate"
    >
      {chip.label}
    </span>
  );
  if (!chip.path) return badge;
  return (
    <Tooltip>
      <TooltipTrigger asChild>{badge}</TooltipTrigger>
      <TooltipContent side="top" align="start">
        <span className="max-w-[22rem] break-all text-xs">{chip.path}</span>
      </TooltipContent>
    </Tooltip>
  );
}

function OverflowRepoTooltip({ chips }: { chips: RepositoryChip[] }) {
  return (
    <div className="flex max-w-[24rem] flex-col gap-1 text-xs">
      {chips.map((chip) => (
        <div key={`${chip.label}:${chip.path ?? ""}`} className="min-w-0">
          <div className="font-medium">{chip.label}</div>
          {chip.path && <div className="break-all text-muted-foreground">{chip.path}</div>}
        </div>
      ))}
    </div>
  );
}

export function RepoChipRow({ chips }: { chips: RepositoryChip[] }) {
  if (chips.length === 0) return null;
  const visible = chips.slice(0, REPO_CHIPS_VISIBLE);
  const overflow = chips.slice(REPO_CHIPS_VISIBLE);
  return (
    <div className="mb-1 flex items-center gap-1 min-w-0 overflow-hidden">
      {visible.map((chip) => (
        <RepoChip key={`${chip.label}:${chip.path ?? ""}`} chip={chip} />
      ))}
      {overflow.length > 0 && (
        <Tooltip>
          <TooltipTrigger asChild>
            <span className="shrink-0 rounded-sm bg-muted px-1 py-px text-[9px] font-medium text-muted-foreground/80">
              +{overflow.length}
            </span>
          </TooltipTrigger>
          <TooltipContent side="top" align="start">
            <OverflowRepoTooltip chips={overflow} />
          </TooltipContent>
        </Tooltip>
      )}
    </div>
  );
}
