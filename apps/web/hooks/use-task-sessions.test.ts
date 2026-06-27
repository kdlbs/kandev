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

  it("loads sessions on mount once connected", async () => {
    renderHook(() => useTaskSessions(TASK_ID));

    await waitFor(() =>
      expect(apiMock.listTaskSessions).toHaveBeenCalledWith(TASK_ID, {
        cache: "no-store",
      }),
    );
    expect(mockState.setTaskSessionsForTask).toHaveBeenCalledWith(TASK_ID, [session("sess-1")]);
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

  it("does not refetch a loaded session list on foreground visibility while disconnected", async () => {
    mockState.connection.status = "disconnected";
    mockState.taskSessionsByTask.itemsByTaskId[TASK_ID] = [session("old", "RUNNING")];
    mockState.taskSessionsByTask.loadedByTaskId[TASK_ID] = true;

    renderHook(() => useTaskSessions(TASK_ID));
    await act(async () => {});
    document.dispatchEvent(new Event("visibilitychange"));

    expect(apiMock.listTaskSessions).not.toHaveBeenCalled();
  });
});
