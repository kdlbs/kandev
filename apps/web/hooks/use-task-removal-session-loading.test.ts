import { beforeEach, describe, expect, it, vi } from "vitest";
import { renderHook } from "@testing-library/react";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { TaskSession } from "@/lib/types/http";

const listTaskSessionsMock = vi.fn();
const TASK_ID = "task-cached";

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

function makeStore(cachedSession: TaskSession): StoreApi<AppState> {
  const state = {
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
      useTaskRemoval({ store: makeStore(makeSession("cached-session")) }),
    );

    const sessions = await result.current.loadTaskSessionsForTask(TASK_ID, { force: true });

    expect(listTaskSessionsMock).toHaveBeenCalledWith(TASK_ID, { cache: "no-store" });
    expect(sessions).toEqual([freshSession]);
  });

  it("rejects a failed forced refresh instead of returning a stale owner", async () => {
    listTaskSessionsMock.mockRejectedValue(new Error("offline"));
    const { result } = renderHook(() =>
      useTaskRemoval({ store: makeStore(makeSession("cached-session")) }),
    );

    await expect(result.current.loadTaskSessionsForTask(TASK_ID, { force: true })).rejects.toThrow(
      "offline",
    );
  });

  it("rejects an older forced response after a newer snapshot wins", async () => {
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
    const store = makeStore(makeSession("cached-session"));
    const { result } = renderHook(() => useTaskRemoval({ store }));

    const olderLoad = result.current.loadTaskSessionsForTask(TASK_ID, { force: true });
    const newerLoad = result.current.loadTaskSessionsForTask(TASK_ID, { force: true });
    const newerSession = makeSession("newer-session");
    resolveNewer({ sessions: [newerSession] });
    await expect(newerLoad).resolves.toEqual([newerSession]);
    resolveOlder({ sessions: [makeSession("older-session")] });
    await expect(olderLoad).rejects.toMatchObject({ name: "AbortError" });

    expect(store.getState().setTaskSessionsForTask).toHaveBeenCalledTimes(1);
    expect(store.getState().setTaskSessionsForTask).toHaveBeenCalledWith(TASK_ID, [newerSession]);
    expect(store.getState().setTaskSessionsLoading).toHaveBeenNthCalledWith(1, TASK_ID, true);
    expect(store.getState().setTaskSessionsLoading).toHaveBeenNthCalledWith(2, TASK_ID, true);
    expect(store.getState().setTaskSessionsLoading).toHaveBeenNthCalledWith(3, TASK_ID, false);
    expect(store.getState().setTaskSessionsLoading).toHaveBeenCalledTimes(3);
  });
});
