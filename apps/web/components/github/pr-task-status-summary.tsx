"use client";

import type { TaskPR } from "@/lib/types/github";
import { useTranslation } from "react-i18next";
import {
  getMergeQueueSummaryDetail,
  getMergeQueueSummaryStatus,
  hasActiveMergeQueueEntry,
} from "./pr-merge-queue-status";
import {
  ChangeRequestTaskStatusSummary,
  type ChangeRequestTaskStatusSummaryData,
  type ChangeRequestTaskSummaryRow,
  type ChangeRequestTaskSummaryRowKind,
  type ChangeRequestTaskSummaryStatus,
  type ChangeRequestTaskSummaryTone,
} from "@/components/integrations/change-request-task-status-summary";
import {
  getCurrentWorkflowAttention,
  isWorkflowApprovalRequired,
  workflowAttentionWorkflowNames,
} from "./pr-workflow-attention";
import { isAllSkippedCheckSummary, skippedCheckCount } from "./pr-ci-state";

export type PRTaskSummaryRowKind = ChangeRequestTaskSummaryRowKind;
export type PRTaskSummaryTone = ChangeRequestTaskSummaryTone;
export type PRTaskSummaryStatus = ChangeRequestTaskSummaryStatus;
export type PRTaskSummaryRow = ChangeRequestTaskSummaryRow;
export type PRTaskStatusSummaryData = ChangeRequestTaskStatusSummaryData;

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

function workflowAttentionRow(pr: TaskPR): PRTaskSummaryRow | null {
  const attention = getCurrentWorkflowAttention(pr);
  if (!attention) return null;
  if (attention.state === "unknown") {
    return {
      kind: "ci",
      id: "workflow-attention",
      status: "workflow_unavailable",
      tone: "muted",
    };
  }
  if (attention.state !== "approval_required" && attention.state !== "action_required") {
    return null;
  }
  const names = workflowAttentionWorkflowNames(attention);
  return {
    kind: "ci",
    id: "workflow-attention",
    status: isWorkflowApprovalRequired(attention) ? "awaiting_approval" : "workflow_attention",
    tone: "warning",
    ...(names
      ? {
          detail: {
            key: "github:workflowAttentionWorkflows",
            values: { names },
          },
        }
      : {}),
  };
}

function skippedDetail(pr: TaskPR): PRTaskSummaryRow["detail"] | undefined {
  const skipped = skippedCheckCount(pr);
  if (skipped <= 0) return undefined;
  return { key: "github:checksSkippedCount", values: { count: skipped } };
}

function mixedSkippedDetail(pr: TaskPR): PRTaskSummaryRow["detail"] | undefined {
  if (isAllSkippedCheckSummary(pr)) return undefined;
  return skippedDetail(pr);
}

function allSkippedHistoryDetail(pr: TaskPR): PRTaskSummaryRow["detail"] | undefined {
  if (!isAllSkippedCheckSummary(pr)) return undefined;
  return { key: "github:lastDetailedObservationAllSkipped" };
}

function withSkippedDetail(
  row: PRTaskSummaryRow,
  detail: PRTaskSummaryRow["detail"] | undefined,
): PRTaskSummaryRow {
  return detail ? { ...row, detail } : row;
}

function primaryCIRow(
  pr: TaskPR,
  attentionExplainsUnstable: boolean,
  mixedDetail: PRTaskSummaryRow["detail"] | undefined,
): PRTaskSummaryRow | null {
  const historyDetail = allSkippedHistoryDetail(pr);
  if (pr.checks_state === "failure") {
    return withSkippedDetail(
      { kind: "ci", status: "failed", tone: "danger" },
      historyDetail ?? mixedDetail,
    );
  }
  if (pr.checks_state === "pending") {
    return withSkippedDetail(
      { kind: "ci", status: "in_progress", tone: "warning" },
      historyDetail ?? mixedDetail,
    );
  }
  if (pr.checks_state === "unstable") {
    if (attentionExplainsUnstable) return null;
    return withSkippedDetail(
      { kind: "ci", status: "checks_not_successful", tone: "warning" },
      historyDetail ?? mixedDetail,
    );
  }
  if (pr.checks_state === "success") {
    return withSkippedDetail({ kind: "ci", status: "passed", tone: "success" }, mixedDetail);
  }
  if (pr.mergeable_state === "unstable") {
    if (attentionExplainsUnstable) return null;
    return withSkippedDetail(
      { kind: "ci", status: "checks_not_successful", tone: "warning" },
      historyDetail ?? mixedDetail,
    );
  }
  if (isAllSkippedCheckSummary(pr)) {
    return {
      kind: "ci",
      status: "all_skipped",
      tone: "muted",
      detail: { key: "github:lastDetailedObservation" },
    };
  }
  if (!pr.checks_state) return null;
  return withSkippedDetail(rawRow("ci", pr.checks_state), historyDetail);
}

