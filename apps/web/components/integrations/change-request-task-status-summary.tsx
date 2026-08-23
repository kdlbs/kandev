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

export type ChangeRequestTaskSummaryRowKind = "state" | "review" | "ci" | "merge";
export type ChangeRequestTaskSummaryTone =
  | "success"
  | "danger"
  | "warning"
  | "info"
  | "merged"
  | "queued"
  | "muted";
export type ChangeRequestTaskSummaryStatus =
  | "merged"
  | "closed"
  | "approved"
  | "changes_requested"
  | "pending_review"
  | "awaiting_approval"
  | "passed"
  | "failed"
  | "in_progress"
  | "draft"
  | "ready"
  | "conflicts"
  | "behind"
  | "blocked"
  | "unresolved_discussions"
  | "mergeable"
  | "queue_queued"
  | "queue_awaiting_checks"
  | "queue_mergeable"
  | "queue_unmergeable"
  | "queue_locked"
  | "raw";

export type ChangeRequestTaskSummaryRowDetail = {
  key: string;
  values?: Record<string, unknown>;
};

export type ChangeRequestTaskSummaryRow = {
  kind: ChangeRequestTaskSummaryRowKind;
  status: ChangeRequestTaskSummaryStatus;
  tone: ChangeRequestTaskSummaryTone;
  rawValue?: string;
  detail?: ChangeRequestTaskSummaryRowDetail;
};

export type ChangeRequestTaskStatusSummaryData = {
  number: number | string;
  title: string;
  rows: ChangeRequestTaskSummaryRow[];
};

export type ChangeRequestTaskStatusPresentation = {
  summaryTestId: string;
  entryTestId: string;
  identifierTestId: string;
  titleTestId: string;
  rowTestIdPrefix: string;
  icon: TablerIcon;
  entryAriaLabelKey: string;
  identifierLabelKey: string;
  rowLabelKeys: Record<ChangeRequestTaskSummaryRowKind, string>;
  statusLabelKeys?: Partial<Record<Exclude<ChangeRequestTaskSummaryStatus, "raw">, string>>;
};

const ROW_LABEL_KEYS: Record<ChangeRequestTaskSummaryRowKind, string> = {
  state: "github:prTaskStatusState",
  review: "github:prTaskStatusReview",
  ci: "github:prTaskStatusCi",
  merge: "github:prTaskStatusMerge",
};

const TONE_CLASSES: Record<ChangeRequestTaskSummaryTone, string> = {
  success: "text-emerald-600 dark:text-emerald-400",
  danger: "text-red-600 dark:text-red-400",
  warning: "text-amber-600 dark:text-amber-400",
  info: "text-sky-600 dark:text-sky-400",
  merged: "text-purple-600 dark:text-purple-400",
  queued: "text-[#966600]",
  muted: "text-muted-foreground",
};

const STATUS_LABEL_KEYS: Record<Exclude<ChangeRequestTaskSummaryStatus, "raw">, string> = {
  merged: "github:merged",
  closed: "github:closed",
  approved: "github:approved",
  changes_requested: "github:changesRequested",
  pending_review: "github:pendingReview",
  awaiting_approval: "github:pendingReview",
  passed: "github:checkBucketPassed",
  failed: "github:checkBucketFailed",
  in_progress: "github:checkBucketInProgress",
  draft: "github:draft",
  ready: "github:readyToMerge",
  conflicts: "github:conflicts",
  behind: "github:behindBase",
  blocked: "github:blocked",
  unresolved_discussions: "github:blocked",
  mergeable: "github:mergeable",
  queue_queued: "github:mergeQueueStateQueued",
  queue_awaiting_checks: "github:mergeQueueStateAwaitingChecks",
  queue_mergeable: "github:mergeQueueStateMergeable",
  queue_unmergeable: "github:mergeQueueStateUnmergeable",
  queue_locked: "github:mergeQueueStateLocked",
};

const STATUS_ICONS: Record<ChangeRequestTaskSummaryStatus, TablerIcon> = {
  merged: IconGitMerge,
  closed: IconX,
  approved: IconCheck,
  changes_requested: IconX,
  pending_review: IconClockHour4,
  awaiting_approval: IconClockHour4,
  passed: IconCheck,
  failed: IconX,
  in_progress: IconClockHour4,
  draft: IconGitPullRequestDraft,
  ready: IconGitMerge,
  conflicts: IconAlertTriangle,
  behind: IconAlertTriangle,
  blocked: IconAlertTriangle,
  unresolved_discussions: IconAlertTriangle,
  mergeable: IconGitMerge,
  queue_queued: IconClockHour4,
  queue_awaiting_checks: IconClockHour4,
  queue_mergeable: IconClockHour4,
  queue_unmergeable: IconClockHour4,
  queue_locked: IconClockHour4,
  raw: IconCircleDot,
};

