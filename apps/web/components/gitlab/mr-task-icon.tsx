"use client";

import { IconGitMerge } from "@tabler/icons-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { cn } from "@/lib/utils";
import { useTaskMRs } from "@/hooks/domains/gitlab/use-task-mr";
import type { TaskMR } from "@/lib/types/gitlab";

const MUTED_FOREGROUND = "text-muted-foreground";
const PURPLE_500 = "text-purple-500";
const RED_500 = "text-red-500";
const YELLOW_500 = "text-yellow-500";
const SKY_400 = "text-sky-400";
const EMERALD_400 = "text-emerald-400";

/**
 * The single source of MR status colour for the GitLab directory (C10, Q5).
 * Consumed by both the Kanban card badge (MRTaskIcon, below) and the MR
 * topbar button (mr-topbar-button.tsx) — previously two private,
 * disagreeing copies (statusTextColor / MRStatusIcon). Evaluated in this
 * exact priority order (first match wins); see plan AC35 for the table this
 * mirrors row-for-row.
 */
export function getMRStatusColor(mr: TaskMR): string {
  if (mr.state === "merged") return PURPLE_500;
  if (mr.state === "closed") return MUTED_FOREGROUND;
  if (mr.state === "locked") return MUTED_FOREGROUND;
  if (mr.pipeline_state === "failure") return RED_500;
  if (mr.draft) return MUTED_FOREGROUND;
  if (mr.approval_state === "approved" && mr.pipeline_state === "success") return EMERALD_400;
  if (mr.approval_state === "pending") return SKY_400;
  if (mr.pipeline_state === "pending") return YELLOW_500;
  return MUTED_FOREGROUND;
}

const STATUS_RANK: Record<string, number> = {
  // Higher = more attention-worthy. Drives the aggregated badge colour when
  // a task has multiple MRs (mirrors github's aggregatePRStatusColor).
  [RED_500]: 4,
  [YELLOW_500]: 3,
  [SKY_400]: 2,
  [EMERALD_400]: 1,
  [PURPLE_500]: 0,
  [MUTED_FOREGROUND]: 0,
};

/** Mirrors mrAutomationReadyToMerge's non-fetch-dependent subset for display purposes only. */
export function isMRReadyToMerge(mr: TaskMR): boolean {
  return (
    mr.state === "open" &&
    !mr.draft &&
    mr.pipeline_state === "success" &&
    mr.approval_state === "approved"
  );
}

export function getMRTooltip(mr: TaskMR): string {
  const parts = [`!${mr.mr_iid}: ${mr.mr_title}`];
  if (mr.state !== "open") parts.push(`State: ${mr.state}`);
  if (mr.approval_state) parts.push(`Review: ${mr.approval_state}`);
  if (mr.pipeline_state) parts.push(`Pipeline: ${mr.pipeline_state}`);
  if (mr.draft) parts.push("Draft");
  else if (isMRReadyToMerge(mr)) parts.push("Ready to merge");
  return parts.join(" | ");
}

/**
 * Picks the most attention-worthy colour across N MRs. Terminal
 * (merged/closed/locked) MRs are dropped when at least one MR is still
 * open, so a landed MR followed by a new open one surfaces the live MR's
 * status. Falls back to the first MR's colour when every MR is terminal.
 * Mirrors github's aggregatePRStatusColor (AC28).
 */
export function aggregateMRStatusColor(mrs: TaskMR[]): string {
  if (mrs.length === 0) return MUTED_FOREGROUND;
  const open = mrs.filter((mr) => mr.state === "open");
  const target = open.length > 0 ? open : mrs;
  let bestColor = MUTED_FOREGROUND;
  let bestRank = -1;
  for (const mr of target) {
    const color = getMRStatusColor(mr);
    const rank = STATUS_RANK[color] ?? 0;
    if (rank > bestRank) {
      bestRank = rank;
      bestColor = color;
    }
  }
  return bestColor;
}

/**
 * Colour + glyph for a single MR's combined state, exported so the MR
 * topbar button can render the identical trigger icon without a second
 * copy of the priority table (AC36).
 */
export function MRStatusIcon({ mr, className }: { mr: TaskMR; className?: string }) {
  return <IconGitMerge className={cn("h-3.5 w-3.5", getMRStatusColor(mr), className)} />;
}

/**
 * Linked-MR icon badge for the task's Kanban card, mirroring github's
 * PRTaskIcon placement and style. Reads state.taskMRs directly via
 * useTaskMRs (C8's store-shape difference from taskPRs).
 */
export function MRTaskIcon({ taskId }: { taskId: string }) {
  const mrs = useTaskMRs(taskId);
  if (!Array.isArray(mrs) || mrs.length === 0) return null;
  if (mrs.length === 1) return <SingleMRIcon taskId={taskId} mr={mrs[0]} />;
  return <MultiMRIcon taskId={taskId} mrs={mrs} />;
}

function SingleMRIcon({ taskId, mr }: { taskId: string; mr: TaskMR }) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          data-testid={`mr-task-icon-${taskId}`}
          data-mr-state={mr.state}
          data-mr-count="1"
          data-mr-ready-to-merge={isMRReadyToMerge(mr) ? "true" : "false"}
          className={cn("inline-flex items-center shrink-0", getMRStatusColor(mr))}
        >
          <IconGitMerge className="h-3.5 w-3.5" />
        </span>
      </TooltipTrigger>
      {/* MR title/state come from the linked GitLab MR itself, not
          translatable UI copy — mirrors github's untranslated getPRTooltip. */}
      <TooltipContent>{getMRTooltip(mr)}</TooltipContent>
    </Tooltip>
  );
}

function MultiMRIcon({ taskId, mrs }: { taskId: string; mrs: TaskMR[] }) {
  const aggregateColor = aggregateMRStatusColor(mrs);
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          data-testid={`mr-task-icon-${taskId}`}
          data-mr-count={mrs.length}
          className={cn("inline-flex items-center gap-0.5 shrink-0", aggregateColor)}
        >
          <IconGitMerge className="h-3.5 w-3.5" />
          <span className="text-[9px] font-semibold leading-none">{mrs.length}</span>
        </span>
      </TooltipTrigger>
      <TooltipContent>
        <div className="flex flex-col gap-1 text-xs">
          {mrs.map((mr) => (
            <div key={mr.id} className="flex items-center gap-2">
              <span className={cn("inline-flex shrink-0", getMRStatusColor(mr))}>
                <IconGitMerge className="h-3 w-3" />
              </span>
              <span>{getMRTooltip(mr)}</span>
            </div>
          ))}
        </div>
      </TooltipContent>
    </Tooltip>
  );
}
