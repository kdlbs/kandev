"use client";

import { IconGitMerge } from "@tabler/icons-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useTranslation } from "react-i18next";
import { cn } from "@/lib/utils";
import { useChangeRequestTaskTooltipState } from "@/components/integrations/use-change-request-task-tooltip-state";
import { useTaskMRs } from "@/hooks/domains/gitlab/use-task-mr";
import type { TaskMR } from "@/lib/types/gitlab";
import { deriveMRTaskStatusSummary, MRTaskStatusSummary } from "./mr-task-status-summary";

const MUTED_FOREGROUND = "text-muted-foreground";
const PURPLE_500 = "text-purple-500";
const RED_500 = "text-red-500";
const YELLOW_500 = "text-yellow-500";
const SKY_400 = "text-sky-400";
const EMERALD_400 = "text-emerald-400";

/**
 * The eight-state MR chip status derivation (spec: gitlab-mr-status-chip,
 * State machine). Total function, evaluated in this exact priority order —
 * first match wins. `getMRStatusColor` below is re-expressed as a lookup
 * from this so exactly one priority table exists (was previously its own
 * copy of the same if-chain).
 */
export type MRChipStatus =
  | "merged"
  | "closed"
  | "failed"
  | "draft"
  | "ready"
  | "awaiting_approval"
  | "running"
  | "neutral";

export function mrChipStatus(mr: TaskMR): MRChipStatus {
  if (mr.state === "merged") return "merged";
  if (mr.state === "closed" || mr.state === "locked") return "closed";
  if (mr.pipeline_state === "failure") return "failed";
  if (mr.draft) return "draft";
  if (mr.approval_state === "approved" && mr.pipeline_state === "success") return "ready";
  if (mr.approval_state === "pending") return "awaiting_approval";
  if (mr.pipeline_state === "pending") return "running";
  return "neutral";
}

const MR_CHIP_STATUS_COLOR: Record<MRChipStatus, string> = {
  merged: PURPLE_500,
  closed: MUTED_FOREGROUND,
  failed: RED_500,
  draft: MUTED_FOREGROUND,
  ready: EMERALD_400,
  awaiting_approval: SKY_400,
  running: YELLOW_500,
  neutral: MUTED_FOREGROUND,
};

/**
 * The single source of MR status colour for the GitLab directory (C10, Q5).
 * Consumed by both the Kanban card badge (MRTaskIcon, below) and the MR
 * topbar button (mr-topbar-button.tsx) — previously two private,
 * disagreeing copies (statusTextColor / MRStatusIcon). See plan AC35 for
 * the table this mirrors row-for-row.
 */
export function getMRStatusColor(mr: TaskMR): string {
  return MR_CHIP_STATUS_COLOR[mrChipStatus(mr)];
}

// Higher = more attention-worthy. Drives the aggregated badge colour when a
// task has multiple MRs (mirrors github's aggregatePRStatusColor). Exported
// so a test can assert order-equivalence against MR_CHIP_STATUS_RANK below
// without hand-restating either table (spec round-4 finding F34).
export const STATUS_RANK: Record<string, number> = {
  [RED_500]: 4,
  [YELLOW_500]: 3,
  [SKY_400]: 2,
  [EMERALD_400]: 1,
  [PURPLE_500]: 0,
  [MUTED_FOREGROUND]: 0,
};

// Ranks the eight chip statuses by attention-worthiness for selectChipMR's
// rank filter (step 2). A separate table from STATUS_RANK above: this one is
// keyed by status, that one by colour, and they serve different callers —
// they are required only to be order-equivalent under the colour map, not
// identical (spec: State machine, "Rank, and why aggregation has no tiebreak
// of its own").
export const MR_CHIP_STATUS_RANK: Record<MRChipStatus, number> = {
  failed: 5,
  running: 4,
  awaiting_approval: 3,
  ready: 2,
  merged: 0,
  closed: 0,
  draft: 0,
  neutral: 0,
};

/**
 * Total order over MRs used to break rank ties deterministically:
 * `mr_iid` ascending, then `project_path` ascending (byte-wise `<`), then
 * `id` ascending. `id` is the association primary key, so this is total.
 * Shared by selectChipMR (below) and the chip's badge-selection rule, which
 * uses the identical tiebreak (spec: Selection and ordering).
 */
export function compareChipMR(a: TaskMR, b: TaskMR): number {
  if (a.mr_iid !== b.mr_iid) return a.mr_iid - b.mr_iid;
  if (a.project_path !== b.project_path) return a.project_path < b.project_path ? -1 : 1;
  if (a.id === b.id) return 0;
  return a.id < b.id ? -1 : 1;
}

function isChipMRBetter(candidate: TaskMR, current: TaskMR): boolean {
  const candidateRank = MR_CHIP_STATUS_RANK[mrChipStatus(candidate)];
  const currentRank = MR_CHIP_STATUS_RANK[mrChipStatus(current)];
  if (candidateRank !== currentRank) return candidateRank > currentRank;
  return compareChipMR(candidate, current) < 0;
}

/**
 * Deterministically picks the one MR the chip renders and acts on: restrict
 * to `state === "open"` exactly, keep the maximum MR_CHIP_STATUS_RANK, then
 * tiebreak via compareChipMR. Pure and non-mutating — it never sorts or
 * otherwise reorders `mrs` in place, so a caller holding the same array
 * reference (useTaskMRs returns the store array by reference) never has it
 * silently reordered under it (spec round-4 finding F30).
 */
