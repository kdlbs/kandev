"use client";

import {
  IconAlertTriangle,
  IconCheck,
  IconCircleDot,
  IconClockHour4,
  IconGitMerge,
  IconGitPullRequest,
  IconGitPullRequestDraft,
  IconX,
  type Icon as TablerIcon,
} from "@tabler/icons-react";
import type { TFunction } from "i18next";
import { useTranslation } from "react-i18next";
import { cn } from "@/lib/utils";
import type { TaskPR } from "@/lib/types/github";

export type PRTaskSummaryRowKind = "state" | "review" | "ci" | "merge";
export type PRTaskSummaryTone = "success" | "danger" | "warning" | "info" | "merged" | "muted";
export type PRTaskSummaryStatus =
  | "merged"
  | "closed"
  | "approved"
  | "changes_requested"
  | "pending_review"
  | "passed"
  | "failed"
  | "in_progress"
  | "draft"
  | "ready"
  | "conflicts"
  | "behind"
  | "blocked"
  | "mergeable"
  | "raw";

export type PRTaskSummaryRow = {
  kind: PRTaskSummaryRowKind;
  status: PRTaskSummaryStatus;
  tone: PRTaskSummaryTone;
  rawValue?: string;
};

export type PRTaskStatusSummaryData = {
  number: number;
  title: string;
  rows: PRTaskSummaryRow[];
};

function rawRow(kind: PRTaskSummaryRowKind, rawValue: string): PRTaskSummaryRow {
  return { kind, status: "raw", tone: "muted", rawValue };
}

function deriveStateRow(state: string): PRTaskSummaryRow | null {
  if (!state || state === "open") return null;
  if (state === "merged") return { kind: "state", status: "merged", tone: "merged" };
  if (state === "closed") return { kind: "state", status: "closed", tone: "danger" };
  return rawRow("state", state);
}

function deriveReviewRow(reviewState: string): PRTaskSummaryRow | null {
  if (!reviewState) return null;
  if (reviewState === "approved") {
    return { kind: "review", status: "approved", tone: "success" };
  }
  if (reviewState === "changes_requested") {
    return { kind: "review", status: "changes_requested", tone: "danger" };
  }
  if (reviewState === "pending") {
    return { kind: "review", status: "pending_review", tone: "info" };
  }
  return rawRow("review", reviewState);
}

function deriveCIRow(checksState: string): PRTaskSummaryRow | null {
  if (!checksState) return null;
  if (checksState === "success") return { kind: "ci", status: "passed", tone: "success" };
  if (checksState === "failure") return { kind: "ci", status: "failed", tone: "danger" };
  if (checksState === "pending") {
    return { kind: "ci", status: "in_progress", tone: "warning" };
  }
  return rawRow("ci", checksState);
}

function deriveMergeRow(pr: TaskPR, readyToMerge: boolean): PRTaskSummaryRow | null {
  if (pr.state !== "open") return null;
  if (pr.mergeable_state === "draft") {
    return { kind: "merge", status: "draft", tone: "muted" };
  }
  if (readyToMerge) return { kind: "merge", status: "ready", tone: "success" };
  if (!pr.mergeable_state || pr.mergeable_state === "unknown") return null;
  if (pr.mergeable_state === "dirty") {
    return { kind: "merge", status: "conflicts", tone: "danger" };
  }
  if (pr.mergeable_state === "behind") {
    return { kind: "merge", status: "behind", tone: "warning" };
  }
  if (pr.mergeable_state === "blocked") {
    return { kind: "merge", status: "blocked", tone: "muted" };
  }
  if (pr.mergeable_state === "clean") {
    return { kind: "merge", status: "mergeable", tone: "muted" };
  }
  return rawRow("merge", pr.mergeable_state);
}

