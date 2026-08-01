import { describe, expect, it } from "vitest";
import type { RecentTaskEntry } from "./recent-tasks";
import { isExplicitHomeDestination, resolveStartupTaskId } from "./startup-page";

const WORKSPACE_ID = "workspace-1";

function recentTask(taskId: string, workspaceId: string): RecentTaskEntry {
  return {
    taskId,
    title: taskId,
    visitedAt: "2026-07-31T10:00:00Z",
    workspaceId,
  };
}

describe("startup page resolution", () => {
  it("resumes the newest recent task in the active workspace", () => {
    expect(
      resolveStartupTaskId({
        startupPage: "last_task",
        workspaceId: WORKSPACE_ID,
        recentTasks: [recentTask("newest", WORKSPACE_ID), recentTask("older", WORKSPACE_ID)],
        hasExplicitDestination: false,
      }),
    ).toBe("newest");
  });

  it("skips newer recent tasks from a different workspace", () => {
    expect(
      resolveStartupTaskId({
        startupPage: "last_task",
        workspaceId: WORKSPACE_ID,
        recentTasks: [recentTask("foreign", "workspace-2"), recentTask("local", WORKSPACE_ID)],
        hasExplicitDestination: false,
      }),
    ).toBe("local");
  });

  it("falls back when there is no eligible task or the overview was explicit", () => {
    expect(
      resolveStartupTaskId({
        startupPage: "last_task",
        workspaceId: WORKSPACE_ID,
        recentTasks: [recentTask("foreign", "workspace-2")],
        hasExplicitDestination: false,
      }),
    ).toBeNull();

    expect(
      resolveStartupTaskId({
        startupPage: "last_task",
        workspaceId: WORKSPACE_ID,
        recentTasks: [recentTask("local", WORKSPACE_ID)],
        hasExplicitDestination: true,
      }),
    ).toBeNull();
  });

  it("recognizes explicit task, workflow, and overview intent", () => {
    expect(isExplicitHomeDestination(new URLSearchParams("home=overview"))).toBe(true);
    expect(isExplicitHomeDestination(new URLSearchParams("workflowId=workflow-1"))).toBe(true);
    expect(isExplicitHomeDestination(new URLSearchParams("taskId=task-1"))).toBe(true);
    expect(isExplicitHomeDestination(new URLSearchParams("workspaceId=workspace-1"))).toBe(false);
  });
});
