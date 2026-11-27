import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const getCoordinator = vi.hoisted(() => vi.fn());
vi.mock("@/lib/api/domains/coordinator-api", () => ({ getCoordinator }));

import { useCoordinatorWatches } from "./use-coordinator-watches";

const SELECTED = { scope: "selected", workflow_ids: ["wf-1"] };

function setup(initial: Partial<Parameters<typeof useCoordinatorWatches>[0]> = {}) {
  const onGone = vi.fn();
  const hook = renderHook(
    (props: Parameters<typeof useCoordinatorWatches>[0]) => useCoordinatorWatches(props),
    {
      initialProps: {
        workspaceId: "ws-1",
        coordinatorId: "c-1",
        routeKey: "kanban:wf-1",
        enabled: true,
        onGone,
        ...initial,
      },
    },
  );
  return { ...hook, onGone };
}

beforeEach(() => getCoordinator.mockReset());
afterEach(() => cleanup());

describe("useCoordinatorWatches", () => {
  it("reads nothing while disabled", () => {
    setup({ enabled: false });
    expect(getCoordinator).not.toHaveBeenCalled();
  });

  it("reports loaded watches", async () => {
    getCoordinator.mockResolvedValue({ id: "c-1", watches: SELECTED });
    const { result } = setup();
    await waitFor(() => expect(result.current.loaded).toBe(true));
    expect(result.current.watches).toEqual(SELECTED);
  });

  it("reports loaded with no watches when the field is absent", async () => {
    getCoordinator.mockResolvedValue({ id: "c-1" });
    const { result } = setup();
    await waitFor(() => expect(result.current.loaded).toBe(true));
    expect(result.current.watches).toBeUndefined();
  });

  it("re-reads on a route key change and keeps the last value while it fails", async () => {
    getCoordinator.mockResolvedValueOnce({ id: "c-1", watches: SELECTED });
    const { result, rerender } = setup();
    await waitFor(() => expect(result.current.loaded).toBe(true));

    getCoordinator.mockRejectedValueOnce(new Error("offline"));
    rerender({
      workspaceId: "ws-1",
      coordinatorId: "c-1",
      routeKey: "kanban:wf-2",
      enabled: true,
      onGone: vi.fn(),
    });
    await waitFor(() => expect(getCoordinator).toHaveBeenCalledTimes(2));
    expect(result.current.watches).toEqual(SELECTED);
  });

  it("drops an older response that lands after a newer request", async () => {
    let resolveFirst: (v: unknown) => void = () => {};
    getCoordinator.mockImplementationOnce(() => new Promise((r) => (resolveFirst = r)));
    getCoordinator.mockResolvedValueOnce({
      id: "c-1",
      watches: { scope: "all", workflow_ids: [] },
    });
    const { result, rerender } = setup();
    rerender({
      workspaceId: "ws-1",
      coordinatorId: "c-1",
      routeKey: "kanban:wf-2",
      enabled: true,
      onGone: vi.fn(),
    });
    await waitFor(() => expect(result.current.loaded).toBe(true));
    await act(async () => resolveFirst({ id: "c-1", watches: SELECTED }));
    expect(result.current.watches).toEqual({ scope: "all", workflow_ids: [] });
  });

  it("does not show another coordinator's watches after a switch", async () => {
    getCoordinator.mockResolvedValueOnce({ id: "c-1", watches: SELECTED });
    const { result, rerender } = setup();
    await waitFor(() => expect(result.current.loaded).toBe(true));
    getCoordinator.mockReturnValueOnce(new Promise(() => {}));
    rerender({
      workspaceId: "ws-1",
      coordinatorId: "c-2",
      routeKey: "kanban:wf-1",
      enabled: true,
      onGone: vi.fn(),
    });
    expect(result.current.loaded).toBe(false);
  });
});