export function derivePRTaskStatusSummary(
  pr: TaskPR,
  readyToMerge: boolean,
): PRTaskStatusSummaryData {
  const rows = [
    deriveStateRow(pr.state),
    deriveReviewRow(pr.review_state),
    deriveCIRow(pr.checks_state),
    deriveMergeRow(pr, readyToMerge),
  ].filter((row): row is PRTaskSummaryRow => row !== null);

  return { number: pr.pr_number, title: pr.pr_title, rows };
}

const ROW_LABEL_KEYS: Record<PRTaskSummaryRowKind, string> = {
  state: "github:prTaskStatusState",
  review: "github:prTaskStatusReview",
  ci: "github:prTaskStatusCi",
  merge: "github:prTaskStatusMerge",
};

const TONE_CLASSES: Record<PRTaskSummaryTone, string> = {
  success: "text-emerald-600 dark:text-emerald-400",
  danger: "text-red-600 dark:text-red-400",
  warning: "text-amber-600 dark:text-amber-400",
  info: "text-sky-600 dark:text-sky-400",
  merged: "text-purple-600 dark:text-purple-400",
  muted: "text-muted-foreground",
};

const STATUS_LABEL_KEYS: Record<Exclude<PRTaskSummaryStatus, "raw">, string> = {
  merged: "github:merged",
  closed: "github:closed",
  approved: "github:approved",
  changes_requested: "github:changesRequested",
  pending_review: "github:pendingReview",
  passed: "github:checkBucketPassed",
  failed: "github:checkBucketFailed",
  in_progress: "github:checkBucketInProgress",
  draft: "github:draft",
  ready: "github:readyToMerge",
  conflicts: "github:conflicts",
  behind: "github:behindBase",
  blocked: "github:blocked",
  mergeable: "github:mergeable",
};

const STATUS_ICONS: Record<PRTaskSummaryStatus, TablerIcon> = {
  merged: IconGitMerge,
  closed: IconX,
  approved: IconCheck,
  changes_requested: IconX,
  pending_review: IconClockHour4,
  passed: IconCheck,
  failed: IconX,
  in_progress: IconClockHour4,
  draft: IconGitPullRequestDraft,
  ready: IconGitMerge,
  conflicts: IconAlertTriangle,
  behind: IconAlertTriangle,
  blocked: IconAlertTriangle,
  mergeable: IconGitMerge,
  raw: IconCircleDot,
};

function getStatusText(row: PRTaskSummaryRow, t: TFunction): string {
  if (row.status === "raw") return row.rawValue ?? "";
  return t(STATUS_LABEL_KEYS[row.status]);
}

function SummaryStatusIcon({ status }: { status: PRTaskSummaryStatus }) {
  const StatusIcon = STATUS_ICONS[status];
  return <StatusIcon aria-hidden="true" className="size-3.5 shrink-0" />;
}

function SummaryRow({ row }: { row: PRTaskSummaryRow }) {
  const { t } = useTranslation();
  return (
    <div
      data-testid={`pr-task-status-${row.kind}`}
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

export function PRTaskStatusSummary({ summaries }: { summaries: PRTaskStatusSummaryData[] }) {
  const { t } = useTranslation();
  return (
    <div data-testid="pr-task-status-summary" className="w-full">
      {summaries.map((summary, index) => (
        <section
          key={`${summary.number}-${index}`}
          data-testid="pr-task-status-entry"
          aria-label={t("github:pullRequestStatus", { number: summary.number })}
          className={cn(index > 0 && "mt-3 border-t border-border/60 pt-3")}
        >
          <div className="flex items-start gap-2">
            <IconGitPullRequest
              aria-hidden="true"
              className="mt-0.5 size-4 shrink-0 text-muted-foreground"
            />
            <div className="min-w-0 flex-1">
              <div
                data-testid="pr-task-status-number"
                className="text-[11px] font-medium tabular-nums text-muted-foreground"
              >
                {t("github:prTaskStatusNumber", { number: summary.number })}
              </div>
              <div
                data-testid="pr-task-status-title"
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
