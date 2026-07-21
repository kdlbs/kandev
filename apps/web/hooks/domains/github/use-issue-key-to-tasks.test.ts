import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, renderHook } from "@testing-library/react";
import { createElement, type ReactNode } from "react";
import { StateProvider, useAppStore } from "@/components/state-provider";
import type { TaskIssueLink } from "@/lib/types/github";
import { issueKey, useIssueKeyToTasks } from "./use-issue-key-to-tasks";

vi.mock("./use-task-issues", () => ({ useWorkspaceTaskIssues: () => undefined }));

afterEach(() => cleanup());

function wrapper({ children }: { children: ReactNode }) {
  return createElement(StateProvider, null, children);
}

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
    const { result } = renderHook(
      () => ({
        map: useIssueKeyToTasks("ws-1"),
        setTaskIssues: useAppStore((state) => state.setTaskIssues),
      }),
      { wrapper },
    );

    act(() =>
      result.current.setTaskIssues("ws-1", { a: link("a"), b: link("b"), c: link("c", 2) }),
    );

    expect(result.current.map.get("kdlbs/kandev#1672")?.map((item) => item.task_id)).toEqual([
      "a",
      "b",
    ]);
    expect(result.current.map.get("kdlbs/kandev#2")?.[0]?.task_id).toBe("c");
  });

  it("does not expose links cached for another workspace", () => {
    const { result } = renderHook(
      () => ({
        map: useIssueKeyToTasks("ws-2"),
        setTaskIssues: useAppStore((state) => state.setTaskIssues),
      }),
      { wrapper },
    );

    act(() => result.current.setTaskIssues("ws-1", { a: link("a") }));

    expect(result.current.map.size).toBe(0);
  });
});

describe("issueKey", () => {
  it("formats owner/repo#number", () => {
    expect(issueKey("kdlbs", "kandev", 1672)).toBe("kdlbs/kandev#1672");
  });
});
