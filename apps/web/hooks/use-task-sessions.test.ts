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
  };
  taskSessionsByTask: {
    itemsByTaskId: Record<string, TaskSession[]>;
    loadingByTaskId: Record<string, boolean>;
    loadedByTaskId: Record<string, boolean>;
    errorByTaskId: Record<string, string | null>;
  };
  connection: { status: string };
  setTaskSessionsForTask: ReturnType<typeof vi.fn>;
  setTaskSessionsLoading: ReturnType<typeof vi.fn>;
  setTaskSessionsError: ReturnType<typeof vi.fn>;
};

let mockState: MockTaskSessionsState;

vi.mock("@/components/state-provider", () => {
  const getState = () => mockState;
  return {
    useAppStore: (selector: (state: MockTaskSessionsState) => unknown) => selector(mockState),
    useAppStoreApi: () => ({ getState }),
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
    taskSessions: { activityEpochBySession: {} },
    taskSessionsByTask: {
      itemsByTaskId: {},
      loadingByTaskId: {},
      loadedByTaskId: {},
      errorByTaskId: {},
    },
    connection: { status: "connected" },
    setTaskSessionsForTask: vi.fn(),
    setTaskSessionsLoading: vi.fn(),
    setTaskSessionsError: vi.fn(),
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((innerResolve) => {
    resolve = innerResolve;
  });
  return { promise, resolve };
}

beforeEach(() => {
  resetMockState();
  setDocumentVisibility("visible");
  apiMock.listTaskSessions.mockResolvedValue({ sessions: [session("sess-1")] });
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.clearAllMocks();
});

describe("useTaskSessions initial load", () => {
  it("loads sessions on mount", async () => {
    renderHook(() => useTaskSessions(TASK_ID));

    await waitFor(() =>
      expect(apiMock.listTaskSessions).toHaveBeenCalledWith(TASK_ID, {
        cache: "no-store",
      }),
    );
    expect(mockState.setTaskSessionsForTask).toHaveBeenCalledWith(TASK_ID, [session("sess-1")], {});
  });

  it("loads sessions on mount when the WebSocket is disconnected", async () => {
    mockState.connection.status = "disconnected";

    renderHook(() => useTaskSessions(TASK_ID));

    await waitFor(() =>
      expect(apiMock.listTaskSessions).toHaveBeenCalledWith(TASK_ID, {
        cache: "no-store",
      }),
    );
    expect(mockState.setTaskSessionsForTask).toHaveBeenCalledWith(TASK_ID, [session("sess-1")], {});
  });

  it("reconciles existing live rows while exposing an initial load error", async () => {
    const error = new Error(SERVICE_UNAVAILABLE);
    const consoleError = vi.spyOn(console, "error").mockImplementation(() => undefined);
    const liveSessions = [session("existing"), session("live-upsert")];
    mockState.taskSessionsByTask.itemsByTaskId[TASK_ID] = liveSessions;
    mockState.taskSessionsByTask.loadedByTaskId[TASK_ID] = false;
    apiMock.listTaskSessions.mockRejectedValueOnce(error);

    renderHook(() => useTaskSessions(TASK_ID));

    await waitFor(() =>
      expect(apiMock.listTaskSessions).toHaveBeenCalledWith(TASK_ID, {
        cache: "no-store",
      }),
    );
    expect(mockState.setTaskSessionsForTask).toHaveBeenCalledWith(TASK_ID, liveSessions, {
      existing: 0,
      "live-upsert": 0,
    });
    expect(mockState.setTaskSessionsError).toHaveBeenCalledWith(TASK_ID, SERVICE_UNAVAILABLE);
    expect(consoleError).toHaveBeenCalledWith("Failed to load task sessions:", error);
  });

  it("exposes an initial load error and clears it after retry success", async () => {
    const error = new Error(SERVICE_UNAVAILABLE);
    apiMock.listTaskSessions.mockRejectedValueOnce(error).mockResolvedValueOnce({
      sessions: [session("recovered")],
    });
    mockState.setTaskSessionsError.mockImplementation((id: string, message: string | null) => {
      mockState.taskSessionsByTask.errorByTaskId[id] = message;
    });
    mockState.setTaskSessionsForTask.mockImplementation((id: string, sessions: TaskSession[]) => {
      mockState.taskSessionsByTask.itemsByTaskId[id] = sessions;
      mockState.taskSessionsByTask.loadedByTaskId[id] = true;
      mockState.taskSessionsByTask.errorByTaskId[id] = null;
    });
    mockState.setTaskSessionsLoading.mockImplementation((id: string, loading: boolean) => {
      mockState.taskSessionsByTask.loadingByTaskId[id] = loading;
    });
    const consoleError = vi.spyOn(console, "error").mockImplementation(() => undefined);

    const { result, rerender } = renderHook(() => useTaskSessions(TASK_ID));
    await waitFor(() => expect(mockState.setTaskSessionsError).toHaveBeenCalled());
    rerender();

    expect(result.current.isLoaded).toBe(false);
    expect(result.current.error).toBe(SERVICE_UNAVAILABLE);

    await act(async () => {
      await result.current.loadSessions(true);
    });
    rerender();

    expect(result.current.isLoaded).toBe(true);
    expect(result.current.error).toBeNull();
    expect(result.current.sessions).toEqual([session("recovered")]);
    expect(consoleError).toHaveBeenCalledWith("Failed to load task sessions:", error);
  });
});

describe("useTaskSessions persistent failures", () => {
  it("does not retry automatically after an initial failure", async () => {
    const error = new Error(SERVICE_UNAVAILABLE);
    vi.spyOn(console, "error").mockImplementation(() => undefined);
    apiMock.listTaskSessions.mockRejectedValue(error);
    mockState.setTaskSessionsLoading.mockImplementation((id: string, loading: boolean) => {
      mockState.taskSessionsByTask.loadingByTaskId[id] = loading;
    });
    mockState.setTaskSessionsError.mockImplementation((id: string, message: string | null) => {
      mockState.taskSessionsByTask.errorByTaskId[id] = message;
    });

    const { result, rerender } = renderHook(() => useTaskSessions(TASK_ID));
    rerender();
    await waitFor(() => expect(mockState.setTaskSessionsError).toHaveBeenCalledTimes(1));
    rerender();

    expect(result.current.error).toBe(SERVICE_UNAVAILABLE);
    expect(apiMock.listTaskSessions).toHaveBeenCalledTimes(1);

    await act(async () => {
      await result.current.loadSessions(true);
    });
    expect(apiMock.listTaskSessions).toHaveBeenCalledTimes(2);
    expect(result.current.error).toBe(SERVICE_UNAVAILABLE);
  });
});

describe("useTaskSessions live reconciliation", () => {
  it("preserves and rehydrates a live session added during an older request", async () => {
    const existing = session("existing");
    const livePartial = session("live-upsert");
    const liveHydrated = { ...livePartial, name: "Hydrated live session" };
    const firstResponse = deferred<{ sessions: TaskSession[] }>();
    mockState.taskSessionsByTask.itemsByTaskId[TASK_ID] = [existing];
    mockState.setTaskSessionsLoading.mockImplementation((id: string, loading: boolean) => {
      mockState.taskSessionsByTask.loadingByTaskId[id] = loading;
    });
    mockState.setTaskSessionsForTask.mockImplementation((id: string, sessions: TaskSession[]) => {
      mockState.taskSessionsByTask.itemsByTaskId[id] = sessions;
      mockState.taskSessionsByTask.loadedByTaskId[id] = true;
    });
    apiMock.listTaskSessions
      .mockReturnValueOnce(firstResponse.promise)
      .mockResolvedValueOnce({ sessions: [existing, liveHydrated] });

    const { rerender } = renderHook(() => useTaskSessions(TASK_ID));
    await waitFor(() => expect(apiMock.listTaskSessions).toHaveBeenCalledTimes(1));

    mockState.taskSessionsByTask.itemsByTaskId[TASK_ID] = [existing, livePartial];
    firstResponse.resolve({ sessions: [existing] });

    await waitFor(() =>
      expect(mockState.setTaskSessionsForTask).toHaveBeenCalledWith(
        TASK_ID,
        [existing, livePartial],
        {
          [existing.id]: 0,
        },
      ),
    );
    rerender();

    await waitFor(() => expect(apiMock.listTaskSessions).toHaveBeenCalledTimes(2));
    expect(mockState.setTaskSessionsForTask).toHaveBeenLastCalledWith(
      TASK_ID,
      [existing, liveHydrated],
      {
        [existing.id]: 0,
        [liveHydrated.id]: 0,
      },
    );
  });
});

describe("useTaskSessions refreshes", () => {
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
    expect(mockState.setTaskSessionsForTask).toHaveBeenCalledWith(
      TASK_ID,
      [session("old", "COMPLETED")],
      { old: 0 },
    );
  });

  // @covers AC-PLATFORM-BACKGROUND-WORK-LIVENESS-001.9
  it("captures activity epochs before a reconnect refresh", async () => {
    const existing = session("old", "WAITING_FOR_INPUT");
    mockState.connection.status = "disconnected";
    mockState.taskSessions.activityEpochBySession[existing.id] = 4;
    mockState.taskSessionsByTask.itemsByTaskId[TASK_ID] = [existing];
    mockState.taskSessionsByTask.loadedByTaskId[TASK_ID] = true;
    apiMock.listTaskSessions.mockResolvedValueOnce({ sessions: [existing] });

    const { rerender } = renderHook(() => useTaskSessions(TASK_ID));
    await act(async () => {});
    mockState.connection.status = "connected";
    rerender();

    await waitFor(() =>
      expect(mockState.setTaskSessionsForTask).toHaveBeenCalledWith(TASK_ID, [existing], {
        [existing.id]: 4,
      }),
    );
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
    expect(mockState.setTaskSessionsError).toHaveBeenCalledWith(TASK_ID, "network down");
    expect(consoleError).toHaveBeenCalledWith("Failed to load task sessions:", error);
  });
});

