import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { TaskSession } from "@/lib/types/http";
import { sessionId, taskId } from "@/lib/types/ids";

const apiMock = vi.hoisted(() => ({
  listTaskSessions: vi.fn(),
}));

type MockTaskSessionsState = {
  taskSessions: {
    activityEpochBySession: Record<string, number>;
    readCursorEpochBySession: Record<string, number>;
  };
  taskSessionsByTask: {
    itemsByTaskId: Record<string, TaskSession[]>;
    loadingByTaskId: Record<string, boolean>;
    loadedByTaskId: Record<string, boolean>;
    errorByTaskId: Record<string, string | null>;
  };
  connection: { status: string };
  auth: {
    mode: string;
    authenticated: boolean;
    user: { id: string } | null;
  };
  workspaceContextGeneration: number;
  setTaskSessionsForTask: ReturnType<typeof vi.fn>;
  setTaskSessionsLoading: ReturnType<typeof vi.fn>;
  setTaskSessionsError: ReturnType<typeof vi.fn>;
};

let mockState: MockTaskSessionsState;

vi.mock("@/components/state-provider", () => {
  const getState = () => mockState;
  const storeApi = { getState };
  return {
    useAppStore: (selector: (state: MockTaskSessionsState) => unknown) => selector(mockState),
    useAppStoreApi: () => storeApi,
  };
});

vi.mock("@/lib/api", () => apiMock);

import { useTaskSessions } from "./use-task-sessions";

const TASK_ID = taskId("task-1");
const SERVICE_UNAVAILABLE = "service unavailable";

function session(id: string, state: TaskSession["state"] = "RUNNING"): TaskSession {
  return {
    id: sessionId(id),
    task_id: TASK_ID,
    state,
    started_at: "2026-06-27T00:00:00Z",
    updated_at: "2026-06-27T00:00:00Z",
  };
}

function setDocumentVisibility(value: DocumentVisibilityState) {
  Object.defineProperty(document, "visibilityState", {
    configurable: true,
    value,
  });
}

function resetMockState() {
  mockState = {
    taskSessions: { activityEpochBySession: {}, readCursorEpochBySession: {} },
    taskSessionsByTask: {
      itemsByTaskId: {},
      loadingByTaskId: {},
      loadedByTaskId: {},
      errorByTaskId: {},
    },
    connection: { status: "connected" },
    auth: { mode: "optional", authenticated: false, user: null },
    workspaceContextGeneration: 0,
    setTaskSessionsForTask: vi.fn(),
    setTaskSessionsLoading: vi.fn(),
    setTaskSessionsError: vi.fn(),
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: unknown) => void;
  const promise = new Promise<T>((innerResolve, innerReject) => {
    resolve = innerResolve;
    reject = innerReject;
  });
  return { promise, resolve, reject };
}

beforeEach(() => {
  apiMock.listTaskSessions.mockReset();
  resetMockState();
  setDocumentVisibility("visible");
  apiMock.listTaskSessions.mockResolvedValue({ sessions: [session("sess-1")] });
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.clearAllMocks();
});

