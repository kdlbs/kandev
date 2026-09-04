import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { GitStatusEntry } from "@/lib/state/slices/session-runtime/types";
import { pendingKey, useSessionGit } from "./use-session-git";

const mocks = vi.hoisted(() => ({
  statuses: [] as Array<{ repository_name: string; status: GitStatusEntry }>,
  gitOps: {
    isLoading: false,
    loadingOperation: null,
    pull: vi.fn(),
    push: vi.fn(),
    rebase: vi.fn(),
    merge: vi.fn(),
    abort: vi.fn(),
    commit: vi.fn(),
    stage: vi.fn(),
    unstage: vi.fn(),
    discard: vi.fn(),
    revertCommit: vi.fn(),
    renameBranch: vi.fn(),
    reset: vi.fn(),
    createPR: vi.fn(),
  },
}));

vi.mock("./use-session-git-status", () => ({
  useSessionGitStatus: () => undefined,
  useSessionGitStatusByRepo: () => mocks.statuses,
}));

vi.mock("./use-session-commits", () => ({
  useSessionCommits: () => ({ commits: [], loading: false }),
}));

vi.mock("./use-cumulative-diff", () => ({
  useCumulativeDiff: () => ({ diff: null }),
}));

vi.mock("@/hooks/use-git-operations", () => ({
  useGitOperations: () => mocks.gitOps,
}));

function status(path: string, staged: boolean): GitStatusEntry {
  return {
    branch: "main",
    remote_branch: null,
    modified: [path],
    added: [],
    deleted: [],
    untracked: [],
    renamed: [],
    ahead: 0,
    behind: 0,
    files: {
      [path]: {
        path,
        status: "modified",
        staged,
      },
    },
    timestamp: null,
  };
}

function deferredResult() {
  let resolve!: (value: { success: true; operation: string; output: string }) => void;
  const promise = new Promise<{ success: true; operation: string; output: string }>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

describe("useSessionGit pending file actions", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.statuses = [];
  });

  afterEach(cleanup);

  it.each([
    { operation: "stage" as const, initialStaged: false, completedStaged: true },
    { operation: "unstage" as const, initialStaged: true, completedStaged: false },
  ])(
    "keeps $operation pending through stale repository refreshes",
    async ({ operation, initialStaged, completedStaged }) => {
      const path = `${operation}-pending.txt`;
      const pending = deferredResult();
      mocks.gitOps[operation].mockReturnValueOnce(pending.promise);
      mocks.statuses = [{ repository_name: "", status: status(path, initialStaged) }];

      const hook = renderHook(() => useSessionGit("session-1"));

      act(() => {
        void hook.result.current[`${operation}File`]([path], "");
      });
      await waitFor(() => {
        expect(hook.result.current.pendingStageFiles).toContain(pendingKey("", path));
      });

      mocks.statuses = [{ repository_name: "", status: status(path, initialStaged) }];
      hook.rerender();

      await waitFor(() => {
        expect(hook.result.current.pendingStageFiles).toContain(pendingKey("", path));
      });

      mocks.statuses = [{ repository_name: "", status: status(path, completedStaged) }];
      hook.rerender();

      await waitFor(() => {
        expect(hook.result.current.pendingStageFiles).not.toContain(pendingKey("", path));
      });

      pending.resolve({ success: true, operation, output: "" });
    },
  );

  it("clears pending state when the operation returns a failure without a status change", async () => {
    const path = "failed-stage.txt";
    mocks.gitOps.stage.mockResolvedValueOnce({
      success: false,
      operation: "stage",
      output: "",
      error: "stage failed",
    });
    mocks.statuses = [{ repository_name: "", status: status(path, false) }];

    const hook = renderHook(() => useSessionGit("session-1"));

    await act(async () => {
      await hook.result.current.stageFile([path], "");
    });

    expect(hook.result.current.pendingStageFiles).not.toContain(pendingKey("", path));
  });
});
