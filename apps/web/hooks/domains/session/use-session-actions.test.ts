import { describe, it, expect, vi, beforeEach } from "vitest";
import { act, renderHook, waitFor } from "@testing-library/react";
import {
  isSessionStoppable,
  isSessionDeletable,
  isSessionResumable,
  useSessionActions,
} from "./use-session-actions";

const mockToast = vi.fn().mockReturnValue("toast-1");
const mockUpdateToast = vi.fn();
const mockRequest = vi.fn();
const mockDeleteTask = vi.hoisted(() => vi.fn());
const mockRemoveTaskSession = vi.fn();
const mockRemoveQuickChatSession = vi.fn();
const mockSetActiveSession = vi.fn((taskId: string, sessionId: string) => {
  mockState.tasks = {
    ...(mockState.tasks as object),
    activeTaskId: taskId,
    activeSessionId: sessionId,
  };
});
const mockSetActiveSessionAuto = vi.fn((taskId: string, sessionId: string) => {
  mockState.tasks = {
    ...(mockState.tasks as object),
    activeTaskId: taskId,
    activeSessionId: sessionId,
  };
});
const mockClearActiveSession = vi.fn();
const networkErrorMessage = "network down";

let mockConnected = true;
const mockStoreApi = { getState: () => mockState };

let mockState: Record<string, unknown> = {};

/** Resets transport fakes and eligible-session state for each lifecycle scenario. */
function resetSessionActionMocks() {
  vi.clearAllMocks();
  mockConnected = true;
  mockToast.mockReturnValue("toast-1");
  mockRequest.mockResolvedValue(undefined);
  mockDeleteTask.mockResolvedValue(undefined);
  mockState = {
    tasks: { activeTaskId: "t1", activeSessionId: null },
    taskRemoval: { navigationRevision: 0 },
    taskSessionsByTask: { itemsByTaskId: { t1: [{ id: "s1", state: "RUNNING" }] } },
    quickChat: { sessions: [] },
    setActiveSession: mockSetActiveSession,
    setActiveSessionAuto: mockSetActiveSessionAuto,
    clearActiveSession: mockClearActiveSession,
  };
}

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mockToast, updateToast: mockUpdateToast }),
}));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => (mockConnected ? { request: mockRequest } : null),
}));

vi.mock("@/lib/api/domains/kanban-api", () => ({
  deleteTask: mockDeleteTask,
  deleteTaskAfterUserAction: mockDeleteTask,
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: Record<string, unknown>) => unknown) =>
    selector({
      removeTaskSession: mockRemoveTaskSession,
      removeQuickChatSession: mockRemoveQuickChatSession,
    }),
  useAppStoreApi: () => mockStoreApi,
}));

describe("session state predicates", () => {
  it("isSessionStoppable returns true for active states", () => {
    expect(isSessionStoppable("RUNNING")).toBe(true);
    expect(isSessionStoppable("STARTING")).toBe(true);
    expect(isSessionStoppable("WAITING_FOR_INPUT")).toBe(true);
    expect(isSessionStoppable("COMPLETED")).toBe(false);
    expect(isSessionStoppable("FAILED")).toBe(false);
  });

  it("isSessionDeletable returns false for in-flight states", () => {
    expect(isSessionDeletable("RUNNING")).toBe(false);
    expect(isSessionDeletable("STARTING")).toBe(false);
    expect(isSessionDeletable("WAITING_FOR_INPUT")).toBe(true);
    expect(isSessionDeletable("COMPLETED")).toBe(true);
    expect(isSessionDeletable("FAILED")).toBe(true);
  });

  it("isSessionResumable returns true for terminal states", () => {
    expect(isSessionResumable("COMPLETED")).toBe(true);
    expect(isSessionResumable("FAILED")).toBe(true);
    expect(isSessionResumable("CANCELLED")).toBe(true);
    expect(isSessionResumable("RUNNING")).toBe(false);
    expect(isSessionResumable("STARTING")).toBe(false);
  });
});

