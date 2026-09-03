import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const lifecycleMocks = vi.hoisted(() => ({
  deleteTask: vi.fn(),
  getWebSocketClient: vi.fn(),
  removeQuickChatSession: vi.fn(),
  request: vi.fn(),
  quickChatSessions: [] as Array<{ sessionId: string; taskId?: string }>,
  setActiveSession: vi.fn(),
  performLayoutSwitch: vi.fn(),
  activeSessionId: null as string | null,
  environmentIdBySessionId: {} as Record<string, string>,
  taskSessionIds: [] as string[],
}));

vi.mock("@/lib/api/domains/kanban-api", () => ({
  deleteTask: lifecycleMocks.deleteTask,
}));
vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: lifecycleMocks.getWebSocketClient,
}));
vi.mock("@/lib/state/dockview-store", () => ({
  performLayoutSwitch: lifecycleMocks.performLayoutSwitch,
}));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: Record<string, unknown>) => unknown) =>
    selector({
      removeQuickChatSession: lifecycleMocks.removeQuickChatSession,
      setActiveSession: lifecycleMocks.setActiveSession,
    }),
  useAppStoreApi: () => ({
    getState: () => ({
      quickChat: { sessions: lifecycleMocks.quickChatSessions },
      tasks: { activeSessionId: lifecycleMocks.activeSessionId },
      environmentIdBySessionId: lifecycleMocks.environmentIdBySessionId,
      taskSessionsByTask: {
        itemsByTaskId: { "task-1": lifecycleMocks.taskSessionIds.map((id) => ({ id })) },
      },
    }),
  }),
}));

import {
  sessionStatusTooltip,
  useSessionLifecycleActions,
  useSessionSelectionHandlers,
} from "./sessions-dropdown";

describe("sessionStatusTooltip", () => {
  it("prioritizes permission over clarification for input-capable sessions", () => {
    expect(sessionStatusTooltip("RUNNING", { permission: true, clarification: true })).toBe(
      "Permission requested",
    );
  });

  it("surfaces clarification over activity for input-capable sessions", () => {
    expect(
      sessionStatusTooltip("WAITING_FOR_INPUT", { permission: false, clarification: true }),
    ).toBe("Waiting for input");
  });

  it("labels background-idle sessions as running when no input is pending", () => {
    expect(
      sessionStatusTooltip(
        "WAITING_FOR_INPUT",
        { permission: false, clarification: false },
        "background",
      ),
    ).toBe("Background running");
  });

  it.each([
    ["STARTING", "Running"],
    ["COMPLETED", "Complete"],
    ["FAILED", "Failed"],
    ["CANCELLED", "Cancelled"],
  ] as const)("ignores stale pending input for %s sessions", (state, expected) => {
    expect(sessionStatusTooltip(state, { permission: true, clarification: true })).toBe(expected);
  });

  it("does not show background-running for a terminal session with a stale background substate", () => {
    expect(
      sessionStatusTooltip("COMPLETED", { permission: false, clarification: false }, "background"),
    ).toBe("Complete");
  });
});

describe("useSessionSelectionHandlers", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    lifecycleMocks.activeSessionId = "session-a";
    lifecycleMocks.environmentIdBySessionId = { "session-a": "env-a", "session-b": "env-b" };
    lifecycleMocks.taskSessionIds = ["session-a", "session-b"];
  });

  it("routes selection through the override and skips the global active session and dockview layout switch", () => {
    const onSelectSession = vi.fn();
    const close = vi.fn();
    const { result } = renderHook(() => useSessionSelectionHandlers("task-1", onSelectSession));

    result.current.handleSelectSession("session-b", close);

    expect(onSelectSession).toHaveBeenCalledWith("session-b");
    expect(close).toHaveBeenCalledTimes(1);
    expect(lifecycleMocks.setActiveSession).not.toHaveBeenCalled();
    expect(lifecycleMocks.performLayoutSwitch).not.toHaveBeenCalled();
  });

  it("falls back to the global active session and dockview layout switch without an override", () => {
    const close = vi.fn();
    const { result } = renderHook(() => useSessionSelectionHandlers("task-1", undefined));

    result.current.handleSelectSession("session-b", close);

    expect(lifecycleMocks.setActiveSession).toHaveBeenCalledWith("task-1", "session-b");
    expect(lifecycleMocks.performLayoutSwitch).toHaveBeenCalledWith("env-a", "env-b", "session-b", [
      "session-a",
      "session-b",
    ]);
    expect(close).toHaveBeenCalledTimes(1);
  });
});

describe("useSessionLifecycleActions", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    lifecycleMocks.deleteTask.mockResolvedValue(undefined);
    lifecycleMocks.request.mockResolvedValue(undefined);
    lifecycleMocks.getWebSocketClient.mockReturnValue({ request: lifecycleMocks.request });
    lifecycleMocks.quickChatSessions.length = 0;
  });

  it("deletes the backing task for a Quick Chat session", async () => {
    lifecycleMocks.quickChatSessions.push({ sessionId: "session-1", taskId: "task-1" });
    const loadSessions = vi.fn();
    const { result } = renderHook(() => useSessionLifecycleActions("task-1", loadSessions));

    await act(async () => {
      await result.current.handleDeleteSession("session-1");
    });

    expect(lifecycleMocks.deleteTask).toHaveBeenCalledWith("task-1");
    expect(lifecycleMocks.request).not.toHaveBeenCalled();
    expect(lifecycleMocks.removeQuickChatSession).toHaveBeenCalledWith("session-1");
    expect(loadSessions).toHaveBeenCalledWith(true);
  });

  it("keeps ordinary session deletion on the session endpoint", async () => {
    const loadSessions = vi.fn();
    const { result } = renderHook(() => useSessionLifecycleActions("task-1", loadSessions));

    await act(async () => {
      await result.current.handleDeleteSession("session-1");
    });

    expect(lifecycleMocks.request).toHaveBeenCalledWith(
      "session.delete",
      { session_id: "session-1" },
      15000,
    );
    expect(lifecycleMocks.deleteTask).not.toHaveBeenCalled();
    expect(loadSessions).toHaveBeenCalledWith(true);
  });
});
