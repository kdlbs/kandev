import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const getMock = vi.fn();
vi.mock("@/lib/api/domains/coordinator-api", () => ({
  getCoordinatorSettings: (...args: unknown[]) => getMock(...args),
}));
vi.mock("@/lib/ws/connection", () => ({ useWebSocketClient: () => null }));

import { useCoordinatorWatchSet, watchSetFromSettings } from "./use-coordinator-watch-set";

const settings = (ids: string[]) => ({
  policy: { actions: {} },
  policy_revision: 1,
  watches: { scope: "selected", workflow_ids: ids },
});

beforeEach(() => vi.clearAllMocks());

describe("useCoordinatorWatchSet", () => {
  it("stays inert while disabled", () => {
    const { result } = renderHook(() => useCoordinatorWatchSet("w", "c", false));
    expect(getMock).not.toHaveBeenCalled();
    expect(result.current.input.value).toBeUndefined();
  });

  it("reads the watch set and lets a stale response lose to a newer one", async () => {
    let resolveFirst!: (v: unknown) => void;
    getMock.mockReturnValueOnce(new Promise((r) => (resolveFirst = r)));
    const { result } = renderHook(() => useCoordinatorWatchSet("w", "c", true));
    getMock.mockResolvedValueOnce(settings(["new"]));
    await act(async () => result.current.retry());
    await waitFor(() => expect(result.current.input.value?.workflowIds).toEqual(["new"]));
    await act(async () => resolveFirst(settings(["old"])));
    expect(result.current.input.value?.workflowIds).toEqual(["new"]);
  });

  it("keeps the last value and flags an error when a re-read fails", async () => {
    getMock.mockResolvedValueOnce(settings(["a"]));
    const { result } = renderHook(() => useCoordinatorWatchSet("w", "c", true));
    await waitFor(() => expect(result.current.input.value).toBeDefined());
    getMock.mockRejectedValueOnce(new Error("x"));
    await act(async () => result.current.retry());
    await waitFor(() => expect(result.current.input.error).toBe(true));
    expect(result.current.input.value?.workflowIds).toEqual(["a"]);
  });

  it("drops the kept value when the coordinator changes", async () => {
    getMock.mockResolvedValueOnce(settings(["a"]));
    const { result, rerender } = renderHook(({ id }) => useCoordinatorWatchSet("w", id, true), {
      initialProps: { id: "c1" },
    });
    await waitFor(() => expect(result.current.input.value).toBeDefined());
    getMock.mockReturnValueOnce(new Promise(() => {}));
    rerender({ id: "c2" });
    expect(result.current.input.value).toBeUndefined();
  });
});

describe("watchSetFromSettings", () => {
  const base = { policy: { actions: {} }, policy_revision: 1 } as never;
  it("carries no projects when the stored scope is all", () => {
    const set = watchSetFromSettings({
      ...(base as object),
      watches: { scope: "all", workflow_ids: [] },
    } as never);
    expect(set.projects).toBeUndefined();
  });

  it("maps a selected scope to camelCase and keeps an unknown list as null", () => {
    const make = (ids: string[] | null) =>
      watchSetFromSettings({
        ...(base as object),
        watches: { scope: "all", workflow_ids: [] },
        projects: { scope: "selected", repository_ids: ids, include_no_repository: true },
      } as never);
    expect(make(["r1"]).projects).toEqual({
      scope: "selected",
      repositoryIds: ["r1"],
      includeNoRepository: true,
    });
    expect(make(null).projects?.repositoryIds).toBeNull();
  });
});