describe("useTaskSessions queued reconnect refreshes", () => {
  beforeEach(() => {
    resetMockState();
    setDocumentVisibility("visible");
    apiMock.listTaskSessions.mockResolvedValue({ sessions: [session("sess-1")] });
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
    vi.clearAllMocks();
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
    expect(mockState.setTaskSessionsForTask).toHaveBeenCalledWith(
      TASK_ID,
      [session("old", "COMPLETED")],
      { old: 0 },
    );
  });

  it("queues a reconnect resync while the initial load is still running", async () => {
    mockState.connection.status = "disconnected";
    mockState.taskSessionsByTask.loadingByTaskId[TASK_ID] = true;
    apiMock.listTaskSessions.mockResolvedValueOnce({ sessions: [session("old", "COMPLETED")] });

    const { rerender } = renderHook(() => useTaskSessions(TASK_ID));
    await act(async () => {});
    mockState.connection.status = "connected";
    rerender();

    expect(apiMock.listTaskSessions).not.toHaveBeenCalled();
    mockState.taskSessionsByTask.loadingByTaskId[TASK_ID] = false;
    mockState.taskSessionsByTask.loadedByTaskId[TASK_ID] = true;
    await act(async () => {
      rerender();
    });

    await waitFor(() =>
      expect(apiMock.listTaskSessions).toHaveBeenCalledWith(TASK_ID, {
        cache: "no-store",
      }),
    );
    expect(mockState.setTaskSessionsForTask).toHaveBeenCalledWith(
      TASK_ID,
      [session("old", "COMPLETED")],
      {},
    );
  });
});

