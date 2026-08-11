"use client";

import {
  IconCircleCheckFilled,
  IconCircleXFilled,
  IconClock,
  IconGitMerge,
  IconLoader2,
  IconPointFilled,
} from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { cn } from "@/lib/utils";
import { isMRReadyToMerge } from "./mr-task-icon";
import type { MRChipStatus } from "./mr-task-icon";
import type { ChipAutomation } from "./mr-status-chip-selection";
import type { TaskMR } from "@/lib/types/gitlab";

export const MR_CHIP_TRIGGER_CLASS =
  "cursor-pointer inline-flex items-center gap-1 rounded-md px-1 py-0.5 text-xs";

const STATUS_LABEL_KEY: Record<MRChipStatus, string> = {
  merged: "gitlab:mrChipStatusMerged",
  closed: "gitlab:mrChipStatusClosed",
  failed: "gitlab:mrChipStatusFailed",
  draft: "gitlab:mrChipStatusDraft",
  ready: "gitlab:mrChipStatusReady",
  awaiting_approval: "gitlab:mrChipStatusAwaitingApproval",
  running: "gitlab:mrChipStatusRunning",
  neutral: "gitlab:mrChipStatusNeutral",
};

export function MRStatusChipGlyph({ status }: { status: MRChipStatus }) {
  const cls = "h-3.5 w-3.5";
  const testProps = { "data-testid": "mr-status-glyph", "data-status": status } as const;
  switch (status) {
    case "merged":
      return (
        <IconGitMerge {...testProps} className={cn(cls, "text-purple-500")} aria-hidden="true" />
      );
    case "failed":
      return (
        <IconCircleXFilled {...testProps} className={cn(cls, "text-red-500")} aria-hidden="true" />
      );
    case "ready":
      return (
        <IconCircleCheckFilled
          {...testProps}
          className={cn(cls, "text-emerald-400")}
          aria-hidden="true"
        />
      );
    case "awaiting_approval":
      return <IconClock {...testProps} className={cn(cls, "text-sky-400")} aria-hidden="true" />;
    case "running":
      return (
        <IconLoader2
          {...testProps}
          className={cn(cls, "text-yellow-500 animate-spin [animation-duration:3s]")}
          aria-hidden="true"
        />
      );
    default:
      // closed, draft, neutral all render the same muted dot.
      return (
        <IconPointFilled
          {...testProps}
          className={cn(cls, "text-muted-foreground")}
          aria-hidden="true"
        />
      );
  }
}

function useMRChipAriaLabel(openCount: number, liveStatus: MRChipStatus, actedOnMR: TaskMR) {
  const { t } = useTranslation();
  const status = t(STATUS_LABEL_KEY[liveStatus]);
  if (openCount > 1) return t("gitlab:mrChipAriaLabelMulti", { count: openCount, status });
  return t("gitlab:mrChipAriaLabelSingle", { mriid: actedOnMR.mr_iid, status });
}

export type MRChipTriggerAttrs = {
  "data-testid": "mr-status-chip";
  "data-status": MRChipStatus;
  "data-mr-count": number;
  "data-mr-iid": number;
  "data-mr-state": string;
  "data-mr-ready-to-merge": "true" | "false";
  "data-selection-frozen": "true" | "false";
  "aria-label": string;
  className: string;
};

export function useMRChipTriggerAttrs({
  openCount,
  liveStatus,
  actedOnMR,
  frozen,
}: {
  openCount: number;
  liveStatus: MRChipStatus;
  actedOnMR: TaskMR;
  frozen: boolean;
}): MRChipTriggerAttrs {
  return {
    "data-testid": "mr-status-chip",
    "data-status": liveStatus,
    "data-mr-count": openCount,
    "data-mr-iid": actedOnMR.mr_iid,
    "data-mr-state": actedOnMR.state,
    "data-mr-ready-to-merge": isMRReadyToMerge(actedOnMR) ? "true" : "false",
    "data-selection-frozen": frozen ? "true" : "false",
    "aria-label": useMRChipAriaLabel(openCount, liveStatus, actedOnMR),
    className: MR_CHIP_TRIGGER_CLASS,
  };
}

function AutomationFlagBadges({ automation }: { automation: ChipAutomation }) {
  const { t } = useTranslation();
  if (!automation.autoFixEnabled && !automation.autoMergeEnabled) return null;
  const round = automation.autoFixRound;
  return (
    <>
      {automation.autoFixEnabled && round && (
        <span
          data-testid="mr-status-auto-fix-chip"
          data-auto-fix-exhausted={round.exhausted ? "true" : "false"}
          className={cn(
            "rounded-sm px-1 py-0.5 text-[9px] font-medium leading-none",
            round.exhausted
              ? "bg-yellow-500/15 text-yellow-500"
              : "bg-emerald-500/15 text-emerald-500",
          )}
        >
          {t("gitlab:mrChipAutoFix")} {round.current}/{round.max}
        </span>
      )}
      {automation.autoMergeEnabled && (
        <span
          data-testid="mr-status-auto-merge-chip"
          className="rounded-sm bg-sky-500/15 px-1 py-0.5 text-[9px] font-medium leading-none text-sky-500"
        >
          {t("gitlab:mrChipAutoMerge")}
        </span>
      )}
    </>
  );
}

export function MRStatusChipTriggerContent({
  openCount,
  liveStatus,
  automation,
}: {
  openCount: number;
  liveStatus: MRChipStatus;
  automation: ChipAutomation;
}) {
  return (
    <>
      <IconGitMerge className="h-3.5 w-3.5 text-muted-foreground" aria-hidden="true" />
      <MRStatusChipGlyph status={liveStatus} />
      {openCount > 1 && (
        <span className="text-[9px] font-semibold leading-none tabular-nums">{openCount}</span>
      )}
      <AutomationFlagBadges automation={automation} />
    </>
  );
}
