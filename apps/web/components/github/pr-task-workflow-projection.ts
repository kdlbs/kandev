import type { useTranslation } from "react-i18next";
import type { TaskPR } from "@/lib/types/github";
import { parseStrictRfc3339Timestamp } from "@/lib/utils/strict-timestamp";
import type { TaskPRInfo } from "./pr-task-automation";
import { derivePRTaskStatusSummary } from "./pr-task-status-summary";

type PRTaskStatusSummaryData = ReturnType<typeof derivePRTaskStatusSummary>;
export type ProjectedPRTaskStatusSummary = Omit<PRTaskStatusSummaryData, "number"> & {
  number: number;
};

function compactPRLifecycleLabel(
  state: string,
  t: ReturnType<typeof useTranslation>["t"],
): string | null {
  switch (state.toLowerCase()) {
    case "merged":
      return t("github:merged");
    case "closed":
      return t("github:closed");
    case "draft":
      return t("github:draft");
    case "open":
      return t("common:open");
    default:
      return null;
  }
}

function compactPRAggregateLabel(
  state: string | undefined,
  t: ReturnType<typeof useTranslation>["t"],
): string | null {
  switch (state?.toLowerCase()) {
    case "failure":
      return t("github:needsAttention");
    case "pending":
      return t("common:pending");
    case "awaiting_review":
      return t("github:pendingReview");
    case "blocked":
      return t("github:blocked");
    case "ready":
      return null;
    case "queued":
      return t("github:mergeQueueStateQueued");
    case "passing":
      return t("github:checksPassed");
    case "draft":
      return t("github:draft");
    case "merged":
      return t("github:merged");
    case "closed":
      return t("github:closed");
    default:
      return null;
  }
}

export function getCompactPRStatusAccessibleLabels(
  prInfo: TaskPRInfo,
  t: ReturnType<typeof useTranslation>["t"],
): string[] {
  return [
    compactPRLifecycleLabel(prInfo.state, t),
    compactPRAggregateLabel(prInfo.aggregateState, t),
    prInfo.workflowApprovalRequired ? t("github:workflowAwaitingApproval") : null,
  ]
    .filter((label): label is string => label !== null)
    .filter((label, index, labels) => labels.indexOf(label) === index);
}

export function compactWorkflowApprovalIsNewerThanFullPRs(
  prs: TaskPR[],
  prInfo?: TaskPRInfo,
): boolean {
  if (!prInfo || typeof prInfo.workflowApprovalRequired !== "boolean" || prs.length === 0) {
    return false;
  }
  const summaryUpdatedAt = parseStrictRfc3339Timestamp(prInfo.statusSummaryUpdatedAt);
  if (summaryUpdatedAt === null) return false;
  const fullPRFreshness = prs.map((pr) => {
    const syncedAt = parseStrictRfc3339Timestamp(pr.last_synced_at ?? undefined);
    if (syncedAt === null) return null;
    const attentionAt = parseStrictRfc3339Timestamp(pr.workflow_attention?.observed_at);
    return attentionAt !== null && attentionAt > syncedAt ? attentionAt : syncedAt;
  });
  return fullPRFreshness.every((updatedAt) => updatedAt !== null && summaryUpdatedAt > updatedAt);
}

export function getCompactWorkflowStatusSummaries(
  prInfo: TaskPRInfo,
): ProjectedPRTaskStatusSummary[] {
  const summaries = new Map<string, ProjectedPRTaskStatusSummary>();
  const addRow = (
    number: number,
    repository: string | undefined,
    row: PRTaskStatusSummaryData["rows"][number],
  ) => {
    const key = `${repository ?? ""}#${number}`;
    const summary = summaries.get(key) ?? { number, title: "", rows: [] };
    const detail = repository
      ? {
          key: "github:prTaskStatusRepositoryNumber",
          values: { repository, number },
        }
      : undefined;
    summary.rows.push({ ...row, ...(detail ? { detail } : {}) });
    summaries.set(key, summary);
  };

  if (prInfo.workflowApprovalRequired === true) {
    addRow(prInfo.workflowApprovalPRNumber ?? prInfo.number, prInfo.workflowApprovalRepository, {
      kind: "ci",
      id: "workflow-attention",
      status: "awaiting_approval",
      tone: "warning",
    });
  }
  if (prInfo.hasMergeConflicts === true) {
    addRow(prInfo.mergeConflictPRNumber ?? prInfo.number, prInfo.mergeConflictRepository, {
      kind: "merge",
      id: "merge-conflict",
      status: "conflicts",
      tone: "danger",
    });
  }
  return [...summaries.values()];
}

export function getCompactStaleWorkflowPRs(prInfo: TaskPRInfo) {
  if (prInfo.workflowApprovalRequired !== true || !prInfo.workflowApprovalStale) return [];
  return [
    {
      number: prInfo.workflowApprovalPRNumber ?? prInfo.number,
      repository: prInfo.workflowApprovalRepository,
    },
  ];
}

export function getProjectedPRRepository(prInfo: TaskPRInfo | undefined, number: number) {
  if (!prInfo) return undefined;
  if (
    prInfo.workflowApprovalRequired === true &&
    (prInfo.workflowApprovalPRNumber ?? prInfo.number) === number
  ) {
    return prInfo.workflowApprovalRepository;
  }
  if (
    prInfo.hasMergeConflicts === true &&
    (prInfo.mergeConflictPRNumber ?? prInfo.number) === number
  ) {
    return prInfo.mergeConflictRepository;
  }
  return undefined;
}