describe("useTaskSessions foreground refresh", () => {
  beforeEach(() => {
    resetMockState();
    setDocumentVisibility("visible");
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
    vi.clearAllMocks();
  });

  it("refetches a loaded session list when the Kandev window regains focus", async () => {
    mockState.taskSessionsByTask.itemsByTaskId[TASK_ID] = [session("old", "RUNNING")];
    mockState.taskSessionsByTask.loadedByTaskId[TASK_ID] = true;
    apiMock.listTaskSessions.mockResolvedValueOnce({ sessions: [session("old", "COMPLETED")] });

    renderHook(() => useTaskSessions(TASK_ID));
    await act(async () => {});
    apiMock.listTaskSessions.mockClear();

    window.dispatchEvent(new Event("focus"));

    await waitFor(() =>
      expect(apiMock.listTaskSessions).toHaveBeenCalledWith(TASK_ID, {
        cache: "no-store",
      }),
    );
    expect(mockState.setTaskSessionsForTask).toHaveBeenCalledWith(
      TASK_ID,
      [session("old", "COMPLETED")],
      { old: 0 },
    );
  });
});

describe("useTaskSessions queued refreshes", () => {
  beforeEach(() => {
    resetMockState();
    setDocumentVisibility("visible");
    apiMock.listTaskSessions.mockResolvedValue({ sessions: [session("sess-1")] });
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
    vi.clearAllMocks();
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
    expect(mockState.setTaskSessionsForTask).toHaveBeenLastCalledWith(
      TASK_ID,
      [session("old", "COMPLETED")],
      { old: 0 },
    );
  });

  it("serializes forced reloads started before loading state rerenders", async () => {
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
    const secondReload = result.current.loadSessions(true);
    expect(apiMock.listTaskSessions).toHaveBeenCalledTimes(1);
    rerender();

    firstResponse.resolve({ sessions: [session("old", "RUNNING")] });
    await firstReload;
    rerender();

    await secondReload;
    expect(apiMock.listTaskSessions).toHaveBeenCalledTimes(2);
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
    vi.restoreAllMocks();
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
    expect(mockState.setTaskSessionsForTask).toHaveBeenCalledWith(
      TASK_ID,
      [session("old", "COMPLETED")],
      { old: 0 },
    );
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
    expect(mockState.setTaskSessionsForTask).toHaveBeenCalledWith(
      TASK_ID,
      [session("old", "COMPLETED")],
      { old: 0 },
    );
  });

  it("queues a foreground refetch while the initial load is running", async () => {
    mockState.taskSessionsByTask.loadingByTaskId[TASK_ID] = true;
    apiMock.listTaskSessions.mockResolvedValueOnce({ sessions: [session("old", "COMPLETED")] });

    const { rerender } = renderHook(() => useTaskSessions(TASK_ID));
    await act(async () => {});
    document.dispatchEvent(new Event("visibilitychange"));
    await act(async () => {});
    expect(apiMock.listTaskSessions).not.toHaveBeenCalled();

    mockState.taskSessionsByTask.loadingByTaskId[TASK_ID] = false;
    mockState.taskSessionsByTask.loadedByTaskId[TASK_ID] = true;
    await act(async () => {
      rerender();
    });

    await waitFor(() =>
      expect(apiMock.listTaskSessions).toHaveBeenCalledWith(TASK_ID, {
        cache: "no-store",
      }),
    );
    expect(mockState.setTaskSessionsForTask).toHaveBeenCalledWith(
      TASK_ID,
      [session("old", "COMPLETED")],
      {},
    );
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
    expect(mockState.setTaskSessionsForTask).toHaveBeenCalledWith(
      TASK_ID,
      [session("old", "COMPLETED")],
      { old: 0 },
    );
  });
});