// eslint-disable-next-line max-lines-per-function -- session action scenarios share one lifecycle fixture.
describe("useSessionActions", () => {
  beforeEach(resetSessionActionMocks);

  it("setPrimary dispatches session.set_primary with session id", async () => {
    const { result } = renderHook(() => useSessionActions({ sessionId: "s1", taskId: "t1" }));
    await result.current.setPrimary();
    expect(mockRequest).toHaveBeenCalledWith("session.set_primary", { session_id: "s1" }, 15000);
  });

  it("stop dispatches session.stop", async () => {
    const { result } = renderHook(() => useSessionActions({ sessionId: "s1", taskId: "t1" }));
    await result.current.stop();
    expect(mockRequest).toHaveBeenCalledWith("session.stop", { session_id: "s1" }, 15000);
  });

  it("resume dispatches session.launch with intent=resume and 30s timeout", async () => {
    const { result } = renderHook(() => useSessionActions({ sessionId: "s1", taskId: "t1" }));
    await result.current.resume();
    expect(mockRequest).toHaveBeenCalledWith(
      "session.launch",
      { task_id: "t1", intent: "resume", session_id: "s1" },
      30000,
    );
  });

  it("remove deletes via WS, removes from store, and runs onDeleted callback", async () => {
    const onDeleted = vi.fn();
    const { result } = renderHook(() =>
      useSessionActions({ sessionId: "s1", taskId: "t1", onDeleted }),
    );
    await result.current.remove();
    await waitFor(() => expect(onDeleted).toHaveBeenCalled());
    expect(mockRequest).toHaveBeenCalledWith("session.delete", { session_id: "s1" }, 15000);
    expect(mockRemoveTaskSession).toHaveBeenCalledWith("t1", "s1");
    expect(mockRemoveQuickChatSession).toHaveBeenCalledWith("s1");
    expect(mockToast).toHaveBeenCalledWith({
      title: "Deleting session...",
      variant: "loading",
    });
    expect(mockUpdateToast).toHaveBeenCalledWith("toast-1", {
      title: "Deleting session successful",
      variant: "success",
    });
  });

  it("remove deletes the backing task for a Quick Chat session", async () => {
    mockState.quickChat = { sessions: [{ sessionId: "s1", taskId: "t1" }] };
    const { result } = renderHook(() => useSessionActions({ sessionId: "s1", taskId: "t1" }));

    const ok = await result.current.remove();

    expect(ok).toBe(true);
    expect(mockDeleteTask).toHaveBeenCalledWith("t1");
    expect(mockRequest).not.toHaveBeenCalled();
    expect(mockRemoveTaskSession).toHaveBeenCalledWith("t1", "s1");
    expect(mockRemoveQuickChatSession).toHaveBeenCalledWith("s1");
  });

  it("remove no-ops when WS request fails (store untouched)", async () => {
    mockRequest.mockRejectedValueOnce(new Error(networkErrorMessage));
    const onDeleted = vi.fn();
    const { result } = renderHook(() =>
      useSessionActions({ sessionId: "s1", taskId: "t1", onDeleted }),
    );
    await result.current.remove();
    expect(mockRemoveTaskSession).not.toHaveBeenCalled();
    expect(onDeleted).not.toHaveBeenCalled();
  });

  it("remove hands off to most-recent remaining session when active was deleted", async () => {
    mockState = {
      tasks: { activeSessionId: "s1" },
      quickChat: { sessions: [] },
      taskSessionsByTask: {
        itemsByTaskId: {
          t1: [
            { id: "s1", started_at: "2025-01-01T00:00:00Z" },
            { id: "s2", started_at: "2025-01-02T00:00:00Z" },
            { id: "s3", started_at: "2025-01-03T00:00:00Z" },
          ],
        },
      },
      setActiveSessionAuto: mockSetActiveSessionAuto,
      clearActiveSession: mockClearActiveSession,
    };
    const { result } = renderHook(() => useSessionActions({ sessionId: "s1", taskId: "t1" }));
    await result.current.remove();
    expect(mockSetActiveSessionAuto).toHaveBeenCalledWith("t1", "s3");
    expect(mockClearActiveSession).not.toHaveBeenCalled();
  });

  it("remove clears active session when no other sessions remain", async () => {
    mockState = {
      tasks: { activeSessionId: "s1" },
      quickChat: { sessions: [] },
      taskSessionsByTask: {
        itemsByTaskId: { t1: [{ id: "s1", started_at: "2025-01-01T00:00:00Z" }] },
      },
      setActiveSessionAuto: mockSetActiveSessionAuto,
      clearActiveSession: mockClearActiveSession,
    };
    const { result } = renderHook(() => useSessionActions({ sessionId: "s1", taskId: "t1" }));
    await result.current.remove();
    expect(mockClearActiveSession).toHaveBeenCalled();
    expect(mockSetActiveSessionAuto).not.toHaveBeenCalled();
  });

  it("actions no-op when sessionId is missing", async () => {
    const { result } = renderHook(() => useSessionActions({ sessionId: null, taskId: "t1" }));
    await result.current.setPrimary();
    await result.current.stop();
    await result.current.remove();
    expect(mockRequest).not.toHaveBeenCalled();
  });
});

