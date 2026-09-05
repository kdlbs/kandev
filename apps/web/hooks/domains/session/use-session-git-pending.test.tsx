import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { GitStatusEntry } from "@/lib/state/slices/session-runtime/types";
import { pendingKey, useSessionGit } from "./use-session-git";
import { buildPendingFileStateIndex } from "./use-session-git-pending";

const mocks = vi.hoisted(() => ({
  statuses: [] as Array<{ repository_name: string; status: GitStatusEntry }>,
  scopeGeneration: 0,
  checkoutGenerations: {} as Record<string, number>,
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
  useSessionGitPendingCheckoutGenerations: () => mocks.checkoutGenerations,
  useSessionGitPendingScope: (sessionId: string | null) =>
    sessionId ? `${sessionId}:environment:${mocks.scopeGeneration}` : "",
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

function status(path: string, staged: boolean, branch = "main"): GitStatusEntry {
  return {
    branch,
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

type OperationResult = {
  success: boolean;
  operation: string;
  output: string;
  error?: string;
};

function deferredResult() {
  let resolve!: (value: OperationResult) => void;
  const promise = new Promise<OperationResult>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

beforeEach(() => {
  vi.clearAllMocks();
  mocks.statuses = [];
  mocks.scopeGeneration = 0;
  mocks.checkoutGenerations = {};
});

afterEach(cleanup);

describe("useSessionGit pending target-state reconciliation", () => {
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
        expect(hook.result.current.pendingStageFiles).toContain(pendingKey("", path));
      });

      await act(async () => {
        pending.resolve({ success: true, operation, output: "" });
        await pending.promise;
      });

      await waitFor(() => {
        expect(hook.result.current.pendingStageFiles).not.toContain(pendingKey("", path));
      });
    },
  );
});

describe("useSessionGit failed request cleanup", () => {
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

describe("useSessionGit partial repository failures", () => {
  it.each([
    { operation: "stage" as const, initialStaged: false, completedStaged: true },
    { operation: "unstage" as const, initialStaged: true, completedStaged: false },
  ])(
    "keeps successful repository scopes pending after a partial $operation failure",
    async ({ operation, initialStaged, completedStaged }) => {
      const successfulPath = `successful-${operation}.txt`;
      const failedPath = `failed-${operation}.txt`;
      mocks.statuses = [
        { repository_name: "repo-a", status: status(successfulPath, initialStaged) },
        { repository_name: "repo-b", status: status(failedPath, initialStaged) },
      ];
      mocks.gitOps[operation].mockImplementation(
        async (_paths: string[] | undefined, repositoryName: string | undefined) => ({
          success: repositoryName === "repo-a",
          operation,
          output: "",
          error: repositoryName === "repo-a" ? undefined : `${operation} failed`,
        }),
      );

      const hook = renderHook(() => useSessionGit("session-1"));

      await act(async () => {
        await hook.result.current[`${operation}File`]([successfulPath, failedPath]);
      });

      expect(hook.result.current.pendingStageFiles).toEqual(
        new Set([pendingKey("repo-a", successfulPath)]),
      );

      mocks.statuses = [
        { repository_name: "repo-a", status: status(successfulPath, completedStaged) },
        { repository_name: "repo-b", status: status(failedPath, initialStaged) },
      ];
      hook.rerender();
      await waitFor(() => {
        expect(hook.result.current.pendingStageFiles).not.toContain(
          pendingKey("repo-a", successfulPath),
        );
      });
    },
  );
});

describe("useSessionGit overlapping request ownership", () => {
  it.each([
    { operation: "stage" as const, initialStaged: false, completedStaged: true },
    { operation: "unstage" as const, initialStaged: true, completedStaged: false },
  ])(
    "keeps a newer same-direction $operation request pending when the older request fails",
    async ({ operation, initialStaged, completedStaged }) => {
      const path = `overlapping-${operation}.txt`;
      const older = deferredResult();
      const newer = deferredResult();
      mocks.statuses = [{ repository_name: "", status: status(path, initialStaged) }];
      mocks.gitOps[operation].mockReturnValueOnce(older.promise).mockReturnValueOnce(newer.promise);

      const hook = renderHook(() => useSessionGit("session-1"));

      act(() => {
        void hook.result.current[`${operation}File`]([path], "");
      });
      await waitFor(() => {
        expect(hook.result.current.pendingStageFiles).toContain(pendingKey("", path));
      });

      act(() => {
        void hook.result.current[`${operation}File`]([path], "");
      });
      await act(async () => {
        older.resolve({
          success: false,
          operation,
          output: "",
          error: `older ${operation} failed`,
        });
        await older.promise;
      });

      expect(hook.result.current.pendingStageFiles).toContain(pendingKey("", path));

      newer.resolve({ success: true, operation, output: "" });
      mocks.statuses = [{ repository_name: "", status: status(path, completedStaged) }];
      hook.rerender();
      await waitFor(() => {
        expect(hook.result.current.pendingStageFiles).not.toContain(pendingKey("", path));
      });
    },
  );

  it.each([
    { operation: "stage" as const, initialStaged: false, completedStaged: true },
    { operation: "unstage" as const, initialStaged: true, completedStaged: false },
  ])(
    "does not clear a newer same-direction $operation request from an older success status",
    async ({ operation, initialStaged, completedStaged }) => {
      const path = `overlapping-success-${operation}.txt`;
      const older = deferredResult();
      const newer = deferredResult();
      mocks.statuses = [{ repository_name: "", status: status(path, initialStaged) }];
      mocks.gitOps[operation].mockReturnValueOnce(older.promise).mockReturnValueOnce(newer.promise);

      const hook = renderHook(() => useSessionGit("session-1"));

      act(() => {
        void hook.result.current[`${operation}File`]([path], "");
      });
      await waitFor(() => {
        expect(hook.result.current.pendingStageFiles).toContain(pendingKey("", path));
      });

      act(() => {
        void hook.result.current[`${operation}File`]([path], "");
      });

      await act(async () => {
        older.resolve({ success: true, operation, output: "" });
        await older.promise;
      });

      mocks.statuses = [{ repository_name: "", status: status(path, completedStaged) }];
      hook.rerender();

      await waitFor(() => {
        expect(hook.result.current.pendingStageFiles).toContain(pendingKey("", path));
      });

      await act(async () => {
        newer.resolve({ success: true, operation, output: "" });
        await newer.promise;
      });

      await waitFor(() => {
        expect(hook.result.current.pendingStageFiles).not.toContain(pendingKey("", path));
      });
    },
  );
});

describe("pending file state indexing", () => {
  it("tracks staged and unstaged facets for each repository and path", () => {
    const index = buildPendingFileStateIndex([
      {
        path: "shared.ts",
        repository_name: "repo-a",
        status: "modified",
        staged: true,
        staged_change: { status: "modified" },
        unstaged_change: { status: "modified" },
      },
      {
        path: "only-staged.ts",
        repository_name: "repo-b",
        status: "modified",
        staged: true,
      },
    ]);

    expect(index.get(pendingKey("repo-a", "shared.ts"))).toEqual({
      hasStaged: true,
      hasUnstaged: true,
    });
    expect(index.get(pendingKey("repo-b", "only-staged.ts"))).toEqual({
      hasStaged: true,
      hasUnstaged: false,
    });
  });
});

describe("useSessionGit pending scope ownership", () => {
  it("resets pending ownership when the active session changes", async () => {
    const path = "shared-worktree-path.txt";
    const older = deferredResult();
    const newer = deferredResult();
    mocks.statuses = [{ repository_name: "", status: status(path, false) }];
    mocks.gitOps.stage.mockReturnValueOnce(older.promise).mockReturnValueOnce(newer.promise);

    const hook = renderHook(({ sessionId }) => useSessionGit(sessionId), {
      initialProps: { sessionId: "session-1" },
    });

    act(() => {
      void hook.result.current.stageFile([path], "");
    });
    await waitFor(() => {
      expect(hook.result.current.pendingStageFiles).toContain(pendingKey("", path));
    });

    hook.rerender({ sessionId: "session-2" });
    await waitFor(() => {
      expect(hook.result.current.pendingStageFiles).not.toContain(pendingKey("", path));
    });

    act(() => {
      void hook.result.current.stageFile([path], "");
    });
    await waitFor(() => {
      expect(hook.result.current.pendingStageFiles).toContain(pendingKey("", path));
    });

    await act(async () => {
      older.resolve({
        success: false,
        operation: "stage",
        output: "",
        error: "previous session failed",
      });
      await older.promise;
    });

    expect(hook.result.current.pendingStageFiles).toContain(pendingKey("", path));

    newer.resolve({ success: true, operation: "stage", output: "" });
  });

  it("resets pending ownership when the checked-out branch changes", async () => {
    const path = "branch-owned-stage.txt";
    const pending = deferredResult();
    mocks.statuses = [{ repository_name: "", status: status(path, false, "feature/old") }];
    mocks.gitOps.stage.mockReturnValueOnce(pending.promise);

    const hook = renderHook(() => useSessionGit("session-1"));

    act(() => {
      void hook.result.current.stageFile([path], "");
    });
    await waitFor(() => {
      expect(hook.result.current.pendingStageFiles).toContain(pendingKey("", path));
    });

    mocks.scopeGeneration += 1;
    mocks.statuses = [{ repository_name: "", status: status(path, false, "feature/new") }];
    hook.rerender();

    await waitFor(() => {
      expect(hook.result.current.pendingStageFiles).not.toContain(pendingKey("", path));
    });

    pending.resolve({ success: true, operation: "stage", output: "" });
  });

  it("resets only the repository affected by a checkout generation", async () => {
    const repoAPath = "repo-a-stage.txt";
    const repoBPath = "repo-b-stage.txt";
    const repoAPending = deferredResult();
    const repoBPending = deferredResult();
    mocks.statuses = [
      { repository_name: "repo-a", status: status(repoAPath, false) },
      { repository_name: "repo-b", status: status(repoBPath, false) },
    ];
    mocks.gitOps.stage
      .mockReturnValueOnce(repoAPending.promise)
      .mockReturnValueOnce(repoBPending.promise);

    const hook = renderHook(() => useSessionGit("session-1"));
    act(() => {
      void hook.result.current.stageFile([repoAPath], "repo-a");
      void hook.result.current.stageFile([repoBPath], "repo-b");
    });
    await waitFor(() => {
      expect(hook.result.current.pendingStageFiles).toEqual(
        new Set([pendingKey("repo-a", repoAPath), pendingKey("repo-b", repoBPath)]),
      );
    });

    mocks.checkoutGenerations = { "repo-a": 1 };
    mocks.statuses = [
      { repository_name: "repo-a", status: status(repoAPath, false) },
      { repository_name: "repo-b", status: status(repoBPath, false) },
    ];
    hook.rerender();

    expect(hook.result.current.pendingStageFiles).toEqual(
      new Set([pendingKey("repo-b", repoBPath)]),
    );

    repoAPending.resolve({ success: true, operation: "stage", output: "" });
    repoBPending.resolve({ success: true, operation: "stage", output: "" });
  });
});
