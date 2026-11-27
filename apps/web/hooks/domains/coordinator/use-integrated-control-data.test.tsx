import type { ReactNode } from "react";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { useIntegratedControlData } from "./use-integrated-control-data";

const api = vi.hoisted(() => ({
  workspaces: vi.fn(),
  coordinators: vi.fn(),
  handlers: new Map<string, Set<() => void>>(),
}));
vi.mock("@/lib/api", async (original) => ({
  ...(await original<typeof import("@/lib/api")>()),
  listWorkspaces: api.workspaces,
}));
vi.mock("@/lib/api/domains/coordinator-api", () => ({ listCoordinators: api.coordinators }));
vi.mock("@/lib/ws/connection", () => ({ useWebSocketClient: () => client }));
const client = {
  on: (event: string, callback: () => void) => {
    const callbacks = api.handlers.get(event) ?? new Set();
    api.handlers.set(event, callbacks);
    callbacks.add(callback);
    return () => callbacks.delete(callback);
  },
};
const beta = { id: "beta", name: "Beta", scopes: ["workspace.read", "workspace.manage"] };
const coordinator = { id: "coordinator", workspace_id: "beta", conversation_task_id: "task" };
function wrapper({ children }: { children: ReactNode }) {
  return (
    <StateProvider
      initialState={{
        workspaces: {
          activeId: "alpha",
          items: [
            {
              id: "alpha",
              name: "Live Alpha",
              owner_id: "owner",
              created_at: "2026-01-01",
              updated_at: "2026-01-01",
            },
          ],
        },
      }}
    >
      {children}
    </StateProvider>
  );
}
function mount(enabled = true, coordinators = true) {
  return renderHook(
    ({ enabled, coordinators }) => ({
      ...useIntegratedControlData(enabled, coordinators),
      store: useAppStoreApi(),
    }),
    { wrapper, initialProps: { enabled, coordinators } },
  );
}
function emit(event: string) {
  act(() => api.handlers.get(event)?.forEach((callback) => callback()));
}
beforeEach(() => {
  api.handlers.clear();
  api.workspaces.mockReset().mockResolvedValue({ workspaces: [beta] });
  api.coordinators.mockReset().mockResolvedValue({ coordinators: [coordinator] });
});
afterEach(cleanup);

describe("Integrated control authorized read lifetime", () => {
  it("OFF performs no reads or subscriptions on mount or foreground events", async () => {
    const { result } = mount(false);
    await act(async () => {
      window.dispatchEvent(new Event("focus"));
      window.dispatchEvent(new Event("online"));
      document.dispatchEvent(new Event("visibilitychange"));
      await result.current.refresh();
    });
    expect(api.workspaces).not.toHaveBeenCalled();
    expect(api.coordinators).not.toHaveBeenCalled();
    expect(api.handlers.size).toBe(0);
    expect(result.current.data).toBeUndefined();
  });
  it("reads only authorized workspaces and preserves the ordinary workspace projection", async () => {
    api.workspaces.mockResolvedValue({
      workspaces: [beta, { id: "denied", name: "Denied", scopes: [] }],
    });
    const { result } = mount();
    const ordinary = result.current.store.getState().workspaces;
    await waitFor(() => expect(result.current.data?.byWorkspace.beta.items).toEqual([coordinator]));
    expect(result.current.data?.workspaces.map((item) => item.id)).toEqual(["beta"]);
    expect(api.coordinators).toHaveBeenCalledTimes(1);
    expect(api.coordinators.mock.calls[0][0]).toBe("beta");
    expect(result.current.store.getState().workspaces).toBe(ordinary);
  });
  it("does not discover coordinators when their separate feature is off", async () => {
    const { result } = mount(true, false);
    await waitFor(() => expect(result.current.data?.workspaces).toHaveLength(1));
    expect(result.current.data?.byWorkspace).toEqual({});
    expect(api.coordinators).not.toHaveBeenCalled();
  });
  it("distinguishes a denied collection from an empty successful collection", async () => {
    api.coordinators.mockRejectedValueOnce(new Error("denied"));
    const { result } = mount();
    await waitFor(() => expect(result.current.data?.byWorkspace.beta.error).toBe(true));
    expect(result.current.data?.byWorkspace.beta.items).toEqual([]);
    api.coordinators.mockResolvedValueOnce({ coordinators: [] });
    await act(async () => result.current.refresh());
    expect(result.current.data?.byWorkspace.beta).toEqual({ items: [], error: false });
  });
  it("invalidates pending reads and removes listeners on unmount", async () => {
    const pending = Promise.withResolvers<{ coordinators: (typeof coordinator)[] }>();
    api.coordinators.mockReturnValueOnce(pending.promise);
    const { result, unmount } = mount();
    await waitFor(() => expect(api.coordinators).toHaveBeenCalledTimes(1));
    const signal = api.coordinators.mock.calls[0][1].init.signal as AbortSignal;
    const refresh = result.current.refresh;
    unmount();
    expect(signal.aborted).toBe(true);
    expect([...api.handlers.values()].every((callbacks) => callbacks.size === 0)).toBe(true);
    await act(async () => {
      pending.resolve({ coordinators: [coordinator] });
      await refresh();
    });
    expect(api.workspaces).toHaveBeenCalledTimes(1);
  });
  it("discards older refreshes and publishes only the latest live response", async () => {
    const first = Promise.withResolvers<{ coordinators: (typeof coordinator)[] }>();
    api.coordinators.mockReturnValueOnce(first.promise);
    const { result } = mount();
    await waitFor(() => expect(api.coordinators).toHaveBeenCalledTimes(1));
    api.coordinators.mockResolvedValueOnce({ coordinators: [{ ...coordinator, id: "latest" }] });
    emit("coordinator.updated");
    await waitFor(() => expect(result.current.data?.byWorkspace.beta.items[0].id).toBe("latest"));
    await act(async () => first.resolve({ coordinators: [coordinator] }));
    expect(result.current.data?.byWorkspace.beta.items[0].id).toBe("latest");
  });
  it("masks and reloads data after an authorization generation changes", async () => {
    const { result } = mount();
    await waitFor(() => expect(result.current.data).toBeDefined());
    const pending = Promise.withResolvers<{ coordinators: (typeof coordinator)[] }>();
    api.coordinators.mockReturnValueOnce(pending.promise);
    act(() =>
      result.current.store.setState((state) => ({
        workspaceContextGeneration: state.workspaceContextGeneration + 1,
      })),
    );
    expect(result.current.data).toBeUndefined();
    await waitFor(() => expect(api.coordinators).toHaveBeenCalledTimes(2));
    await act(async () => pending.resolve({ coordinators: [] }));
    expect(result.current.data?.byWorkspace.beta.items).toEqual([]);
  });
  it("clears selectable data after a failed workspace refresh and remains retryable", async () => {
    const { result } = mount();
    await waitFor(() => expect(result.current.data).toBeDefined());
    api.workspaces.mockRejectedValueOnce(new Error("denied"));
    await act(async () => result.current.refresh());
    expect(result.current.error).toBe(true);
    expect(result.current.data).toBeUndefined();
    await act(async () => result.current.refresh());
    expect(result.current.error).toBe(false);
    expect(result.current.data).toBeDefined();
  });
});
