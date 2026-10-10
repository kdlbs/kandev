import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const getWorkspaceAggregate = vi.hoisted(() => vi.fn());
const setWorkspaceAggregate = vi.hoisted(() => vi.fn());

vi.mock("@/components/state-provider", () => ({
  useAppStore: (
    selector: (state: { setWorkspaceAggregate: typeof setWorkspaceAggregate }) => unknown,
  ) => selector({ setWorkspaceAggregate }),
}));

vi.mock("@/lib/api/domains/office-extended-api", () => ({ getWorkspaceAggregate }));

import { useWorkspaceAggregate } from "./use-workspace-aggregate";

const response = {
  workspaces: [
    {
      workspace_id: "ws-1",
      name: "Workspace one",
      task_count: 1,
      open_tasks: 1,
      in_progress_tasks: 0,
      blocked_tasks: 0,
      done_tasks: 0,
      pending_approvals: 0,
      agent_count: 1,
      running_agents: 0,
    },
  ],
  recent_activity: [],
};

function deferred<T>() {
  let resolve: (value: T) => void = () => {};
  const promise = new Promise<T>((res) => {
    resolve = res;
  });
  return { promise, resolve };
}

describe("useWorkspaceAggregate", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    getWorkspaceAggregate.mockResolvedValue(response);
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.clearAllMocks();
  });

  it("loads into the shared store and refreshes while the overview stays open", async () => {
    const { result } = renderHook(() => useWorkspaceAggregate());

    await act(async () => {
      await Promise.resolve();
    });

    expect(result.current.loadState).toBe("loaded");
    expect(setWorkspaceAggregate).toHaveBeenCalledWith({
      workspaces: response.workspaces,
      recentActivity: [],
    });
    expect(getWorkspaceAggregate).toHaveBeenCalledTimes(1);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(30_000);
    });

    expect(getWorkspaceAggregate).toHaveBeenCalledTimes(2);
  });

  it("keeps an initial failure distinct and retries it", async () => {
    getWorkspaceAggregate.mockRejectedValueOnce(new Error("offline"));
    const { result } = renderHook(() => useWorkspaceAggregate());

    await act(async () => {
      await Promise.resolve();
    });

    expect(result.current.loadState).toBe("error");
    expect(setWorkspaceAggregate).not.toHaveBeenCalled();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(30_000);
    });

    expect(result.current.loadState).toBe("loaded");
    expect(getWorkspaceAggregate).toHaveBeenCalledTimes(2);
  });

  it("ignores a response that arrives after the hook unmounts", async () => {
    let resolveRequest: (value: typeof response) => void = () => {};
    getWorkspaceAggregate.mockReturnValue(
      new Promise<typeof response>((resolve) => {
        resolveRequest = resolve;
      }),
    );
    const { unmount } = renderHook(() => useWorkspaceAggregate());
    unmount();

    await act(async () => {
      resolveRequest(response);
      await Promise.resolve();
    });

    expect(setWorkspaceAggregate).not.toHaveBeenCalled();
  });

  it("ignores an old response after the page remounts", async () => {
    const stale = deferred<typeof response>();
    const current = deferred<typeof response>();
    getWorkspaceAggregate.mockReturnValueOnce(stale.promise).mockReturnValueOnce(current.promise);
    const oldPage = renderHook(() => useWorkspaceAggregate());
    oldPage.unmount();
    const { result } = renderHook(() => useWorkspaceAggregate());

    expect(getWorkspaceAggregate).toHaveBeenCalledTimes(2);
    await act(async () => {
      current.resolve({
        ...response,
        workspaces: [{ ...response.workspaces[0], name: "Current workspace" }],
      });
      await current.promise;
    });
    await act(async () => {
      stale.resolve({
        ...response,
        workspaces: [{ ...response.workspaces[0], name: "Stale workspace" }],
      });
      await stale.promise;
    });

    expect(result.current.loadState).toBe("loaded");
    expect(setWorkspaceAggregate).toHaveBeenCalledTimes(1);
    expect(setWorkspaceAggregate).toHaveBeenCalledWith({
      workspaces: [{ ...response.workspaces[0], name: "Current workspace" }],
      recentActivity: [],
    });
  });
});