describe("useTaskSessions read retirement", () => {
  it.each(["user", "workspace"])(
    "does not merge a retired %s response or settle its successor",
    async (change) => {
      const oldResponse = deferred<{ sessions: TaskSession[] }>();
      const newResponse = deferred<{ sessions: TaskSession[] }>();
      apiMock.listTaskSessions
        .mockReturnValueOnce(oldResponse.promise)
        .mockReturnValueOnce(newResponse.promise);
      const oldHook = renderHook(() => useTaskSessions(TASK_ID));
      await waitFor(() => expect(apiMock.listTaskSessions).toHaveBeenCalledTimes(1));
      if (change === "user") mockState.auth.user = { id: "new-user" };
      else mockState.workspaceContextGeneration++;
      oldHook.unmount();
      const newHook = renderHook(() => useTaskSessions(TASK_ID));
      await waitFor(() => expect(apiMock.listTaskSessions).toHaveBeenCalledTimes(2));
      mockState.setTaskSessionsLoading.mockClear();
      await act(async () => oldResponse.resolve({ sessions: [session("obsolete")] }));
      expect(mockState.setTaskSessionsForTask).not.toHaveBeenCalled();
      expect(mockState.setTaskSessionsError).not.toHaveBeenCalled();
      expect(mockState.setTaskSessionsLoading).not.toHaveBeenCalledWith(TASK_ID, false);
      await act(async () => newResponse.resolve({ sessions: [session("successor")] }));
      expect(mockState.setTaskSessionsForTask).toHaveBeenCalledWith(
        TASK_ID,
        [session("successor")],
        {},
      );
      newHook.unmount();
    },
  );

  it("releases retired loading ownership so a still-mounted hook can start in the new scope", async () => {
    const oldResponse = deferred<{ sessions: TaskSession[] }>();
    const newResponse = deferred<{ sessions: TaskSession[] }>();
    apiMock.listTaskSessions
      .mockReturnValueOnce(oldResponse.promise)
      .mockReturnValueOnce(newResponse.promise);
    mockState.setTaskSessionsLoading.mockImplementation((id: string, loading: boolean) => {
      mockState.taskSessionsByTask.loadingByTaskId[id] = loading;
    });
    const hook = renderHook(() => useTaskSessions(TASK_ID));
    await waitFor(() => expect(apiMock.listTaskSessions).toHaveBeenCalledTimes(1));
    expect(mockState.taskSessionsByTask.loadingByTaskId[TASK_ID]).toBe(true);
    mockState.workspaceContextGeneration++;
    hook.rerender();
    expect(mockState.taskSessionsByTask.loadingByTaskId[TASK_ID]).toBe(false);
    hook.rerender();
    await waitFor(() => expect(apiMock.listTaskSessions).toHaveBeenCalledTimes(2));
    mockState.setTaskSessionsLoading.mockClear();
    await act(async () => oldResponse.resolve({ sessions: [session("retired")] }));
    expect(mockState.setTaskSessionsForTask).not.toHaveBeenCalled();
    expect(mockState.taskSessionsByTask.loadingByTaskId[TASK_ID]).toBe(true);
    expect(mockState.setTaskSessionsLoading).not.toHaveBeenCalled();
    await act(async () => newResponse.resolve({ sessions: [session("successor")] }));
    expect(mockState.setTaskSessionsForTask).toHaveBeenCalledWith(
      TASK_ID,
      [session("successor")],
      {},
    );
    expect(mockState.taskSessionsByTask.loadingByTaskId[TASK_ID]).toBe(false);
    hook.unmount();
  });

  it("does not commit a response after the final consumer unmounts", async () => {
    const response = deferred<{ sessions: TaskSession[] }>();
    apiMock.listTaskSessions.mockReturnValueOnce(response.promise);
    const hook = renderHook(() => useTaskSessions(TASK_ID));
    await waitFor(() => expect(apiMock.listTaskSessions).toHaveBeenCalledTimes(1));
    hook.unmount();
    mockState.setTaskSessionsLoading.mockClear();
    await act(async () => response.resolve({ sessions: [session("obsolete")] }));
    expect(mockState.setTaskSessionsForTask).not.toHaveBeenCalled();
    expect(mockState.setTaskSessionsError).not.toHaveBeenCalled();
    expect(mockState.setTaskSessionsLoading).not.toHaveBeenCalled();
  });
});

describe("useTaskSessions retained and successor consumers", () => {
  it("does not publish a retired transport error or settle a successor read", async () => {
    const oldResponse = deferred<{ sessions: TaskSession[] }>();
    const newResponse = deferred<{ sessions: TaskSession[] }>();
    apiMock.listTaskSessions
      .mockReturnValueOnce(oldResponse.promise)
      .mockReturnValueOnce(newResponse.promise);
    const oldHook = renderHook(() => useTaskSessions(TASK_ID));
    await waitFor(() => expect(apiMock.listTaskSessions).toHaveBeenCalledTimes(1));
    mockState.workspaceContextGeneration++;
    oldHook.unmount();
    const successor = renderHook(() => useTaskSessions(TASK_ID));
    await waitFor(() => expect(apiMock.listTaskSessions).toHaveBeenCalledTimes(2));
    mockState.setTaskSessionsLoading.mockClear();
    await act(async () => oldResponse.reject(new Error(SERVICE_UNAVAILABLE)));
    expect(mockState.setTaskSessionsError).not.toHaveBeenCalled();
    expect(mockState.setTaskSessionsLoading).not.toHaveBeenCalled();
    await act(async () => newResponse.resolve({ sessions: [session("successor")] }));
    expect(mockState.setTaskSessionsForTask).toHaveBeenCalledWith(
      TASK_ID,
      [session("successor")],
      {},
    );
    successor.unmount();
  });

  it("keeps the shared read live after only one consumer unmounts", async () => {
    const response = deferred<{ sessions: TaskSession[] }>();
    apiMock.listTaskSessions.mockReturnValueOnce(response.promise);
    const first = renderHook(() => useTaskSessions(TASK_ID));
    const second = renderHook(() => useTaskSessions(TASK_ID));
    await waitFor(() => expect(apiMock.listTaskSessions).toHaveBeenCalledTimes(1));
    first.unmount();
    await act(async () => response.resolve({ sessions: [session("retained")] }));
    expect(mockState.setTaskSessionsForTask).toHaveBeenCalledTimes(1);
    expect(mockState.setTaskSessionsForTask).toHaveBeenCalledWith(
      TASK_ID,
      [session("retained")],
      {},
    );
    second.unmount();
  });
});
