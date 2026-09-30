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
});
