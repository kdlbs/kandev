import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, renderHook, waitFor } from "@testing-library/react";
import { createElement, type ReactNode } from "react";
import { StateProvider, useAppStore } from "@/components/state-provider";
import type { TaskIssueLink } from "@/lib/types/github";
import { useWorkspaceTaskIssues } from "./use-task-issues";

const mocks = vi.hoisted(() => ({ listWorkspaceTaskIssues: vi.fn() }));

vi.mock("@/lib/api/domains/github-api", () => ({
  listWorkspaceTaskIssues: mocks.listWorkspaceTaskIssues,
}));

afterEach(() => {
  cleanup();
  mocks.listWorkspaceTaskIssues.mockReset();
});

function wrapper({ children }: { children: ReactNode }) {
  return createElement(StateProvider, null, children);
}

describe("useWorkspaceTaskIssues", () => {
  it("loads workspace links into the GitHub store", async () => {
    const link: TaskIssueLink = {
      task_id: "task-1",
      task_title: "Linked task",
      owner: "kdlbs",
      repo: "kandev",
      issue_number: 1672,
      issue_url: "https://github.com/kdlbs/kandev/issues/1672",
      issue_title: "",
    };
    mocks.listWorkspaceTaskIssues.mockResolvedValue({ task_issues: { "task-1": link } });

    const { result } = renderHook(
      () => {
        useWorkspaceTaskIssues("ws-1");
        return useAppStore((state) => state.taskIssues);
      },
      { wrapper },
    );

    await waitFor(() =>
      expect(result.current).toEqual({ workspaceId: "ws-1", byTaskId: { "task-1": link } }),
    );
    expect(mocks.listWorkspaceTaskIssues).toHaveBeenCalledWith("ws-1", {
      cache: "no-store",
    });
  });
});
