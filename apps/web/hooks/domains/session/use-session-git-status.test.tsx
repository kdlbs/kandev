import { cleanup, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { AppState } from "@/lib/state/store";
import {
  useSessionGitPendingScope,
  useSessionGitStatus,
  useSessionGitStatusSnapshots,
} from "./use-session-git-status";

const mocks = vi.hoisted(() => ({
  state: {} as AppState,
  unsubscribeSession: vi.fn(),
  subscribeSession: vi.fn(() => mocks.unsubscribeSession),
}));
const FIRST_SESSION_ID = "session-1";
const FIRST_ENVIRONMENT_ID = "environment-1";
const READY_PATCH = "ready patch";

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: AppState) => unknown) => selector(mocks.state),
}));
vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ subscribeSession: mocks.subscribeSession }),
}));

describe("useSessionGitStatus", () => {
  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it("subscribes only while the session detail is active", () => {
    mocks.state.environmentIdBySessionId = { [FIRST_SESSION_ID]: FIRST_ENVIRONMENT_ID };
    mocks.state.gitStatus = { byEnvironmentId: {} } as AppState["gitStatus"];
    mocks.state.connection = { status: "connected" } as AppState["connection"];

    const hook = renderHook(
      ({ detailActive }) => useSessionGitStatus(FIRST_SESSION_ID, { detailActive }),
      {
        initialProps: { detailActive: false },
      },
    );

    expect(mocks.subscribeSession).not.toHaveBeenCalled();

    hook.rerender({ detailActive: true });
    expect(mocks.subscribeSession).toHaveBeenCalledOnce();
    expect(mocks.subscribeSession).toHaveBeenCalledWith(FIRST_SESSION_ID);

    hook.rerender({ detailActive: false });
    expect(mocks.subscribeSession).toHaveBeenCalledOnce();
    expect(mocks.unsubscribeSession).toHaveBeenCalledOnce();
  });
});

describe("useSessionGitPendingScope", () => {
  afterEach(cleanup);

  it("changes across session and environment scopes but ignores commit refetches", () => {
    mocks.state.environmentIdBySessionId = { [FIRST_SESSION_ID]: FIRST_ENVIRONMENT_ID };
    mocks.state.sessionCommits = {
      byEnvironmentId: {},
      loading: {},
      refetchTrigger: { [FIRST_ENVIRONMENT_ID]: 0 },
    };

    const hook = renderHook(({ sessionId }) => useSessionGitPendingScope(sessionId), {
      initialProps: { sessionId: FIRST_SESSION_ID as string | null },
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
    mocks.state.environmentIdBySessionId = { [FIRST_SESSION_ID]: FIRST_ENVIRONMENT_ID };
    mocks.state.connection = { status: "disconnected" } as AppState["connection"];
    mocks.state.gitStatus = {
      byEnvironmentId: { [FIRST_ENVIRONMENT_ID]: status },
      byEnvironmentRepo: { [FIRST_ENVIRONMENT_ID]: { "": status } },
      refreshByEnvironmentId: {
        [FIRST_ENVIRONMENT_ID]: {
          state: "unavailable",
          request_id: "request-a",
          error_code: "status_unavailable",
        },
      },
    } as AppState["gitStatus"];
    mocks.state.gitStatusDisplay = {
      byEnvironmentRepo: {
        [FIRST_ENVIRONMENT_ID]: {
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
      byEnvironmentId: { [FIRST_ENVIRONMENT_ID]: { "": 0 } },
    } as AppState["gitCheckoutGeneration"];

    const { result } = renderHook(() => useSessionGitStatusSnapshots(FIRST_SESSION_ID));

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

describe("useSessionGitStatusSnapshots display scope", () => {
  afterEach(cleanup);

  it("keeps the display scope when an absent comparison target arrives as an empty string", () => {
    mocks.state.environmentIdBySessionId = { [FIRST_SESSION_ID]: FIRST_ENVIRONMENT_ID };
    mocks.state.connection = { status: "disconnected" } as AppState["connection"];
    mocks.state.gitStatus = {
      byEnvironmentId: {},
      byEnvironmentRepo: {},
    } as AppState["gitStatus"];
    mocks.state.gitCheckoutGeneration = {
      byEnvironmentId: {},
    } as AppState["gitCheckoutGeneration"];
    const display = {
      checkoutGeneration: 0,
      branch: "main",
      headCommit: "head-a",
      baseCommit: "base-a",
      comparisonTarget: null,
      files: {},
    };
    mocks.state.gitStatusDisplay = {
      byEnvironmentRepo: { [FIRST_ENVIRONMENT_ID]: { "": display } },
    };
    const hook = renderHook(() => useSessionGitStatusSnapshots(FIRST_SESSION_ID));
    const initialScope = hook.result.current.displayScopeByRepo[""];

    mocks.state.gitStatusDisplay = {
      byEnvironmentRepo: {
        [FIRST_ENVIRONMENT_ID]: { "": { ...display, comparisonTarget: "" } },
      },
    };
    hook.rerender();
    expect(hook.result.current.displayScopeByRepo[""]).toBe(initialScope);

    mocks.state.gitStatusDisplay = {
      byEnvironmentRepo: {
        [FIRST_ENVIRONMENT_ID]: { "": { ...display, comparisonTarget: "origin/main" } },
      },
    };
    hook.rerender();
    expect(hook.result.current.displayScopeByRepo[""]).not.toBe(initialScope);
  });
});