export function selectChipMR(mrs: TaskMR[]): TaskMR | null {
  let best: TaskMR | null = null;
  for (const mr of mrs) {
    if (mr.state !== "open") continue;
    if (!best || isChipMRBetter(mr, best)) best = mr;
  }
  return best;
}

/**
 * `aggregateMRChipStatus(mrs)` is defined as the composition
 * `mrChipStatus(selectChipMR(mrs))`, returning `neutral` when selectChipMR
 * returns null. It has no tiebreak rule of its own — see State machine,
 * "Rank, and why aggregation has no tiebreak of its own".
 */
export function aggregateMRChipStatus(mrs: TaskMR[]): MRChipStatus {
  const selected = selectChipMR(mrs);
  return selected ? mrChipStatus(selected) : "neutral";
}

/**
 * Best-effort client-side approximation of the backend's
 * mrAutomationReadyToMerge, evaluated against whatever TaskMR fields are
 * already on hand — for the tooltip/badge only; it never gates a merge.
 * Deliberately does NOT check approval_state: the backend gate doesn't
 * either. It trusts GitLab's own merge-readiness verdict
 * (detailed_merge_status, or merge_status on pre-15.6 hosts) to have
 * already accounted for approval rules server-side, so an unsynced empty
 * approval_state can't make this read "not ready" while the backend is
 * about to merge. `unresolved_discussions` is only populated for
 * automation-subscribed MRs (see the TaskMR field's own doc comment), so
 * this can still under-report for a non-subscribed MR — but it must never
 * claim "ready" while a populated, non-zero count or a non-mergeable
 * verdict says otherwise.
 */
export function isMRReadyToMerge(mr: TaskMR): boolean {
  if (mr.state !== "open" || mr.draft) return false;
  if (mr.pipeline_state !== "success") return false;
  if (mr.unresolved_discussions > 0) return false;
  return mrMergeStatusReady(mr);
}

function mrMergeStatusReady(mr: TaskMR): boolean {
  if (mr.detailed_merge_status) return mr.detailed_merge_status === "mergeable";
  return mr.merge_status === "can_be_merged";
}

/**
 * True when at least one MR is open AND every open MR is ready to merge.
 * Terminal (merged/closed/locked) siblings are ignored so they can't drag the
 * result to false. Mirrors github's areAllOpenPRsReadyToMerge.
 */
export function areAllOpenMRsReadyToMerge(mrs: TaskMR[]): boolean {
  const openMRs = mrs.filter((mr) => mr.state === "open");
  return openMRs.length > 0 && openMRs.every(isMRReadyToMerge);
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
  const { t } = useTranslation();
  const tooltip = useChangeRequestTaskTooltipState();
  const readyToMerge = isMRReadyToMerge(mr);
  const summary = deriveMRTaskStatusSummary(mr, readyToMerge);
  return (
    <Tooltip open={tooltip.open}>
      <TooltipTrigger asChild>
        <span
          data-testid={`mr-task-icon-${taskId}`}
          data-mr-state={mr.state}
          data-mr-count="1"
          data-mr-ready-to-merge={readyToMerge ? "true" : "false"}
          role="img"
          tabIndex={0}
          aria-label={t("gitlab:mergeRequestStatus", { iid: mr.mr_iid })}
          onPointerEnter={tooltip.onPointerEnter}
          onPointerLeave={tooltip.onPointerLeave}
          onFocus={tooltip.onFocus}
          onBlur={tooltip.onBlur}
          className={cn("inline-flex items-center shrink-0", getMRStatusColor(mr))}
        >
          <IconGitMerge aria-hidden="true" className="h-3.5 w-3.5" />
        </span>
      </TooltipTrigger>
      <TooltipContent
        sideOffset={6}
        onEscapeKeyDown={tooltip.onEscapeKeyDown}
        className="w-80 max-w-[calc(100vw-1rem)] p-3"
      >
        <MRTaskStatusSummary summaries={[summary]} />
      </TooltipContent>
    </Tooltip>
  );
}

function MultiMRIcon({ taskId, mrs }: { taskId: string; mrs: TaskMR[] }) {
  const { t } = useTranslation();
  const tooltip = useChangeRequestTaskTooltipState();
  const aggregateColor = aggregateMRStatusColor(mrs);
  const allReady = areAllOpenMRsReadyToMerge(mrs);
  const summaries = mrs.map((mr) => deriveMRTaskStatusSummary(mr, isMRReadyToMerge(mr)));
  return (
    <Tooltip open={tooltip.open}>
      <TooltipTrigger asChild>
        <span
          data-testid={`mr-task-icon-${taskId}`}
          data-mr-count={mrs.length}
          data-mr-ready-to-merge={allReady ? "true" : "false"}
          role="img"
          tabIndex={0}
          aria-label={t("gitlab:mergeRequestStatuses", { count: mrs.length })}
          onPointerEnter={tooltip.onPointerEnter}
          onPointerLeave={tooltip.onPointerLeave}
          onFocus={tooltip.onFocus}
          onBlur={tooltip.onBlur}
          className={cn("inline-flex items-center gap-0.5 shrink-0", aggregateColor)}
        >
          <IconGitMerge aria-hidden="true" className="h-3.5 w-3.5" />
          <span className="text-[9px] font-semibold leading-none tabular-nums">{mrs.length}</span>
        </span>
      </TooltipTrigger>
      <TooltipContent
        sideOffset={6}
        onEscapeKeyDown={tooltip.onEscapeKeyDown}
        className="w-80 max-w-[calc(100vw-1rem)] p-3"
      >
        <MRTaskStatusSummary summaries={summaries} />
      </TooltipContent>
    </Tooltip>
  );
}
