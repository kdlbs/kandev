import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, renderHook, waitFor } from "@testing-library/react";
import { createElement, type ReactNode, useState } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
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
  const [queryClient] = useState(
    () => new QueryClient({ defaultOptions: { queries: { retry: false } } }),
  );
  return createElement(QueryClientProvider, { client: queryClient }, children);
}

describe("useWorkspaceTaskIssues", () => {
  it("loads workspace links into the Query cache", async () => {
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

    const { result } = renderHook(() => useWorkspaceTaskIssues("ws-1"), { wrapper });

    await waitFor(() => expect(result.current.data).toEqual({ "task-1": link }));
    expect(mocks.listWorkspaceTaskIssues).toHaveBeenCalledWith("ws-1", expect.any(Object));
  });
});
