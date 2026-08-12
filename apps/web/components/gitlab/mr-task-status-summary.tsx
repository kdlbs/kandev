"use client";

import {
  IconAlertTriangle,
  IconCheck,
  IconCircleDot,
  IconClockHour4,
  IconGitMerge,
  IconGitPullRequestDraft,
  IconX,
  type Icon as TablerIcon,
} from "@tabler/icons-react";
import type { TFunction } from "i18next";
import { useTranslation } from "react-i18next";
import { cn } from "@/lib/utils";
import type { TaskMR } from "@/lib/types/gitlab";

export type MRTaskSummaryRowKind = "state" | "review" | "ci" | "merge";
export type MRTaskSummaryTone = "success" | "danger" | "warning" | "info" | "merged" | "muted";
export type MRTaskSummaryStatus =
  | "merged"
  | "closed"
  | "approved"
  | "awaiting_approval"
  | "passed"
  | "failed"
  | "in_progress"
  | "draft"
  | "ready"
  | "mergeable"
  | "conflicts"
  | "unresolved_discussions"
  | "raw";

export type MRTaskSummaryRow = {
  kind: MRTaskSummaryRowKind;
  status: MRTaskSummaryStatus;
  tone: MRTaskSummaryTone;
  rawValue?: string;
};

export type MRTaskStatusSummaryData = {
  iid: number;
  title: string;
  rows: MRTaskSummaryRow[];
};

function rawRow(kind: MRTaskSummaryRowKind, rawValue: string): MRTaskSummaryRow {
  return { kind, status: "raw", tone: "muted", rawValue };
}

function deriveStateRow(state: string): MRTaskSummaryRow | null {
  if (!state || state === "open") return null;
  if (state === "merged") return { kind: "state", status: "merged", tone: "merged" };
  if (state === "closed" || state === "locked") {
    return { kind: "state", status: "closed", tone: "danger" };
  }
  return rawRow("state", state);
}

function deriveReviewRow(approvalState: string): MRTaskSummaryRow | null {
  if (!approvalState) return null;
  if (approvalState === "approved") {
    return { kind: "review", status: "approved", tone: "success" };
  }
  if (approvalState === "pending") {
    return { kind: "review", status: "awaiting_approval", tone: "info" };
  }
  return rawRow("review", approvalState);
}

function deriveCIRow(pipelineState: string): MRTaskSummaryRow | null {
  if (!pipelineState) return null;
  if (pipelineState === "success") return { kind: "ci", status: "passed", tone: "success" };
  if (pipelineState === "failure") return { kind: "ci", status: "failed", tone: "danger" };
  if (pipelineState === "pending") return { kind: "ci", status: "in_progress", tone: "warning" };
  return rawRow("ci", pipelineState);
}

// checking/unchecked/ci_still_running produce no merge row: the CI row above
// already communicates the wait, so a second "still checking" line would be
// redundant noise.
const MERGE_STATUS_NO_ROW = new Set(["checking", "unchecked", "ci_still_running"]);

function deriveMergeStatusRow(mergeStatus: string): MRTaskSummaryRow | null {
  if (!mergeStatus || MERGE_STATUS_NO_ROW.has(mergeStatus)) return null;
  if (mergeStatus === "mergeable" || mergeStatus === "can_be_merged") {
    return { kind: "merge", status: "mergeable", tone: "muted" };
  }
  if (mergeStatus === "broken_status" || mergeStatus === "cannot_be_merged") {
    return { kind: "merge", status: "conflicts", tone: "danger" };
  }
  if (mergeStatus === "discussions_not_resolved") {
    return { kind: "merge", status: "unresolved_discussions", tone: "warning" };
  }
  if (mergeStatus === "not_approved") {
    return { kind: "merge", status: "awaiting_approval", tone: "muted" };
  }
  return rawRow("merge", mergeStatus);
}

function deriveMergeRow(mr: TaskMR, readyToMerge: boolean): MRTaskSummaryRow | null {
  if (mr.state !== "open") return null;
  if (mr.draft) return { kind: "merge", status: "draft", tone: "muted" };
  if (readyToMerge) return { kind: "merge", status: "ready", tone: "success" };
  return deriveMergeStatusRow(mr.detailed_merge_status || mr.merge_status);
}

export function deriveMRTaskStatusSummary(
  mr: TaskMR,
  readyToMerge: boolean,
): MRTaskStatusSummaryData {
  const rows = [
    deriveStateRow(mr.state),
    deriveReviewRow(mr.approval_state),
    deriveCIRow(mr.pipeline_state),
    deriveMergeRow(mr, readyToMerge),
  ].filter((row): row is MRTaskSummaryRow => row !== null);

  return { iid: mr.mr_iid, title: mr.mr_title, rows };
}

