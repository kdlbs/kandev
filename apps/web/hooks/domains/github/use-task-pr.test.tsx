import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createElement, type ReactNode } from "react";
import { act, cleanup, renderHook } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import type { AppState } from "@/lib/state/store";
import type { TaskPR } from "@/lib/types/github";

const requestMock = vi.fn();
const deleteTaskPRMock = vi.hoisted(() => vi.fn());

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ request: requestMock }),
}));

// listWorkspaceTaskPRs is only used by useWorkspacePRs (not under test
// here). Stub it so the module import doesn't fail in jsdom.
vi.mock("@/lib/api/domains/github-api", () => ({
  listWorkspaceTaskPRs: vi.fn().mockResolvedValue(null),
  deleteTaskPR: deleteTaskPRMock,
}));

import { useTaskPR } from "./use-task-pr";
import { useAppStoreApi } from "@/components/state-provider";

function wrapper({ children }: { children: ReactNode }) {
  return createElement(StateProvider, null, children);
}

function createStateWrapper(initialState: Partial<AppState>) {
  return function StateTestWrapper({ children }: { children: ReactNode }) {
    return createElement(StateProvider, { initialState, children });
  };
}

// Renders the hook alongside the store api so a test can drive
// `connection.status` transitions the way `useWebSocket` does in prod.
function useTaskPRWithStore(taskId: string | null) {
  const result = useTaskPR(taskId);
  const store = useAppStoreApi();
  return { result, store };
}

const linkedPR = { id: "association-1", task_id: "task-1" } as TaskPR;

function unlinkState(workspaceId: string | null): Partial<AppState> {
  return {
    workspaces: { items: [], activeId: workspaceId },
    taskPRs: { byTaskId: { "task-1": [linkedPR] } },
  };
}

beforeEach(() => {
  vi.useFakeTimers();
  requestMock.mockReset();
  requestMock.mockResolvedValue({ prs: [] });
  deleteTaskPRMock.mockReset();
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

describe("useTaskPR — permanent flag", () => {
  // The dominant production signal in the SyncWatchesBatched storm was the
  // frontend polling `github.task_pr.sync` every 5s for tasks whose repos
  // were deleted/inaccessible. The backend now returns `permanent: true`
  // on those responses; the hook must stop the retry interval cold.
  it("stops the 5s retry interval when the backend reports permanent: true", async () => {
    requestMock.mockResolvedValue({ prs: [], permanent: true });

    renderHook(() => useTaskPR("task-1"), { wrapper });

    // Initial freshness sync fires synchronously from the mount effect.
    // Flush the resolved promise so the permanent flag is applied before
    // the interval would otherwise fire.
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(requestMock).toHaveBeenCalledTimes(1);

    // Advance well past several retry windows. If the permanent
    // short-circuit regressed, this would burst 5-6 additional calls
    // into requestMock.
    await act(async () => {
      await vi.advanceTimersByTimeAsync(5_000 * 6);
    });
    expect(requestMock).toHaveBeenCalledTimes(1);
  });

  // Without permanent, the existing retry cadence must still kick in so
  // tasks waiting on a freshly-pushed branch still get their PR detected.
  it("retries every 5s when permanent is absent and no PR is in the store", async () => {
    requestMock.mockResolvedValue({ prs: [] });

    renderHook(() => useTaskPR("task-1"), { wrapper });

    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(requestMock).toHaveBeenCalledTimes(1);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(5_000);
    });
    expect(requestMock).toHaveBeenCalledTimes(2);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(5_000);
    });
    expect(requestMock).toHaveBeenCalledTimes(3);
  });
});

describe("useTaskPR — unlink", () => {
  it("rejects without a task or active workspace and skips the API", async () => {
    const noTask = renderHook(() => useTaskPR(null), {
      wrapper: createStateWrapper(unlinkState("ws-1")),
    });
    await expect(noTask.result.current.unlink(linkedPR.id)).rejects.toThrow(
      "No active workspace is selected.",
    );

    const noWorkspace = renderHook(() => useTaskPR("task-1"), {
      wrapper: createStateWrapper(unlinkState(null)),
    });
    await expect(noWorkspace.result.current.unlink(linkedPR.id)).rejects.toThrow(
      "No active workspace is selected.",
    );
    expect(deleteTaskPRMock).not.toHaveBeenCalled();
  });

  it("deletes through the active workspace before removing local state", async () => {
    deleteTaskPRMock.mockResolvedValue(undefined);
    const view = renderHook(() => useTaskPRWithStore("task-1"), {
      wrapper: createStateWrapper(unlinkState("ws-1")),
    });

    await act(async () => {
      await view.result.current.result.unlink(linkedPR.id);
    });

    expect(deleteTaskPRMock).toHaveBeenCalledWith(linkedPR.id, "ws-1");
    expect(view.result.current.store.getState().taskPRs.byTaskId["task-1"]).toBeUndefined();
  });

  it("propagates API failures without removing the local association", async () => {
    deleteTaskPRMock.mockRejectedValue(new Error("unlink failed"));
    const view = renderHook(() => useTaskPRWithStore("task-1"), {
      wrapper: createStateWrapper(unlinkState("ws-1")),
    });

    await expect(view.result.current.result.unlink(linkedPR.id)).rejects.toThrow("unlink failed");
    expect(view.result.current.store.getState().taskPRs.byTaskId["task-1"]).toEqual([linkedPR]);
  });
});

