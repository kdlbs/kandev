import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, renderHook } from "@testing-library/react";
import type { TaskIssueLink } from "@/lib/types/github";
import { issueKey, useIssueKeyToTasks } from "./use-issue-key-to-tasks";

const mocks = vi.hoisted(() => ({ data: {} as Record<string, TaskIssueLink> }));

vi.mock("./use-task-issues", () => ({
  useWorkspaceTaskIssues: () => ({ data: mocks.data }),
}));

afterEach(() => {
  cleanup();
  mocks.data = {};
});

function link(taskId: string, issueNumber = 1672): TaskIssueLink {
  return {
    task_id: taskId,
    task_title: `Task ${taskId}`,
    owner: "kdlbs",
    repo: "kandev",
    issue_number: issueNumber,
    issue_url: `https://github.com/kdlbs/kandev/issues/${issueNumber}`,
    issue_title: "",
  };
}

describe("useIssueKeyToTasks", () => {
  it("groups multiple tasks linked to the same issue", () => {
    mocks.data = { a: link("a"), b: link("b"), c: link("c", 2) };
    const { result } = renderHook(() => useIssueKeyToTasks("ws-1"));

    expect(result.current.get("kdlbs/kandev#1672")?.map((item) => item.task_id)).toEqual([
      "a",
      "b",
    ]);
    expect(result.current.get("kdlbs/kandev#2")?.[0]?.task_id).toBe("c");
  });

  it("returns no links when the workspace query has no data", () => {
    const { result } = renderHook(() => useIssueKeyToTasks("ws-2"));

    expect(result.current.size).toBe(0);
  });
});

describe("issueKey", () => {
  it("formats owner/repo#number", () => {
    expect(issueKey("kdlbs", "kandev", 1672)).toBe("kdlbs/kandev#1672");
  });
});
