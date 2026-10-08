import type { TaskStatusSummary } from "@/lib/types/task-status-summary";

export type TaskPRInfo = {
  number: number;
  state: string;
  count?: number;
  aggregateState?: string;
  autoFixEnabled?: boolean;
  autoMergeEnabled?: boolean;
  hasMergeConflicts?: boolean;
  workflowApprovalRequired?: boolean;
  workflowApprovalStale?: boolean;
  workflowApprovalPRNumber?: number;
  workflowApprovalRepository?: string;
  mergeConflictPRNumber?: number;
  mergeConflictRepository?: string;
  statusSummaryUpdatedAt?: string;
};

type TaskPRSummary = NonNullable<TaskStatusSummary["pull_request"]>;

function capitalize(value: string): string {
  return value.length > 0 ? value[0].toUpperCase() + value.slice(1) : value;
}

/** Map the bounded task-level PR projection to the shared task icon shape. */
export function taskPRInfoFromSummary(
  summary: TaskStatusSummary | null | undefined,
): TaskPRInfo | undefined {
  const pullRequest = summary?.pull_request;
  if (!pullRequest?.number) return undefined;
  return {
    number: pullRequest.number,
    state: capitalize(pullRequest.state ?? pullRequest.aggregate_state ?? "open"),
    ...(typeof pullRequest.count === "number" ? { count: pullRequest.count } : {}),
    aggregateState: pullRequest.aggregate_state,
    ...taskPRAutomationInfo(pullRequest),
    ...taskPRConflictInfo(pullRequest),
    ...taskPRWorkflowApprovalInfo(pullRequest, summary?.updated_at),
  };
}

function taskPRAutomationInfo(pullRequest: TaskPRSummary): Partial<TaskPRInfo> {
  return {
    ...(pullRequest.auto_fix_enabled ? { autoFixEnabled: true } : {}),
    ...(pullRequest.auto_merge_enabled ? { autoMergeEnabled: true } : {}),
  };
}

function taskPRConflictInfo(pullRequest: TaskPRSummary): Partial<TaskPRInfo> {
  return {
    ...(pullRequest.has_merge_conflicts ? { hasMergeConflicts: true } : {}),
    ...(pullRequest.merge_conflict_pr_number
      ? { mergeConflictPRNumber: pullRequest.merge_conflict_pr_number }
      : {}),
    ...(pullRequest.merge_conflict_repository
      ? { mergeConflictRepository: pullRequest.merge_conflict_repository }
      : {}),
  };
}

function taskPRWorkflowApprovalInfo(
  pullRequest: TaskPRSummary,
  updatedAt: string | undefined,
): Partial<TaskPRInfo> {
  return {
    ...(typeof pullRequest.workflow_approval_required === "boolean"
      ? { workflowApprovalRequired: pullRequest.workflow_approval_required }
      : {}),
    ...(pullRequest.workflow_approval_stale ? { workflowApprovalStale: true } : {}),
    ...(pullRequest.workflow_approval_pr_number
      ? { workflowApprovalPRNumber: pullRequest.workflow_approval_pr_number }
      : {}),
    ...(pullRequest.workflow_approval_repository
      ? { workflowApprovalRepository: pullRequest.workflow_approval_repository }
      : {}),
    ...(updatedAt ? { statusSummaryUpdatedAt: updatedAt } : {}),
  };
}