describe("useSessionActions primary feedback", () => {
  beforeEach(resetSessionActionMocks);

  it("setPrimary succeeds without progress or success toasts", async () => {
    const { result } = renderHook(() => useSessionActions({ sessionId: "s1", taskId: "t1" }));

    const ok = await result.current.setPrimary();

    expect(ok).toBe(true);
    expect(mockToast).not.toHaveBeenCalled();
    expect(mockUpdateToast).not.toHaveBeenCalled();
  });

  it("setPrimary shows one error toast when the request fails", async () => {
    mockRequest.mockRejectedValueOnce(new Error(networkErrorMessage));
    const { result } = renderHook(() => useSessionActions({ sessionId: "s1", taskId: "t1" }));

    const ok = await result.current.setPrimary();

    expect(ok).toBe(false);
    expect(mockToast).toHaveBeenCalledTimes(1);
    expect(mockToast).toHaveBeenCalledWith({
      title: "Set primary failed",
      description: networkErrorMessage,
      variant: "error",
    });
    expect(mockUpdateToast).not.toHaveBeenCalled();
  });
});

describe("useSessionActions inline feedback", () => {
  beforeEach(resetSessionActionMocks);

  it("removes without progress or success toasts", async () => {
    const { result } = renderHook(() => useSessionActions({ sessionId: "s1", taskId: "t1" }));

    const ok = await result.current.remove({ feedback: "inline" });

    expect(ok).toBe(true);
    expect(mockToast).not.toHaveBeenCalled();
    expect(mockUpdateToast).not.toHaveBeenCalled();
  });

  it("shows one error toast when deletion fails", async () => {
    mockRequest.mockRejectedValueOnce(new Error(networkErrorMessage));
    const { result } = renderHook(() => useSessionActions({ sessionId: "s1", taskId: "t1" }));

    const ok = await result.current.remove({ feedback: "inline" });

    expect(ok).toBe(false);
    expect(mockToast).toHaveBeenCalledTimes(1);
    expect(mockToast).toHaveBeenCalledWith({
      title: "Deleting session failed",
      description: networkErrorMessage,
      variant: "error",
    });
    expect(mockUpdateToast).not.toHaveBeenCalled();
  });
});

