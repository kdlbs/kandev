import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";

const mocks = vi.hoisted(() => ({
  workspaceId: "ws-1" as string | null,
  activeTaskId: "task-1",
  prs: [] as unknown[],
  request: vi.fn(),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({
      workspaces: { activeId: mocks.workspaceId },
      tasks: { activeTaskId: mocks.activeTaskId },
      taskPRs: { byTaskId: { [mocks.activeTaskId]: mocks.prs } },
    }),
}));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ request: mocks.request }),
}));

import { useActiveTaskPRsWithFiles } from "./use-active-task-pr-files";
import { usePRCommits } from "./use-pr-commits";
import { usePRDiff } from "./use-pr-diff";
import type { TaskPR } from "@/lib/types/github";

afterEach(() => {
  cleanup();
  mocks.workspaceId = "ws-1";
  mocks.activeTaskId = "task-1";
  mocks.prs = [];
  mocks.request.mockReset();
});

function taskPR(): TaskPR {
  return {
    id: "pr-1",
    workspace_id: "ws-1",
    task_id: "task-1",
    owner: "acme",
    repo: "site",
    pr_number: 42,
    pr_url: "https://github.com/acme/site/pull/42",
    pr_title: "Scoped PR",
    head_branch: "feature",
    base_branch: "main",
    author_login: "octocat",
    state: "open",
    review_state: "",
    checks_state: "",
    mergeable_state: "unknown",
    review_count: 0,
    pending_review_count: 0,
    comment_count: 0,
    unresolved_review_threads: 0,
    checks_total: 0,
    checks_passing: 0,
    additions: 1,
    deletions: 0,
    created_at: "2026-07-20T00:00:00Z",
    merged_at: null,
    closed_at: null,
    last_synced_at: "2026-07-20T00:00:00Z",
    updated_at: "2026-07-20T00:00:00Z",
  };
}

describe("PR workspace request scope", () => {
  it("includes the active workspace in PR file and commit requests", async () => {
    mocks.request.mockImplementation((action: string) => {
      if (action === "github.pr_files.get") return Promise.resolve({ files: [] });
      if (action === "github.pr_commits.get") return Promise.resolve({ commits: [] });
      return Promise.reject(new Error(`unexpected action ${action}`));
    });

    renderHook(() => ({
      diff: usePRDiff("acme", "site", 42),
      commits: usePRCommits("acme", "site", 42),
    }));

    await waitFor(() => expect(mocks.request).toHaveBeenCalledTimes(2));
    expect(mocks.request).toHaveBeenCalledWith("github.pr_files.get", {
      workspace_id: "ws-1",
      owner: "acme",
      repo: "site",
      number: 42,
    });
    expect(mocks.request).toHaveBeenCalledWith("github.pr_commits.get", {
      workspace_id: "ws-1",
      owner: "acme",
      repo: "site",
      number: 42,
    });
  });

  it("drops an old workspace response after the active workspace changes", async () => {
    mocks.prs = [taskPR()];
    let resolveOld: ((value: { files: Array<{ filename: string }> }) => void) | undefined;
    let resolveNew: ((value: { files: Array<{ filename: string }> }) => void) | undefined;
    mocks.request
      .mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            resolveOld = resolve;
          }),
      )
      .mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            resolveNew = resolve;
          }),
      );

    const { result, rerender } = renderHook(() => useActiveTaskPRsWithFiles());
    await waitFor(() => expect(mocks.request).toHaveBeenCalledTimes(1));

    mocks.workspaceId = "ws-2";
    rerender();
    await waitFor(() => expect(mocks.request).toHaveBeenCalledTimes(2));
    expect(result.current.filesByPRKey).toEqual({});

    await act(async () => {
      resolveOld!({ files: [{ filename: "old.ts" }] });
      await Promise.resolve();
    });
    expect(result.current.filesByPRKey).toEqual({});

    await act(async () => {
      resolveNew!({ files: [{ filename: "new.ts" }] });
      await Promise.resolve();
    });
    await waitFor(() =>
      expect(Object.values(result.current.filesByPRKey).flat()).toEqual([{ filename: "new.ts" }]),
    );
    expect(mocks.request).toHaveBeenLastCalledWith("github.pr_files.get", {
      workspace_id: "ws-2",
      owner: "acme",
      repo: "site",
      number: 42,
    });
  });
});

