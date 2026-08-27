import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const { mockListWorkspaces, storeState } = vi.hoisted(() => ({
  mockListWorkspaces: vi.fn(),
  storeState: {
    workspaces: { items: [] as { id: string; name: string }[], activeId: null },
    setWorkspaces: vi.fn(),
  },
}));

vi.mock("@/lib/api/domains/workspace-api", () => ({ listWorkspaces: mockListWorkspaces }));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof storeState) => unknown) => selector(storeState),
}));

import { useWorkspaceDestinations } from "./use-workspace-destinations";

/** Deferred promise helper for controlling async test resolution. */
function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((nextResolve, nextReject) => {
    resolve = nextResolve;
    reject = nextReject;
  });
  return { promise, resolve, reject };
}

describe("useWorkspaceDestinations", () => {
  beforeEach(() => {
    mockListWorkspaces.mockReset();
    storeState.setWorkspaces.mockClear();
    storeState.workspaces.items = [];
  });

  it("fetches workspaces when the store list is empty and writes them to the store", async () => {
    const pending = deferred<{ workspaces: { id: string; name: string }[]; total: number }>();
    mockListWorkspaces.mockReturnValue(pending.promise);

    const view = renderHook(() => useWorkspaceDestinations());
    expect(view.result.current.loading).toBe(true);
    expect(view.result.current.error).toBeNull();

    await act(async () =>
      pending.resolve({
        workspaces: [
          { id: "ws-1", name: "One" },
          { id: "ws-2", name: "Two" },
        ],
        total: 2,
      }),
    );
    await waitFor(() => expect(view.result.current.loading).toBe(false));
    const stored = storeState.setWorkspaces.mock.calls[0][0] as {
      id: string;
      name: string;
    }[];
    expect(stored.map((item) => [item.id, item.name])).toEqual([
      ["ws-1", "One"],
      ["ws-2", "Two"],
    ]);
    expect(view.result.current.error).toBeNull();
  });

  it("does not fetch when the store already has workspaces", () => {
    storeState.workspaces.items = [{ id: "ws-1", name: "One" }];
    const view = renderHook(() => useWorkspaceDestinations());
    expect(mockListWorkspaces).not.toHaveBeenCalled();
    expect(view.result.current.error).toBeNull();
  });

  it("preserves failures as an error and retry re-runs the fetch", async () => {
    const first = deferred<{ workspaces: never[]; total: number }>();
    mockListWorkspaces.mockReturnValueOnce(first.promise);

    const view = renderHook(() => useWorkspaceDestinations());
    await act(async () => first.reject(new Error("boom")));
    await waitFor(() => expect(view.result.current.error).not.toBeNull());
    expect(storeState.setWorkspaces).not.toHaveBeenCalled();

    const second = deferred<{ workspaces: { id: string; name: string }[]; total: number }>();
    mockListWorkspaces.mockReturnValueOnce(second.promise);
    act(() => view.result.current.retry());
    await act(async () => second.resolve({ workspaces: [{ id: "ws-1", name: "One" }], total: 1 }));
    await waitFor(() => expect(view.result.current.error).toBeNull());
    expect(storeState.setWorkspaces).toHaveBeenCalledTimes(1);
  });

  it("stops fetching once a hydration or retry populated the store", async () => {
    const pending = deferred<{ workspaces: never[]; total: number }>();
    mockListWorkspaces.mockReturnValue(pending.promise);

    const view = renderHook(() => useWorkspaceDestinations());
    expect(view.result.current.loading).toBe(true);
    storeState.workspaces.items = [{ id: "ws-1", name: "One" }];
    view.rerender();
    await act(async () => pending.reject(new Error("late failure")));
    // The store was populated meanwhile, so the stale failure must not
    // surface as an error, must not trigger another fetch, and must not leave
    // loading stuck true (which would disable submission forever).
    await waitFor(() => expect(view.result.current.error).toBeNull());
    expect(view.result.current.loading).toBe(false);
    expect(mockListWorkspaces).toHaveBeenCalledTimes(1);
  });
});