// @covers AC-TASKS-SESSION-TAB-RESTART-001.2, AC-TASKS-SESSION-TAB-RESTART-001.3, AC-TASKS-SESSION-TAB-RESTART-001.4
// eslint-disable-next-line max-lines-per-function -- restart scenarios exercise one shared transport fixture.
describe("useSessionActions restart", () => {
  beforeEach(resetSessionActionMocks);

  it("stops before resuming the captured session and keeps local data", async () => {
    let finishStop!: () => void;
    mockRequest.mockReturnValueOnce(
      new Promise<void>((resolve) => {
        finishStop = resolve;
      }),
    );
    const { result } = renderHook(() => useSessionActions({ sessionId: "s1", taskId: "t1" }));
    expect(result.current).toHaveProperty("restart");
    let pending!: Promise<boolean>;
    act(() => {
      pending = result.current.restart();
    });
    expect(mockRequest.mock.calls).toEqual([["session.stop", { session_id: "s1" }, 15000]]);
    expect(result.current.isRestarting).toBe(true);
    await act(async () => {
      finishStop();
      expect(await pending).toBe(true);
    });
    expect(mockRequest.mock.calls[1]).toEqual([
      "session.launch",
      { task_id: "t1", intent: "resume", session_id: "s1" },
      30000,
    ]);
    expect(result.current.isRestarting).toBe(false);
    expect(mockRemoveTaskSession).not.toHaveBeenCalled();
    expect(mockSetActiveSessionAuto).not.toHaveBeenCalled();
    expect(mockToast).toHaveBeenCalledTimes(1);
    expect(mockUpdateToast).toHaveBeenCalledWith("toast-1", {
      title: "Session restart requested",
      variant: "success",
    });
  });

  it.each([false, true])(
    "restores the selected session after sibling adoption (launch fails: %s)",
    async (launchFails) => {
      mockState.tasks = { activeTaskId: "t1", activeSessionId: "s1" };
      let finishStop!: () => void;
      mockRequest.mockReturnValueOnce(
        new Promise<void>((resolve) => {
          finishStop = resolve;
        }),
      );
      if (launchFails) mockRequest.mockRejectedValueOnce(new Error(networkErrorMessage));
      const { result } = renderHook(() => useSessionActions({ sessionId: "s1", taskId: "t1" }));
      let pending!: Promise<boolean>;
      act(() => {
        pending = result.current.restart();
      });
      mockState.tasks = { activeTaskId: "t1", activeSessionId: "s2" };
      await act(async () => {
        finishStop();
        expect(await pending).toBe(!launchFails);
      });
      expect(mockState.tasks).toMatchObject({ activeTaskId: "t1", activeSessionId: "s1" });
      expect(mockRequest.mock.calls.map(([action]) => action)).toEqual([
        "session.stop",
        "session.launch",
      ]);
    },
  );

  it.each(["t1", "t2"])(
    "preserves later user navigation to task %s during restart",
    async (activeTaskId) => {
      mockState.tasks = { activeTaskId: "t1", activeSessionId: "s1" };
      let finishStop!: () => void;
      mockRequest.mockReturnValueOnce(
        new Promise<void>((resolve) => {
          finishStop = resolve;
        }),
      );
      const { result } = renderHook(() => useSessionActions({ sessionId: "s1", taskId: "t1" }));
      let pending!: Promise<boolean>;
      act(() => {
        pending = result.current.restart();
      });
      mockState.tasks = { activeTaskId, activeSessionId: "s2" };
      mockState.taskRemoval = { navigationRevision: 1 };
      await act(async () => {
        finishStop();
        await pending;
      });
      expect(mockState.tasks).toMatchObject({ activeTaskId, activeSessionId: "s2" });
      expect(mockSetActiveSession).not.toHaveBeenCalled();
    },
  );

  it.each([0, 1])(
    "releases pending state when request %s fails without replay",
    async (failedRequest) => {
      if (failedRequest) mockRequest.mockResolvedValueOnce(undefined);
      mockRequest.mockRejectedValueOnce(new Error(networkErrorMessage));
      const { result } = renderHook(() => useSessionActions({ sessionId: "s1", taskId: "t1" }));
      expect(result.current).toHaveProperty("restart");
      await act(async () => {
        expect(await result.current.restart()).toBe(false);
      });
      expect(mockRequest).toHaveBeenCalledTimes(failedRequest + 1);
      expect(result.current.isRestarting).toBe(false);
      expect(mockUpdateToast).toHaveBeenCalledWith("toast-1", {
        title: "Session restart failed",
        description: networkErrorMessage,
        variant: "error",
      });
    },
  );

  it("guards duplicate consumers and conflicting actions until the initiator unmounts and completes", async () => {
    let finishStop!: () => void;
    mockRequest.mockReturnValueOnce(
      new Promise<void>((resolve) => {
        finishStop = resolve;
      }),
    );
    const first = renderHook(() => useSessionActions({ sessionId: "s1", taskId: "t1" }));
    const second = renderHook(() => useSessionActions({ sessionId: "s1", taskId: "t1" }));
    expect(first.result.current).toHaveProperty("restart");
    let pending!: Promise<boolean>;
    act(() => {
      pending = first.result.current.restart();
    });
    expect(second.result.current.isRestarting).toBe(true);
    await act(async () => {
      expect(await second.result.current.restart()).toBe(false);
      await second.result.current.stop();
      await second.result.current.resume();
      await second.result.current.remove();
    });
    expect(mockRequest).toHaveBeenCalledTimes(1);
    first.unmount();
    await act(async () => {
      finishStop();
      await pending;
    });
    expect(second.result.current.isRestarting).toBe(false);
  });

  it("does not retarget an accepted restart after navigation", async () => {
    let finishStop!: () => void;
    mockRequest.mockReturnValueOnce(
      new Promise<void>((resolve) => {
        finishStop = resolve;
      }),
    );
    const { result, rerender } = renderHook(
      ({ sessionId }) => useSessionActions({ sessionId, taskId: "t1" }),
      { initialProps: { sessionId: "s1" } },
    );
    expect(result.current).toHaveProperty("restart");
    let pending!: Promise<boolean>;
    act(() => {
      pending = result.current.restart();
    });
    rerender({ sessionId: "s2" });
    expect(result.current.isRestarting).toBe(false);
    await act(async () => {
      finishStop();
      await pending;
    });
    expect(mockRequest.mock.calls[1][1]).toEqual({
      task_id: "t1",
      intent: "resume",
      session_id: "s1",
    });
  });

  it.each(["COMPLETED", "FAILED", "CANCELLED", null])(
    "rejects an ineligible or missing session: %s",
    async (state) => {
      mockState.taskSessionsByTask = { itemsByTaskId: { t1: state ? [{ id: "s1", state }] : [] } };
      const { result } = renderHook(() => useSessionActions({ sessionId: "s1", taskId: "t1" }));
      expect(result.current).toHaveProperty("restart");
      await act(async () => {
        expect(await result.current.restart()).toBe(false);
      });
      expect(mockRequest).not.toHaveBeenCalled();
      expect(mockToast).toHaveBeenCalledWith(expect.objectContaining({ variant: "error" }));
    },
  );

  it("does not retry after an uncertain Stop timeout", async () => {
    mockRequest.mockRejectedValueOnce(new Error("Request timed out"));
    const { result } = renderHook(() => useSessionActions({ sessionId: "s1", taskId: "t1" }));
    await act(async () => {
      expect(await result.current.restart()).toBe(false);
    });
    expect(mockRequest).toHaveBeenCalledTimes(1);
    expect(result.current.isRestarting).toBe(false);
  });

  it("reports a missing connection", async () => {
    mockConnected = false;
    const { result } = renderHook(() => useSessionActions({ sessionId: "s1", taskId: "t1" }));
    expect(result.current).toHaveProperty("restart");
    await act(async () => {
      expect(await result.current.restart()).toBe(false);
    });
    expect(mockRequest).not.toHaveBeenCalled();
    expect(mockUpdateToast).toHaveBeenCalledWith(
      "toast-1",
      expect.objectContaining({
        description: "The connection is unavailable. Reconnect, then try again.",
        variant: "error",
      }),
    );
  });
});
