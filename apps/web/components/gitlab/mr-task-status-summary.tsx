"use client";

import { IconGitMerge } from "@tabler/icons-react";
import type { TaskMR } from "@/lib/types/gitlab";
import {
  ChangeRequestTaskStatusSummary,
  type ChangeRequestTaskStatusPresentation,
  type ChangeRequestTaskSummaryRow,
  type ChangeRequestTaskSummaryRowKind,
  type ChangeRequestTaskSummaryStatus,
  type ChangeRequestTaskSummaryTone,
} from "@/components/integrations/change-request-task-status-summary";

export type MRTaskSummaryRowKind = ChangeRequestTaskSummaryRowKind;
export type MRTaskSummaryTone = ChangeRequestTaskSummaryTone;
export type MRTaskSummaryStatus = ChangeRequestTaskSummaryStatus;
export type MRTaskSummaryRow = ChangeRequestTaskSummaryRow;

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

const MR_PRESENTATION: ChangeRequestTaskStatusPresentation = {
  summaryTestId: "mr-task-status-summary",
  entryTestId: "mr-task-status-entry",
  identifierTestId: "mr-task-status-iid",
  titleTestId: "mr-task-status-title",
  rowTestIdPrefix: "mr-task-status",
  icon: IconGitMerge,
  entryAriaLabelKey: "gitlab:mergeRequestStatus",
  identifierLabelKey: "gitlab:mrTaskStatusIid",
  rowLabelKeys: {
    state: "gitlab:mrTaskStatusState",
    review: "gitlab:mrTaskStatusReview",
    ci: "gitlab:mrTaskStatusCi",
    merge: "gitlab:mrTaskStatusMerge",
  },
  statusLabelKeys: {
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
  },
};

export function MRTaskStatusSummary({ summaries }: { summaries: MRTaskStatusSummaryData[] }) {
  return (
    <ChangeRequestTaskStatusSummary
      summaries={summaries.map((summary) => ({
        number: summary.iid,
        title: summary.title,
        rows: summary.rows,
      }))}
      presentation={MR_PRESENTATION}
    />
  );
}
