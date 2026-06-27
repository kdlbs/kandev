import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { TaskSession } from "@/lib/types/http";
import { sessionId, taskId } from "@/lib/types/ids";

const apiMock = vi.hoisted(() => ({
  listTaskSessions: vi.fn(),
}));

type MockTaskSessionsState = {
  taskSessionsByTask: {
    itemsByTaskId: Record<string, TaskSession[]>;
    loadingByTaskId: Record<string, boolean>;
    loadedByTaskId: Record<string, boolean>;
  };
  connection: { status: string };
  setTaskSessionsForTask: ReturnType<typeof vi.fn>;
  setTaskSessionsLoading: ReturnType<typeof vi.fn>;
};

let mockState: MockTaskSessionsState;

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: MockTaskSessionsState) => unknown) => selector(mockState),
}));

vi.mock("@/lib/api", () => apiMock);

import { useTaskSessions } from "./use-task-sessions";

const TASK_ID = taskId("task-1");

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
    taskSessionsByTask: {
      itemsByTaskId: {},
      loadingByTaskId: {},
      loadedByTaskId: {},
    },
    connection: { status: "connected" },
    setTaskSessionsForTask: vi.fn(),
    setTaskSessionsLoading: vi.fn(),
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((innerResolve) => {
    resolve = innerResolve;
  });
  return { promise, resolve };
}

describe("useTaskSessions", () => {
  beforeEach(() => {
    resetMockState();
    setDocumentVisibility("visible");
    apiMock.listTaskSessions.mockResolvedValue({ sessions: [session("sess-1")] });
  });

  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it("loads sessions on mount", async () => {
    renderHook(() => useTaskSessions(TASK_ID));

    await waitFor(() =>
      expect(apiMock.listTaskSessions).toHaveBeenCalledWith(TASK_ID, {
        cache: "no-store",
      }),
    );
    expect(mockState.setTaskSessionsForTask).toHaveBeenCalledWith(TASK_ID, [session("sess-1")]);
  });

  it("loads sessions on mount when the WebSocket is disconnected", async () => {
    mockState.connection.status = "disconnected";

    renderHook(() => useTaskSessions(TASK_ID));

    await waitFor(() =>
      expect(apiMock.listTaskSessions).toHaveBeenCalledWith(TASK_ID, {
        cache: "no-store",
      }),
    );
    expect(mockState.setTaskSessionsForTask).toHaveBeenCalledWith(TASK_ID, [session("sess-1")]);
  });
});

describe("useTaskSessions refreshes", () => {
  beforeEach(() => {
    resetMockState();
    setDocumentVisibility("visible");
    apiMock.listTaskSessions.mockResolvedValue({ sessions: [session("sess-1")] });
  });

  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it("refetches a loaded session list when the WebSocket reconnects", async () => {
    mockState.connection.status = "disconnected";
    mockState.taskSessionsByTask.itemsByTaskId[TASK_ID] = [session("old", "RUNNING")];
    mockState.taskSessionsByTask.loadedByTaskId[TASK_ID] = true;
    apiMock.listTaskSessions.mockResolvedValueOnce({ sessions: [session("old", "COMPLETED")] });

    const { rerender } = renderHook(() => useTaskSessions(TASK_ID));
    await act(async () => {});
    expect(apiMock.listTaskSessions).not.toHaveBeenCalled();

    mockState.connection.status = "connected";
    rerender();

    await waitFor(() =>
      expect(apiMock.listTaskSessions).toHaveBeenCalledWith(TASK_ID, {
        cache: "no-store",
      }),
    );
    expect(mockState.setTaskSessionsForTask).toHaveBeenCalledWith(TASK_ID, [
      session("old", "COMPLETED"),
    ]);
  });

  it("preserves a loaded session list when a reconnect refetch fails", async () => {
    const error = new Error("network down");
    const consoleError = vi.spyOn(console, "error").mockImplementation(() => undefined);
    mockState.connection.status = "disconnected";
    mockState.taskSessionsByTask.itemsByTaskId[TASK_ID] = [session("old", "RUNNING")];
    mockState.taskSessionsByTask.loadedByTaskId[TASK_ID] = true;
    apiMock.listTaskSessions.mockRejectedValueOnce(error);

    const { rerender } = renderHook(() => useTaskSessions(TASK_ID));
    await act(async () => {});

    mockState.connection.status = "connected";
    rerender();

    await waitFor(() =>
      expect(apiMock.listTaskSessions).toHaveBeenCalledWith(TASK_ID, {
        cache: "no-store",
      }),
    );
    expect(mockState.setTaskSessionsForTask).not.toHaveBeenCalled();
    expect(consoleError).toHaveBeenCalledWith("Failed to load task sessions:", error);
  });

  it("resolves a queued forced reload after the deferred request finishes", async () => {
    mockState.taskSessionsByTask.itemsByTaskId[TASK_ID] = [session("old", "RUNNING")];
    mockState.taskSessionsByTask.loadedByTaskId[TASK_ID] = true;
    mockState.taskSessionsByTask.loadingByTaskId[TASK_ID] = true;
    apiMock.listTaskSessions.mockResolvedValueOnce({ sessions: [session("old", "COMPLETED")] });

    const { result, rerender } = renderHook(() => useTaskSessions(TASK_ID));
    let resolved = false;
    const queuedReload = result.current.loadSessions(true).then(() => {
      resolved = true;
    });
    await act(async () => {});
    expect(resolved).toBe(false);
    expect(apiMock.listTaskSessions).not.toHaveBeenCalled();

    mockState.taskSessionsByTask.loadingByTaskId[TASK_ID] = false;
    await act(async () => {
      rerender();
    });

    await waitFor(() => expect(resolved).toBe(true));
    await queuedReload;
    expect(mockState.setTaskSessionsForTask).toHaveBeenCalledWith(TASK_ID, [
      session("old", "COMPLETED"),
    ]);
  });

  it("waits for the follow-up forced reload when another forced reload is running", async () => {
    mockState.taskSessionsByTask.itemsByTaskId[TASK_ID] = [session("old", "RUNNING")];
    mockState.taskSessionsByTask.loadedByTaskId[TASK_ID] = true;
    mockState.setTaskSessionsLoading.mockImplementation((id: string, loading: boolean) => {
      mockState.taskSessionsByTask.loadingByTaskId[id] = loading;
    });
    const firstResponse = deferred<{ sessions: TaskSession[] }>();
    apiMock.listTaskSessions
      .mockReturnValueOnce(firstResponse.promise)
      .mockResolvedValueOnce({ sessions: [session("old", "COMPLETED")] });

    const { result, rerender } = renderHook(() => useTaskSessions(TASK_ID));
    const firstReload = result.current.loadSessions(true);
    rerender();
    let queuedResolved = false;
    const queuedReload = result.current.loadSessions(true).then(() => {
      queuedResolved = true;
    });

    firstResponse.resolve({ sessions: [session("old", "RUNNING")] });
    await firstReload;
    rerender();

    expect(queuedResolved).toBe(false);
    await waitFor(() => expect(queuedResolved).toBe(true));
    await queuedReload;
    expect(apiMock.listTaskSessions).toHaveBeenCalledTimes(2);
    expect(mockState.setTaskSessionsForTask).toHaveBeenLastCalledWith(TASK_ID, [
      session("old", "COMPLETED"),
    ]);
  });
});

