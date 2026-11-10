import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { defaultState } from "@/lib/state/default-state";
import type { TaskSession } from "@/lib/types/http";
import type { TurnChangeSetSummary } from "@/lib/types/turn-changes";
import { getWebSocketClient, setWebSocketClient } from "@/lib/ws/connection";
import type { WebSocketClient } from "@/lib/ws/client";
import { clearTurnChangeRequestsForTest, useSessionTurnChanges } from "./use-turn-changes";

const api = vi.hoisted(() => ({ list: vi.fn() }));
vi.mock("@/lib/api/domains/turn-changes-api", () => ({
  listTurnChangeHistory: (...args: unknown[]) => api.list(...args),
  getTurnChangeHistoryItem: vi.fn(),
}));

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

function summary(revision: number): TurnChangeSetSummary {
  return {
    id: "change-1",
    task_id: "task-1",
    session_id: "session-1",
    turn_id: "turn-1",
    revision,
    availability: revision === 1 ? "pending" : "ready",
    complete: revision !== 1,
    summary_complete: revision !== 1,
    content_complete: revision !== 1,
    turn_ordinal: 1,
    fallback_anchor: "turn-changes:turn-1",
    file_count: revision === 1 ? 0 : 1,
    binary_file_count: 0,
    unknown_count_file_count: 0,
    repository_count: 0,
    repositories: [],
  };
}

function page(revision: number) {
  return { change_sets: [summary(revision)], total: 1, offset: 0, limit: 50 };
}

const originalClient = getWebSocketClient();
const initialState = {
  connection: { ...defaultState.connection, status: "connected" as const },
  taskSessions: {
    ...defaultState.taskSessions,
    items: { "session-1": { id: "session-1" } as TaskSession },
  },
};
function wrapper({ children }: { children: ReactNode }) {
  return <StateProvider initialState={initialState}>{children}</StateProvider>;
}

beforeEach(() => {
  api.list.mockReset();
  clearTurnChangeRequestsForTest();
});
afterEach(() => {
  cleanup();
  setWebSocketClient(originalClient);
});

it("refreshes after subscription when an older history read is still in flight", async () => {
  const ready = deferred<void>();
  const initial = deferred<ReturnType<typeof page>>();
  const unsubscribe = vi.fn();
  const subscribe = vi.fn(() => ({ ready: ready.promise, unsubscribe }));
  setWebSocketClient({ subscribeSessionWithReady: subscribe } as unknown as WebSocketClient);
  api.list.mockReturnValueOnce(initial.promise).mockResolvedValueOnce(page(2));
  const { result, unmount } = renderHook(
    () => ({ changes: useSessionTurnChanges("session-1"), store: useAppStoreApi() }),
    { wrapper },
  );
  await waitFor(() => expect(api.list).toHaveBeenCalledTimes(1));
  expect(subscribe).toHaveBeenCalledWith("session-1");
  await act(async () => ready.resolve());
  expect(api.list).toHaveBeenCalledTimes(1);
  await act(async () => initial.resolve(page(1)));
  await waitFor(() =>
    expect(result.current.store.getState().turnChanges.bySession["session-1"]?.[0]?.revision).toBe(
      2,
    ),
  );
  expect(api.list).toHaveBeenCalledTimes(2);
  unmount();
  expect(unsubscribe).toHaveBeenCalledTimes(1);
});

it("does not refresh after the consumer unmounts before subscription readiness", async () => {
  const ready = deferred<void>();
  const unsubscribe = vi.fn();
  setWebSocketClient({
    subscribeSessionWithReady: () => ({ ready: ready.promise, unsubscribe }),
  } as unknown as WebSocketClient);
  api.list.mockResolvedValue(page(1));
  const { unmount } = renderHook(() => useSessionTurnChanges("session-1"), { wrapper });
  await waitFor(() => expect(api.list).toHaveBeenCalledTimes(1));
  unmount();
  await act(async () => ready.resolve());
  expect(api.list).toHaveBeenCalledTimes(1);
  expect(unsubscribe).toHaveBeenCalledTimes(1);
});

it("subscribes only while session detail is active and reconciles when reactivated", async () => {
  const unsubscribe = vi.fn();
  const subscribe = vi.fn(() => ({ ready: Promise.resolve(), unsubscribe }));
  setWebSocketClient({ subscribeSessionWithReady: subscribe } as unknown as WebSocketClient);
  api.list.mockResolvedValue(page(1));
  const { rerender } = renderHook(
    ({ detailActive }: { detailActive: boolean }) =>
      useSessionTurnChanges("session-1", { detailActive }),
    { wrapper, initialProps: { detailActive: false } },
  );
  await waitFor(() => expect(api.list).toHaveBeenCalledTimes(1));
  expect(subscribe).not.toHaveBeenCalled();
  rerender({ detailActive: true });
  await waitFor(() => expect(api.list).toHaveBeenCalledTimes(2));
  expect(subscribe).toHaveBeenCalledTimes(1);
  rerender({ detailActive: false });
  expect(unsubscribe).toHaveBeenCalledTimes(1);
  rerender({ detailActive: true });
  await waitFor(() => expect(api.list).toHaveBeenCalledTimes(3));
  expect(subscribe).toHaveBeenCalledTimes(2);
});