describe("useTaskPR — reconnect resync", () => {
  // Reproduces the missing "Pull Request" tab: the task is opened while the
  // WS is down, all 6 retry attempts elapse without a response, and the store
  // never learns about the PR. When the socket later reconnects, the hook must
  // reset the exhausted retry window and refire the sync so the tab appears.
  it("resyncs when the connection transitions to connected after retries are exhausted", async () => {
    // No PR ever comes back while "disconnected", so the retry loop runs to
    // exhaustion (initial call + 6 retries).
    requestMock.mockResolvedValue({ prs: [] });

    const { result } = renderHook(() => useTaskPRWithStore("task-1"), { wrapper });

    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    // Burn through the full retry budget (6 * 5s).
    await act(async () => {
      await vi.advanceTimersByTimeAsync(5_000 * 6);
    });
    const callsAfterExhaustion = requestMock.mock.calls.length;

    // The socket comes up; useWebSocket flips the store to "connected".
    await act(async () => {
      result.current.store.getState().setConnectionStatus("connected", null);
      await vi.advanceTimersByTimeAsync(0);
    });

    expect(requestMock.mock.calls.length).toBeGreaterThan(callsAfterExhaustion);
  });

  // Resetting the retry refs on reconnect is not enough on its own: once the
  // retry budget is exhausted the 5s interval clears itself, and simply zeroing
  // retryRef doesn't recreate it. If the single reconnect sync returns empty,
  // the store stays empty and the tab never appears. The polling interval must
  // restart on reconnect so a fresh 30s window of 5s retries runs.
  it("restarts the 5s retry polling after reconnect when the resync returns empty", async () => {
    requestMock.mockResolvedValue({ prs: [] });

    const { result } = renderHook(() => useTaskPRWithStore("task-1"), { wrapper });

    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    // Advance past the clearing tick (7 * 5s = 35s): the 6 retries fire and the
    // 7th tick hits the budget guard and clears the interval, so no interval is
    // live when the socket reconnects.
    await act(async () => {
      await vi.advanceTimersByTimeAsync(5_000 * 7);
    });
    const callsAfterExhaustion = requestMock.mock.calls.length;

    // Reconnect: the resync fires once immediately (still empty).
    await act(async () => {
      result.current.store.getState().setConnectionStatus("connected", null);
      await vi.advanceTimersByTimeAsync(0);
    });
    const callsAfterReconnect = requestMock.mock.calls.length;
    expect(callsAfterReconnect).toBeGreaterThan(callsAfterExhaustion);

    // A fresh polling window must now run: advancing another 5s tick fires
    // another retry. Without restarting the interval this stays flat.
    await act(async () => {
      await vi.advanceTimersByTimeAsync(5_000);
    });
    expect(requestMock.mock.calls.length).toBeGreaterThan(callsAfterReconnect);
  });

  // A no-op status write (already-connected re-render) must not refire, or
  // every unrelated store update would spam the backend sync.
  it("does not refire while the status stays connected", async () => {
    requestMock.mockResolvedValue({ prs: [] });

    const { result } = renderHook(() => useTaskPRWithStore("task-1"), { wrapper });

    await act(async () => {
      result.current.store.getState().setConnectionStatus("connected", null);
      await vi.advanceTimersByTimeAsync(0);
    });
    const callsAfterConnect = requestMock.mock.calls.length;

    // Re-assert the same status: no transition edge, so no new sync.
    await act(async () => {
      result.current.store.getState().setConnectionStatus("connected", null);
      await vi.advanceTimersByTimeAsync(0);
    });

    expect(requestMock.mock.calls.length).toBe(callsAfterConnect);
  });
});