describe("useTaskSessions foreground refreshes", () => {
  beforeEach(() => {
    resetMockState();
    setDocumentVisibility("visible");
    apiMock.listTaskSessions.mockResolvedValue({ sessions: [session("sess-1")] });
  });

  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it("refetches a loaded session list when a suspended tab becomes visible again", async () => {
    mockState.taskSessionsByTask.itemsByTaskId[TASK_ID] = [session("old", "RUNNING")];
    mockState.taskSessionsByTask.loadedByTaskId[TASK_ID] = true;
    apiMock.listTaskSessions.mockResolvedValueOnce({ sessions: [session("old", "COMPLETED")] });

    renderHook(() => useTaskSessions(TASK_ID));
    await act(async () => {});
    expect(apiMock.listTaskSessions).not.toHaveBeenCalled();

    document.dispatchEvent(new Event("visibilitychange"));

    await waitFor(() =>
      expect(apiMock.listTaskSessions).toHaveBeenCalledWith(TASK_ID, {
        cache: "no-store",
      }),
    );
    expect(mockState.setTaskSessionsForTask).toHaveBeenCalledWith(TASK_ID, [
      session("old", "COMPLETED"),
    ]);
  });

  it("runs a forced foreground refetch after an older request finishes", async () => {
    mockState.taskSessionsByTask.itemsByTaskId[TASK_ID] = [session("old", "RUNNING")];
    mockState.taskSessionsByTask.loadedByTaskId[TASK_ID] = true;
    mockState.taskSessionsByTask.loadingByTaskId[TASK_ID] = true;
    apiMock.listTaskSessions.mockResolvedValueOnce({ sessions: [session("old", "COMPLETED")] });

    const { rerender } = renderHook(() => useTaskSessions(TASK_ID));
    await act(async () => {});
    document.dispatchEvent(new Event("visibilitychange"));
    await act(async () => {});
    expect(apiMock.listTaskSessions).not.toHaveBeenCalled();

    mockState.taskSessionsByTask.loadingByTaskId[TASK_ID] = false;
    await act(async () => {
      rerender();
    });

    await waitFor(() =>
      expect(apiMock.listTaskSessions).toHaveBeenCalledWith(TASK_ID, {
        cache: "no-store",
      }),
    );
    expect(mockState.setTaskSessionsForTask).toHaveBeenCalledWith(TASK_ID, [
      session("old", "COMPLETED"),
    ]);
  });

  it("refetches a loaded session list on foreground visibility while disconnected", async () => {
    mockState.connection.status = "disconnected";
    mockState.taskSessionsByTask.itemsByTaskId[TASK_ID] = [session("old", "RUNNING")];
    mockState.taskSessionsByTask.loadedByTaskId[TASK_ID] = true;
    apiMock.listTaskSessions.mockResolvedValueOnce({ sessions: [session("old", "COMPLETED")] });

    renderHook(() => useTaskSessions(TASK_ID));
    await act(async () => {});
    document.dispatchEvent(new Event("visibilitychange"));

    await waitFor(() =>
      expect(apiMock.listTaskSessions).toHaveBeenCalledWith(TASK_ID, {
        cache: "no-store",
      }),
    );
    expect(mockState.setTaskSessionsForTask).toHaveBeenCalledWith(TASK_ID, [
      session("old", "COMPLETED"),
    ]);
  });
});
