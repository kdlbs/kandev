import { beforeEach, describe, expect, it, onTestFinished, vi } from "vitest";
import { renderHook } from "@testing-library/react";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { TaskSession } from "@/lib/types/http";

const listTaskSessionsMock = vi.fn();
const TASK_ID = "task-cached";
const CACHED_SESSION_ID = "cached-session";

vi.mock("@/lib/api", () => ({
  fetchTask: vi.fn(),
  listTaskSessions: (...args: unknown[]) => listTaskSessionsMock(...args),
}));

import { useTaskRemoval } from "./use-task-removal";

function makeSession(id: string): TaskSession {
  return {
    id,
    task_id: TASK_ID,
    task_environment_id: `${id}-environment`,
    state: "WAITING_FOR_INPUT",
    started_at: "2026-08-15T15:00:00Z",
    updated_at: "2026-08-15T15:00:00Z",
  } as TaskSession;
}

function makeStore(cachedSession: TaskSession, activityEpoch = 0): StoreApi<AppState> {
  const state = {
    taskSessions: {
      items: { [cachedSession.id]: cachedSession },
      activityEpochBySession: { [cachedSession.id]: activityEpoch },
    },
    taskSessionsByTask: {
      itemsByTaskId: { [TASK_ID]: [cachedSession] },
      loadedByTaskId: { [TASK_ID]: true },
      loadingByTaskId: {},
    },
    setTaskSessionsLoading: vi.fn(),
    setTaskSessionsForTask: vi.fn(),
  } as unknown as AppState;
  return {
    getState: () => state,
    setState: vi.fn(),
    subscribe: vi.fn(),
  } as unknown as StoreApi<AppState>;
}

describe("useTaskRemoval session loading", () => {
  beforeEach(() => vi.clearAllMocks());

  it("force-refreshes a previously loaded task session list", async () => {
    const freshSession = makeSession("fresh-session");
    listTaskSessionsMock.mockResolvedValue({ sessions: [freshSession] });
    const { result } = renderHook(() =>
      useTaskRemoval({ store: makeStore(makeSession(CACHED_SESSION_ID)) }),
    );

    const sessions = await result.current.loadTaskSessionsForTask(TASK_ID, { force: true });

    expect(listTaskSessionsMock).toHaveBeenCalledWith(TASK_ID, { cache: "no-store" });
    expect(sessions).toEqual([freshSession]);
  });

  it("rejects a failed forced refresh instead of returning a stale owner", async () => {
    listTaskSessionsMock.mockRejectedValue(new Error("offline"));
    const { result } = renderHook(() =>
      useTaskRemoval({ store: makeStore(makeSession(CACHED_SESSION_ID)) }),
    );

    await expect(result.current.loadTaskSessionsForTask(TASK_ID, { force: true })).rejects.toThrow(
      "offline",
    );
  });

  it("rejects an older forced response after a newer snapshot wins", async () => {
    const consoleError = vi.spyOn(console, "error").mockImplementation(() => undefined);
    onTestFinished(() => consoleError.mockRestore());
    type SessionResponse = { sessions: TaskSession[] };
    let resolveOlder: (response: SessionResponse) => void = () => {};
    let resolveNewer: (response: SessionResponse) => void = () => {};
    const olderResponse = new Promise<SessionResponse>((resolve) => {
      resolveOlder = resolve;
    });
    const newerResponse = new Promise<SessionResponse>((resolve) => {
      resolveNewer = resolve;
    });
    listTaskSessionsMock.mockReturnValueOnce(olderResponse).mockReturnValueOnce(newerResponse);
    const store = makeStore(makeSession(CACHED_SESSION_ID));
    const { result } = renderHook(() => useTaskRemoval({ store }));

    const olderLoad = result.current.loadTaskSessionsForTask(TASK_ID, { force: true });
    const newerLoad = result.current.loadTaskSessionsForTask(TASK_ID, { force: true });
    const newerSession = makeSession("newer-session");
    resolveNewer({ sessions: [newerSession] });
    await expect(newerLoad).resolves.toEqual([newerSession]);
    resolveOlder({ sessions: [makeSession("older-session")] });
    await expect(olderLoad).rejects.toMatchObject({ name: "AbortError" });
    expect(consoleError).not.toHaveBeenCalled();

    expect(store.getState().setTaskSessionsForTask).toHaveBeenCalledTimes(1);
    expect(store.getState().setTaskSessionsForTask).toHaveBeenCalledWith(TASK_ID, [newerSession], {
      [CACHED_SESSION_ID]: 0,
    });
    expect(store.getState().setTaskSessionsLoading).toHaveBeenNthCalledWith(1, TASK_ID, true);
    expect(store.getState().setTaskSessionsLoading).toHaveBeenNthCalledWith(2, TASK_ID, true);
    expect(store.getState().setTaskSessionsLoading).toHaveBeenNthCalledWith(3, TASK_ID, false);
    expect(store.getState().setTaskSessionsLoading).toHaveBeenCalledTimes(3);
  });

  it("captures activity epochs before a deferred forced refresh", async () => {
    type SessionResponse = { sessions: TaskSession[] };
    let resolveResponse: (response: SessionResponse) => void = () => {};
    const response = new Promise<SessionResponse>((resolve) => {
      resolveResponse = resolve;
    });
    listTaskSessionsMock.mockReturnValueOnce(response);
    const cachedSession = makeSession(CACHED_SESSION_ID);
    const store = makeStore(cachedSession, 4);
    const { result } = renderHook(() => useTaskRemoval({ store }));

    const load = result.current.loadTaskSessionsForTask(TASK_ID, { force: true });
    await vi.waitFor(() => expect(listTaskSessionsMock).toHaveBeenCalledOnce());
    store.getState().taskSessions.activityEpochBySession![cachedSession.id] = 5;
    const freshSession = makeSession("fresh-session");
    resolveResponse({ sessions: [freshSession] });

    await expect(load).resolves.toEqual([freshSession]);
    expect(store.getState().setTaskSessionsForTask).toHaveBeenCalledWith(TASK_ID, [freshSession], {
      [cachedSession.id]: 4,
    });
  });
});
