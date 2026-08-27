import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const lifecycleMocks = vi.hoisted(() => ({
  deleteTask: vi.fn(),
  getWebSocketClient: vi.fn(),
  removeQuickChatSession: vi.fn(),
  request: vi.fn(),
  quickChatSessions: [] as Array<{ sessionId: string; taskId?: string }>,
}));

vi.mock("@/lib/api/domains/kanban-api", () => ({
  deleteTask: lifecycleMocks.deleteTask,
}));
vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: lifecycleMocks.getWebSocketClient,
}));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: Record<string, unknown>) => unknown) =>
    selector({ removeQuickChatSession: lifecycleMocks.removeQuickChatSession }),
  useAppStoreApi: () => ({
    getState: () => ({ quickChat: { sessions: lifecycleMocks.quickChatSessions } }),
  }),
}));

import { sessionStatusTooltip, useSessionLifecycleActions } from "./sessions-dropdown";

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