const ROW_LABEL_KEYS: Record<MRTaskSummaryRowKind, string> = {
  state: "gitlab:mrTaskStatusState",
  review: "gitlab:mrTaskStatusReview",
  ci: "gitlab:mrTaskStatusCi",
  merge: "gitlab:mrTaskStatusMerge",
};

const TONE_CLASSES: Record<MRTaskSummaryTone, string> = {
  success: "text-emerald-600 dark:text-emerald-400",
  danger: "text-red-600 dark:text-red-400",
  warning: "text-amber-600 dark:text-amber-400",
  info: "text-sky-600 dark:text-sky-400",
  merged: "text-purple-600 dark:text-purple-400",
  muted: "text-muted-foreground",
};

const STATUS_LABEL_KEYS: Record<Exclude<MRTaskSummaryStatus, "raw">, string> = {
  merged: "gitlab:merged",
  closed: "gitlab:closed",
  approved: "gitlab:approved",
  awaiting_approval: "gitlab:awaitingApproval",
  passed: "gitlab:passed",
  failed: "gitlab:failed",
  in_progress: "gitlab:inProgress",
  draft: "gitlab:draft",
  ready: "gitlab:readyToMerge",
  mergeable: "gitlab:mergeable",
  conflicts: "gitlab:conflicts",
  unresolved_discussions: "gitlab:unresolvedDiscussions",
};

const STATUS_ICONS: Record<MRTaskSummaryStatus, TablerIcon> = {
  merged: IconGitMerge,
  closed: IconX,
  approved: IconCheck,
  awaiting_approval: IconClockHour4,
  passed: IconCheck,
  failed: IconX,
  in_progress: IconClockHour4,
  draft: IconGitPullRequestDraft,
  ready: IconGitMerge,
  mergeable: IconGitMerge,
  conflicts: IconAlertTriangle,
  unresolved_discussions: IconAlertTriangle,
  raw: IconCircleDot,
};

function getStatusText(row: MRTaskSummaryRow, t: TFunction): string {
  if (row.status === "raw") return row.rawValue ?? "";
  return t(STATUS_LABEL_KEYS[row.status]);
}

function SummaryStatusIcon({ status }: { status: MRTaskSummaryStatus }) {
  const StatusIcon = STATUS_ICONS[status];
  return <StatusIcon aria-hidden="true" className="size-3.5 shrink-0" />;
}

function SummaryRow({ row }: { row: MRTaskSummaryRow }) {
  const { t } = useTranslation();
  return (
    <div
      data-testid={`mr-task-status-${row.kind}`}
      className="grid grid-cols-[min-content_minmax(0,1fr)] items-start gap-x-3"
    >
      <span className="text-muted-foreground">{t(ROW_LABEL_KEYS[row.kind])}</span>
      <span className={cn("flex min-w-0 items-center gap-1.5 font-medium", TONE_CLASSES[row.tone])}>
        <SummaryStatusIcon status={row.status} />
        <span className="min-w-0 break-words">{getStatusText(row, t)}</span>
      </span>
    </div>
  );
}

export function MRTaskStatusSummary({ summaries }: { summaries: MRTaskStatusSummaryData[] }) {
  const { t } = useTranslation();
  return (
    <div data-testid="mr-task-status-summary" className="w-full">
      {summaries.map((summary, index) => (
        <section
          key={`${summary.iid}-${index}`}
          data-testid="mr-task-status-entry"
          aria-label={t("gitlab:mergeRequestStatus", { iid: summary.iid })}
          className={cn(index > 0 && "mt-3 border-t border-border/60 pt-3")}
        >
          <div className="flex items-start gap-2">
            <IconGitMerge
              aria-hidden="true"
              className="mt-0.5 size-4 shrink-0 text-muted-foreground"
            />
            <div className="min-w-0 flex-1">
              <div
                data-testid="mr-task-status-iid"
                className="text-[11px] font-medium tabular-nums text-muted-foreground"
              >
                {t("gitlab:mrTaskStatusIid", { iid: summary.iid })}
              </div>
              <div
                data-testid="mr-task-status-title"
                className="text-pretty break-words text-sm font-medium leading-snug text-foreground [overflow-wrap:anywhere]"
              >
                {summary.title}
              </div>
            </div>
          </div>
          {summary.rows.length > 0 && (
            <div className="mt-2.5 space-y-1.5 pl-6">
              {summary.rows.map((row) => (
                <SummaryRow key={row.kind} row={row} />
              ))}
            </div>
          )}
        </section>
      ))}
    </div>
  );
}
