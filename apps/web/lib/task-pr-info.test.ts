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

describe("taskPRInfoFromSummary CI state", () => {
  it("maps explicit CI state with its representative identity", () => {
    const summary = JSON.parse(
      '{"updated_at":"2026-11-10T12:00:00Z","pull_request":{"number":42,"state":"open","ci_state":"failed","ci_pr_number":43,"ci_repository":"acme/widget","ci_stale":true}}',
    ) as TaskStatusSummary;

    expect(taskPRInfoFromSummary(summary)).toMatchObject({
      ciState: "failed",
      ciPRNumber: 43,
      ciRepository: "acme/widget",
      ciStale: true,
      statusSummaryUpdatedAt: "2026-11-10T12:00:00Z",
    });
  });

  it("keeps an explicit none distinct from legacy omission", () => {
    const explicitNone = JSON.parse(
      '{"pull_request":{"number":42,"state":"open","ci_state":"none"}}',
    ) as TaskStatusSummary;
    const legacy = JSON.parse('{"pull_request":{"number":42,"state":"open"}}') as TaskStatusSummary;

    expect(taskPRInfoFromSummary(explicitNone)?.ciState).toBe("none");
    expect(taskPRInfoFromSummary(legacy)?.ciState).toBeUndefined();
  });

  it("ignores unsupported CI values instead of inventing a bubble", () => {
    const summary = JSON.parse(
      '{"pull_request":{"number":42,"state":"open","ci_state":"exploded"}}',
    ) as TaskStatusSummary;

    expect(taskPRInfoFromSummary(summary)?.ciState).toBeUndefined();
  });
});
