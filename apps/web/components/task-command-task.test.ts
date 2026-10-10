import { describe, expect, it } from "vitest";
import type { Task } from "@/lib/types/http";
import { taskCommandItemFromDetail } from "./task-command-task";

describe("taskCommandItemFromDetail", () => {
  it("retains the full task context needed for commands when its sidebar row is off-page", () => {
    const task = {
      id: "archived-task",
      workspace_id: "workspace",
      workflow_id: "workflow",
      workflow_step_id: "step",
      position: 1,
      title: "Archived task",
      description: "Description",
      created_at: "2026-09-25T12:00:00Z",
      updated_at: "2026-09-26T12:00:00Z",
      state: "COMPLETED",
      priority: "high",
      archived_at: "2026-09-26T12:00:00Z",
      repositories: [{ repository_id: "repository" }],
      parent_id: "parent",
      metadata: {
        issue_watch_id: "watch",
        issue_url: "https://example.test/issue/4",
        issue_number: 4,
      },
    } as unknown as Task;

    expect(taskCommandItemFromDetail(task)).toMatchObject({
      id: "archived-task",
      title: "Archived task",
      workspaceId: "workspace",
      workflowId: "workflow",
      workflowStepId: "step",
      isArchived: true,
      parentTaskId: "parent",
      repositoryLinks: [{ repository_id: "repository" }],
      isIssueWatch: true,
      issueInfo: { url: "https://example.test/issue/4", number: 4 },
    });
  });
});
