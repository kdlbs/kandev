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

describe("useTaskSessions foreground refreshes", () => {
  it("refetches a loaded session list when a suspended tab becomes visible again", async () => {
    mockState.taskSessionsByTask.itemsByTaskId[TASK_ID] = [session("old", "RUNNING")];
    mockState.taskSessionsByTask.loadedByTaskId[TASK_ID] = true;
    apiMock.listTaskSessions.mockResolvedValueOnce({ sessions: [session("old", "COMPLETED")] });

    renderHook(() => useTaskSessions(TASK_ID));
    await act(async () => {});
    expect(apiMock.listTaskSessions).not.toHaveBeenCalled();

    document.dispatchEvent(new Event("visibilitychange"));

    await waitFor(() =>
      expect(apiMock.listTaskSessions).toHaveBeenCalledWith(
        TASK_ID,
        expect.objectContaining({
          cache: "no-store",
        }),
      ),
    );
    expect(mockState.setTaskSessionsForTask).toHaveBeenCalledWith(
      TASK_ID,
      [session("old", "COMPLETED")],
      { old: { activity: 0, readCursor: 0, workspaceRecovery: 0 } },
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
      expect(apiMock.listTaskSessions).toHaveBeenCalledWith(
        TASK_ID,
        expect.objectContaining({
          cache: "no-store",
        }),
      ),
    );
    expect(mockState.setTaskSessionsForTask).toHaveBeenCalledWith(
      TASK_ID,
      [session("old", "COMPLETED")],
      { old: { activity: 0, readCursor: 0, workspaceRecovery: 0 } },
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
      expect(apiMock.listTaskSessions).toHaveBeenCalledWith(
        TASK_ID,
        expect.objectContaining({
          cache: "no-store",
        }),
      ),
    );
    expect(mockState.setTaskSessionsForTask).toHaveBeenCalledWith(
      TASK_ID,
      [session("old", "COMPLETED")],
      {},
    );
  });
});

describe("useTaskSessions disconnected foreground refresh", () => {
  it("refetches a loaded session list on foreground visibility while disconnected", async () => {
    mockState.connection.status = "disconnected";
    mockState.taskSessionsByTask.itemsByTaskId[TASK_ID] = [session("old", "RUNNING")];
    mockState.taskSessionsByTask.loadedByTaskId[TASK_ID] = true;
    apiMock.listTaskSessions.mockResolvedValueOnce({ sessions: [session("old", "COMPLETED")] });

    renderHook(() => useTaskSessions(TASK_ID));
    await act(async () => {});
    document.dispatchEvent(new Event("visibilitychange"));

    await waitFor(() =>
      expect(apiMock.listTaskSessions).toHaveBeenCalledWith(
        TASK_ID,
        expect.objectContaining({
          cache: "no-store",
        }),
      ),
    );
    expect(mockState.setTaskSessionsForTask).toHaveBeenCalledWith(
      TASK_ID,
      [session("old", "COMPLETED")],
      { old: { activity: 0, readCursor: 0, workspaceRecovery: 0 } },
    );
  });
});