describe("PR file refresh stability", () => {
  it("discards a superseded timestamp response that resolves last", async () => {
    mocks.prs = [taskPR()];
    let resolveSecond: ((value: { files: Array<{ filename: string }> }) => void) | undefined;
    let resolveThird: ((value: { files: Array<{ filename: string }> }) => void) | undefined;
    mocks.request
      .mockResolvedValueOnce({ files: [{ filename: "stable.ts" }] })
      .mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            resolveSecond = resolve;
          }),
      )
      .mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            resolveThird = resolve;
          }),
      );

    const { result, rerender } = renderHook(() => useActiveTaskPRsWithFiles());
    await waitFor(() => expect(mocks.request).toHaveBeenCalledTimes(1));

    mocks.prs = [{ ...taskPR(), last_synced_at: "2026-07-20T00:01:00Z" }];
    rerender();
    await waitFor(() => expect(mocks.request).toHaveBeenCalledTimes(2));

    mocks.prs = [{ ...taskPR(), last_synced_at: "2026-07-20T00:02:00Z" }];
    rerender();
    await waitFor(() => expect(mocks.request).toHaveBeenCalledTimes(3));

    await act(async () => {
      resolveThird!({ files: [{ filename: "newest.ts" }] });
      await Promise.resolve();
    });
    await waitFor(() =>
      expect(Object.values(result.current.filesByPRKey).flat()).toEqual([
        { filename: "newest.ts" },
      ]),
    );

    await act(async () => {
      resolveSecond!({ files: [{ filename: "superseded.ts" }] });
      await Promise.resolve();
    });
    expect(Object.values(result.current.filesByPRKey).flat()).toEqual([{ filename: "newest.ts" }]);
  });
});

describe("PR file refresh retention", () => {
  it("keeps resolved files visible while a sync timestamp refresh is pending", async () => {
    mocks.prs = [taskPR()];
    let resolveRefresh: ((value: { files: Array<{ filename: string }> }) => void) | undefined;
    mocks.request
      .mockResolvedValueOnce({ files: [{ filename: "stable.ts" }] })
      .mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            resolveRefresh = resolve;
          }),
      );

    const { result, rerender } = renderHook(() => useActiveTaskPRsWithFiles());
    await waitFor(() =>
      expect(Object.values(result.current.filesByPRKey).flat()).toEqual([
        { filename: "stable.ts" },
      ]),
    );

    mocks.prs = [{ ...taskPR(), last_synced_at: "2026-07-20T00:01:00Z" }];
    rerender();
    await waitFor(() => expect(mocks.request).toHaveBeenCalledTimes(2));

    expect(Object.values(result.current.filesByPRKey).flat()).toEqual([{ filename: "stable.ts" }]);

    await act(async () => {
      resolveRefresh!({ files: [{ filename: "refreshed.ts" }] });
      await Promise.resolve();
    });
    await waitFor(() =>
      expect(Object.values(result.current.filesByPRKey).flat()).toEqual([
        { filename: "refreshed.ts" },
      ]),
    );
  });

  it("keeps resolved files visible when a background refresh fails", async () => {
    mocks.prs = [taskPR()];
    mocks.request.mockResolvedValueOnce({ files: [{ filename: "stable.ts" }] });
    let rejectRefresh: ((reason: Error) => void) | undefined;

    const { result, rerender } = renderHook(() => useActiveTaskPRsWithFiles());
    await waitFor(() =>
      expect(Object.values(result.current.filesByPRKey).flat()).toEqual([
        { filename: "stable.ts" },
      ]),
    );

    mocks.request.mockImplementationOnce(
      () =>
        new Promise((_resolve, reject) => {
          rejectRefresh = reject;
        }),
    );
    mocks.prs = [{ ...taskPR(), last_synced_at: "2026-07-20T00:01:00Z" }];
    rerender();
    await waitFor(() => expect(mocks.request).toHaveBeenCalledTimes(2));

    await act(async () => {
      rejectRefresh!(new Error("refresh failed"));
      await Promise.resolve();
    });
    expect(Object.values(result.current.filesByPRKey).flat()).toEqual([{ filename: "stable.ts" }]);
  });

  it("clears retained files when the active task changes or removes its PR", async () => {
    mocks.prs = [taskPR()];
    mocks.request.mockResolvedValueOnce({ files: [{ filename: "task-one.ts" }] });

    const { result, rerender } = renderHook(() => useActiveTaskPRsWithFiles());
    await waitFor(() =>
      expect(Object.values(result.current.filesByPRKey).flat()).toEqual([
        { filename: "task-one.ts" },
      ]),
    );

    mocks.activeTaskId = "task-2";
    mocks.request.mockImplementationOnce(() => new Promise(() => undefined));
    rerender();
    await waitFor(() => expect(mocks.request).toHaveBeenCalledTimes(2));
    expect(result.current.filesByPRKey).toEqual({});

    mocks.prs = [];
    rerender();
    expect(result.current.filesByPRKey).toEqual({});
  });
});
