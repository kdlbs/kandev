import { cleanup, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { AppState } from "@/lib/state/store";
import { useSessionGitPendingScope, useSessionGitStatusSnapshots } from "./use-session-git-status";

const mocks = vi.hoisted(() => ({
  state: {} as AppState,
}));
const READY_PATCH = "ready patch";

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: AppState) => unknown) => selector(mocks.state),
}));

describe("useSessionGitPendingScope", () => {
  afterEach(cleanup);

  it("changes across session and environment scopes but ignores commit refetches", () => {
    mocks.state.environmentIdBySessionId = { "session-1": "environment-1" };
    mocks.state.sessionCommits = {
      byEnvironmentId: {},
      loading: {},
      refetchTrigger: { "environment-1": 0 },
    };

    const hook = renderHook(({ sessionId }) => useSessionGitPendingScope(sessionId), {
      initialProps: { sessionId: "session-1" as string | null },
    });
    const initial = hook.result.current;

    hook.rerender({ sessionId: "session-2" });
    expect(hook.result.current).not.toBe(initial);

    mocks.state.environmentIdBySessionId["session-2"] = "environment-2";
    hook.rerender({ sessionId: "session-2" });
    const environmentScoped = hook.result.current;
    expect(environmentScoped).not.toBe(initial);

    mocks.state.sessionCommits.refetchTrigger["environment-2"] = 1;
    hook.rerender({ sessionId: "session-2" });
    expect(hook.result.current).toBe(environmentScoped);
  });
});

describe("useSessionGitStatusSnapshots", () => {
  afterEach(cleanup);

  it("keeps the raw ready snapshot separate while projecting unavailable freshness", () => {
    const status = {
      status_state: "ready" as const,
      files_complete: true,
      detail_state: "ready" as const,
      branch: "main",
      remote_branch: null,
      modified: ["src/a.ts"],
      added: [],
      deleted: [],
      untracked: [],
      renamed: [],
      ahead: 0,
      behind: 0,
      head_commit: "head-a",
      base_commit: "base-a",
      comparison_target: "origin/main",
      files: {
        "src/a.ts": {
          path: "src/a.ts",
          status: "modified",
          staged: false,
          additions: 3,
          deletions: 2,
          diff: READY_PATCH,
          diff_state: "ready" as const,
        },
      },
      timestamp: "2026-10-06T00:00:00Z",
      tracker_id: "tracker-a",
      tracker_epoch: 1,
      snapshot_revision: 4,
      repository_name: "",
    };
    mocks.state.environmentIdBySessionId = { "session-1": "environment-1" };
    mocks.state.connection = { status: "disconnected" } as AppState["connection"];
    mocks.state.gitStatus = {
      byEnvironmentId: { "environment-1": status },
      byEnvironmentRepo: { "environment-1": { "": status } },
      refreshByEnvironmentId: {
        "environment-1": {
          state: "unavailable",
          request_id: "request-a",
          error_code: "status_unavailable",
        },
      },
    } as AppState["gitStatus"];
    mocks.state.gitStatusDisplay = {
      byEnvironmentRepo: {
        "environment-1": {
          "": {
            checkoutGeneration: 0,
            branch: "main",
            headCommit: "head-a",
            baseCommit: "base-a",
            comparisonTarget: "origin/main",
            files: {
              "src/a.ts": {
                flat: { additions: 3, deletions: 2, diff: READY_PATCH },
              },
            },
          },
        },
      },
    };
    mocks.state.gitCheckoutGeneration = {
      byEnvironmentId: { "environment-1": { "": 0 } },
    } as AppState["gitCheckoutGeneration"];

    const { result } = renderHook(() => useSessionGitStatusSnapshots("session-1"));

    expect(result.current.gitStatus?.files["src/a.ts"]).toMatchObject({
      diff: READY_PATCH,
      diff_state: "ready",
    });
    expect(result.current.displayGitStatus?.files["src/a.ts"]).toMatchObject({
      diff: READY_PATCH,
      diff_state: "unavailable",
      display_stale: true,
    });
  });
});