const DEFAULT_PRESENTATION: ChangeRequestTaskStatusPresentation = {
  summaryTestId: "pr-task-status-summary",
  entryTestId: "pr-task-status-entry",
  identifierTestId: "pr-task-status-number",
  titleTestId: "pr-task-status-title",
  rowTestIdPrefix: "pr-task-status",
  icon: IconGitPullRequest,
  entryAriaLabelKey: "github:pullRequestStatus",
  identifierLabelKey: "github:prTaskStatusNumber",
  rowLabelKeys: ROW_LABEL_KEYS,
};

function getStatusText(
  row: ChangeRequestTaskSummaryRow,
  presentation: ChangeRequestTaskStatusPresentation,
  t: TFunction,
): string {
  if (row.status === "raw") return row.rawValue ?? "";
  return t(presentation.statusLabelKeys?.[row.status] ?? STATUS_LABEL_KEYS[row.status]);
}

function getDetailText(detail: ChangeRequestTaskSummaryRowDetail, t: TFunction): string {
  return t(detail.key, detail.values);
}

function SummaryStatusIcon({ status }: { status: ChangeRequestTaskSummaryStatus }) {
  const StatusIcon = STATUS_ICONS[status];
  return <StatusIcon aria-hidden="true" className="size-3.5 shrink-0" />;
}

function SummaryRow({
  row,
  presentation,
}: {
  row: ChangeRequestTaskSummaryRow;
  presentation: ChangeRequestTaskStatusPresentation;
}) {
  const { t } = useTranslation();
  return (
    <div
      data-testid={`${presentation.rowTestIdPrefix}-${row.kind}`}
      className="grid grid-cols-[min-content_minmax(0,1fr)] items-start gap-x-3"
    >
      <span className="text-muted-foreground">{t(presentation.rowLabelKeys[row.kind])}</span>
      <div className="min-w-0">
        <span
          className={cn("flex min-w-0 items-center gap-1.5 font-medium", TONE_CLASSES[row.tone])}
        >
          <SummaryStatusIcon status={row.status} />
          <span className="min-w-0 break-words">{getStatusText(row, presentation, t)}</span>
        </span>
        {row.detail && (
          <span
            data-testid={presentation.rowTestIdPrefix + "-" + row.kind + "-detail"}
            className="mt-0.5 block text-[11px] leading-snug text-muted-foreground"
          >
            {getDetailText(row.detail, t)}
          </span>
        )}
      </div>
    </div>
  );
}

export function ChangeRequestTaskStatusSummary({
  summaries,
  presentation = DEFAULT_PRESENTATION,
}: {
  summaries: ChangeRequestTaskStatusSummaryData[];
  presentation?: ChangeRequestTaskStatusPresentation;
}) {
  const { t } = useTranslation();
  const ProviderIcon = presentation.icon;
  return (
    <div data-testid={presentation.summaryTestId} className="w-full">
      {summaries.map((summary, index) => (
        <section
          key={`${summary.number}-${index}`}
          data-testid={presentation.entryTestId}
          aria-label={t(presentation.entryAriaLabelKey, {
            number: summary.number,
            iid: summary.number,
          })}
          className={cn(index > 0 && "mt-3 border-t border-border/60 pt-3")}
        >
          <div className="flex items-start gap-2">
            <ProviderIcon
              aria-hidden="true"
              className="mt-0.5 size-4 shrink-0 text-muted-foreground"
            />
            <div className="min-w-0 flex-1">
              <div
                data-testid={presentation.identifierTestId}
                className="text-[11px] font-medium tabular-nums text-muted-foreground"
              >
                {t(presentation.identifierLabelKey, {
                  number: summary.number,
                  iid: summary.number,
                })}
              </div>
              <div
                data-testid={presentation.titleTestId}
                className="text-pretty break-words text-sm font-medium leading-snug text-foreground [overflow-wrap:anywhere]"
              >
                {summary.title}
              </div>
            </div>
          </div>
          {summary.rows.length > 0 && (
            <div className="mt-2.5 space-y-1.5 pl-6">
              {summary.rows.map((row) => (
                <SummaryRow key={row.kind} row={row} presentation={presentation} />
              ))}
            </div>
          )}
        </section>
      ))}
    </div>
  );
}