function deriveCIRows(pr: TaskPR): PRTaskSummaryRow[] {
  const attentionRow = workflowAttentionRow(pr);
  const attentionExplainsUnstable =
    attentionRow?.status === "awaiting_approval" || attentionRow?.status === "workflow_attention";
  const row = primaryCIRow(pr, attentionExplainsUnstable, mixedSkippedDetail(pr));
  return [...(row ? [row] : []), ...(attentionRow ? [attentionRow] : [])];
}

function deriveMergeRow(pr: TaskPR, readyToMerge: boolean): PRTaskSummaryRow | null {
  if (pr.state !== "open") return null;
  if (hasActiveMergeQueueEntry(pr)) {
    return {
      kind: "merge",
      status: getMergeQueueSummaryStatus(pr.merge_queue_state),
      tone: "queued",
      detail: getMergeQueueSummaryDetail(
        pr.merge_queue_position,
        pr.merge_queue_estimated_time_to_merge_seconds,
      ),
    };
  }
  if (pr.has_merge_conflicts ?? pr.mergeable_state === "dirty") {
    return { kind: "merge", status: "conflicts", tone: "danger" };
  }
  if (pr.mergeable_state === "draft") {
    return { kind: "merge", status: "draft", tone: "muted" };
  }
  if (readyToMerge) return { kind: "merge", status: "ready", tone: "success" };
  if (!pr.mergeable_state || pr.mergeable_state === "unknown") return null;
  if (pr.mergeable_state === "behind") {
    return { kind: "merge", status: "behind", tone: "warning" };
  }
  if (pr.mergeable_state === "blocked") {
    return { kind: "merge", status: "blocked", tone: "muted" };
  }
  if (pr.mergeable_state === "clean") {
    return { kind: "merge", status: "mergeable", tone: "muted" };
  }
  if (pr.mergeable_state === "unstable") return null;
  return rawRow("merge", pr.mergeable_state);
}

export function derivePRTaskStatusSummary(
  pr: TaskPR,
  readyToMerge: boolean,
): PRTaskStatusSummaryData {
  const rows = [
    deriveStateRow(pr.state),
    deriveReviewRow(pr.review_state),
    ...deriveCIRows(pr),
    deriveMergeRow(pr, readyToMerge),
  ].filter((row): row is PRTaskSummaryRow => row !== null);

  const author = pr.author_login.trim();
  return {
    number: pr.pr_number,
    title: pr.pr_title,
    ...(author ? { author } : {}),
    rows,
  };
}

export type StaleWorkflowAttentionPR = {
  number: number;
  repository?: string;
};

export function PRTaskStatusSummary({
  summaries,
  staleWorkflowPRs = [],
}: {
  summaries: PRTaskStatusSummaryData[];
  staleWorkflowPRs?: StaleWorkflowAttentionPR[];
}) {
  const { t } = useTranslation();
  return (
    <>
      <ChangeRequestTaskStatusSummary summaries={summaries} />
      {staleWorkflowPRs.length > 0 && (
        <div className="mt-2 space-y-1.5 border-t border-border/60 pt-2">
          {staleWorkflowPRs.map(({ number, repository }) => (
            <p
              key={`${repository ?? ""}-${number}`}
              data-testid="pr-task-stale-workflow-evidence"
              className="flex flex-wrap gap-x-1 text-[11px] leading-snug text-muted-foreground"
            >
              <span className="font-medium">
                {repository
                  ? t("github:prTaskStatusRepositoryNumber", { repository, number })
                  : t("github:prTaskStatusNumber", { number })}
              </span>
              <span>{t("github:workflowAttentionStale")}</span>
            </p>
          ))}
        </div>
      )}
    </>
  );
}
