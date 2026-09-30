import { describe, expect, it } from "vitest";
import { taskPRInfoFromSummary } from "./task-pr-info";
import type { TaskStatusSummary } from "./types/task-status-summary";

describe("taskPRInfoFromSummary workflow approval", () => {
  it("maps only an explicit approval flag from the bounded summary", () => {
    const approval = JSON.parse(
      '{"pull_request":{"number":42,"state":"open","workflow_approval_required":true}}',
    ) as TaskStatusSummary;
    const actionRequired = JSON.parse(
      '{"pull_request":{"number":43,"state":"open","attention":true,"aggregate_state":"failure"}}',
    ) as TaskStatusSummary;

    expect(taskPRInfoFromSummary(approval)?.workflowApprovalRequired).toBe(true);
    expect(taskPRInfoFromSummary(actionRequired)?.workflowApprovalRequired).not.toBe(true);
  });

  it("preserves explicit negative approval evidence and its freshness and disclosure identity", () => {
    const currentNegative = JSON.parse(
      '{"updated_at":"2026-09-30T12:00:00Z","pull_request":{"number":41,"state":"open","workflow_approval_required":false,"workflow_approval_pr_number":42,"workflow_approval_repository":"contributor/fork","workflow_approval_stale":true,"has_merge_conflicts":true,"merge_conflict_pr_number":43,"merge_conflict_repository":"org/repo"}}',
    ) as TaskStatusSummary;

    expect(taskPRInfoFromSummary(currentNegative)).toMatchObject({
      workflowApprovalRequired: false,
      workflowApprovalStale: true,
      workflowApprovalPRNumber: 42,
      workflowApprovalRepository: "contributor/fork",
      statusSummaryUpdatedAt: "2026-09-30T12:00:00Z",
      hasMergeConflicts: true,
      mergeConflictPRNumber: 43,
      mergeConflictRepository: "org/repo",
    });
  });
});
