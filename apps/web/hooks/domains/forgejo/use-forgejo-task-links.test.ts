import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useForgejoTaskLinks } from "./use-forgejo-task-links";

const api = vi.hoisted(() => ({
  issues: vi.fn(),
  prs: vi.fn(),
  refreshIssue: vi.fn(),
  refreshPR: vi.fn(),
  unlinkIssue: vi.fn(),
  unlinkPR: vi.fn(),
}));

vi.mock("@/lib/api/domains/forgejo-api", () => ({
  listForgejoTaskIssues: api.issues,
  listForgejoTaskPRs: api.prs,
  refreshForgejoTaskIssue: api.refreshIssue,
  refreshForgejoTaskPullRequest: api.refreshPR,
  unlinkForgejoIssue: api.unlinkIssue,
  unlinkForgejoPullRequest: api.unlinkPR,
}));

describe("useForgejoTaskLinks", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    api.issues.mockResolvedValue({ issues: [{ id: "issue-1" }] });
    api.prs.mockResolvedValue({ pull_requests: [{ id: "pr-1" }] });
  });

  it("loads only when both workspace and task are present", async () => {
    const { result, rerender } = renderHook(
      ({ workspaceId, taskId }) => useForgejoTaskLinks(workspaceId, taskId),
      { initialProps: { workspaceId: undefined as string | undefined, taskId: "task-1" } },
    );
    expect(api.issues).not.toHaveBeenCalled();

    rerender({ workspaceId: "ws-a", taskId: "task-1" });
    await waitFor(() => expect(result.current.issues).toHaveLength(1));
    expect(api.issues).toHaveBeenCalledWith("task-1", { workspaceId: "ws-a" });
    expect(api.prs).toHaveBeenCalledWith("task-1", { workspaceId: "ws-a" });
  });

  it("refreshes authoritative links after a mutation", async () => {
    const { result } = renderHook(() => useForgejoTaskLinks("ws-a", "task-1"));
    await waitFor(() => expect(result.current.pullRequests).toHaveLength(1));

    await act(async () => result.current.removePullRequest("pr-1"));

    expect(api.unlinkPR).toHaveBeenCalledWith("pr-1", { workspaceId: "ws-a" });
    expect(api.issues).toHaveBeenCalledTimes(2);
    expect(api.prs).toHaveBeenCalledTimes(2);
  });
});