it("keeps a newer live revision when the post-subscription page arrives late", async () => {
  const ready = deferred<void>();
  const refreshed = deferred<ReturnType<typeof page>>();
  setWebSocketClient({
    subscribeSessionWithReady: () => ({ ready: ready.promise, unsubscribe: vi.fn() }),
  } as unknown as WebSocketClient);
  api.list.mockResolvedValueOnce(page(1)).mockReturnValueOnce(refreshed.promise);
  const { result } = renderHook(
    () => ({ changes: useSessionTurnChanges("session-1"), store: useAppStoreApi() }),
    { wrapper },
  );
  await waitFor(() =>
    expect(result.current.store.getState().turnChanges.bySession["session-1"]?.[0]?.revision).toBe(
      1,
    ),
  );
  await act(async () => ready.resolve());
  await waitFor(() => expect(api.list).toHaveBeenCalledTimes(2));
  act(() => result.current.store.getState().mergeTurnChangeSummary("session-1", summary(3)));
  await act(async () => refreshed.resolve(page(2)));
  expect(result.current.store.getState().turnChanges.bySession["session-1"]?.[0]?.revision).toBe(3);
});

it("refreshes again on reconnection and releases each subscription", async () => {
  const first = deferred<void>();
  const second = deferred<void>();
  const unsubscribeFirst = vi.fn();
  const unsubscribeSecond = vi.fn();
  const subscribe = vi
    .fn()
    .mockReturnValueOnce({ ready: first.promise, unsubscribe: unsubscribeFirst })
    .mockReturnValueOnce({ ready: second.promise, unsubscribe: unsubscribeSecond });
  setWebSocketClient({ subscribeSessionWithReady: subscribe } as unknown as WebSocketClient);
  api.list
    .mockResolvedValueOnce(page(1))
    .mockResolvedValueOnce(page(2))
    .mockResolvedValueOnce(page(3));
  const { result, unmount } = renderHook(
    () => ({ changes: useSessionTurnChanges("session-1"), store: useAppStoreApi() }),
    { wrapper },
  );
  await waitFor(() => expect(api.list).toHaveBeenCalledTimes(1));
  await act(async () => first.resolve());
  await waitFor(() =>
    expect(result.current.store.getState().turnChanges.bySession["session-1"]?.[0]?.revision).toBe(
      2,
    ),
  );
  act(() =>
    result.current.store.setState((state) => ({
      connection: { ...state.connection, status: "disconnected" },
    })),
  );
  expect(unsubscribeFirst).toHaveBeenCalledTimes(1);
  act(() =>
    result.current.store.setState((state) => ({
      connection: { ...state.connection, status: "connected" },
    })),
  );
  await waitFor(() => expect(subscribe).toHaveBeenCalledTimes(2));
  await act(async () => second.resolve());
  await waitFor(() =>
    expect(result.current.store.getState().turnChanges.bySession["session-1"]?.[0]?.revision).toBe(
      3,
    ),
  );
  unmount();
  expect(unsubscribeSecond).toHaveBeenCalledTimes(1);
});

it("recovers an initial history error after subscription readiness", async () => {
  const ready = deferred<void>();
  setWebSocketClient({
    subscribeSessionWithReady: () => ({ ready: ready.promise, unsubscribe: vi.fn() }),
  } as unknown as WebSocketClient);
  api.list.mockRejectedValueOnce(new Error("history unavailable")).mockResolvedValueOnce(page(2));
  const { result } = renderHook(
    () => ({ changes: useSessionTurnChanges("session-1"), store: useAppStoreApi() }),
    { wrapper },
  );
  await waitFor(() => expect(result.current.changes.error).toBeInstanceOf(Error));
  await act(async () => ready.resolve());
  await waitFor(() =>
    expect(result.current.store.getState().turnChanges.bySession["session-1"]?.[0]?.revision).toBe(
      2,
    ),
  );
  expect(result.current.changes.error).toBeNull();
  expect(result.current.changes.loading).toBe(false);
});

it("reconciles client installation and replacement without a connection-status change", async () => {
  setWebSocketClient(null);
  const first = deferred<void>();
  const second = deferred<void>();
  const unsubscribeFirst = vi.fn();
  const unsubscribeSecond = vi.fn();
  const subscribeFirst = vi.fn(() => ({ ready: first.promise, unsubscribe: unsubscribeFirst }));
  const subscribeSecond = vi.fn(() => ({ ready: second.promise, unsubscribe: unsubscribeSecond }));
  api.list
    .mockResolvedValueOnce(page(1))
    .mockResolvedValueOnce(page(2))
    .mockResolvedValueOnce(page(3));
  const { result, unmount } = renderHook(
    () => ({ changes: useSessionTurnChanges("session-1"), store: useAppStoreApi() }),
    { wrapper },
  );
  await waitFor(() => expect(api.list).toHaveBeenCalledTimes(1));
  act(() =>
    setWebSocketClient({ subscribeSessionWithReady: subscribeFirst } as unknown as WebSocketClient),
  );
  await waitFor(() => expect(subscribeFirst).toHaveBeenCalledWith("session-1"));
  await act(async () => first.resolve());
  await waitFor(() => expect(result.current.changes.summaries[0]?.revision).toBe(2));
  act(() =>
    setWebSocketClient({
      subscribeSessionWithReady: subscribeSecond,
    } as unknown as WebSocketClient),
  );
  expect(unsubscribeFirst).toHaveBeenCalledTimes(1);
  await waitFor(() => expect(subscribeSecond).toHaveBeenCalledWith("session-1"));
  await act(async () => second.resolve());
  await waitFor(() => expect(result.current.changes.summaries[0]?.revision).toBe(3));
  expect(result.current.store.getState().connection.status).toBe("connected");
  unmount();
  expect(unsubscribeSecond).toHaveBeenCalledTimes(1);
});
