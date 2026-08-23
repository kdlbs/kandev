"use client";

import type { TaskPR } from "@/lib/types/github";
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

export function PRTaskStatusSummary({ summaries }: { summaries: PRTaskStatusSummaryData[] }) {
  return <ChangeRequestTaskStatusSummary summaries={summaries} />;
}
