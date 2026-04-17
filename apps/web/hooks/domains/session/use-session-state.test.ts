import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook } from "@testing-library/react";
import type { TaskSession } from "@/lib/types/http";

// Mock state
let mockActiveTaskId: string | null = null;
let mockActiveSessionId: string | null = null;
let mockSessionItems: Record<string, TaskSession> = {};
let mockSession: TaskSession | null = null;
let mockTask: { id: string; description: string } | null = null;

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: Record<string, unknown>) => unknown) =>
    selector({
      tasks: {
        activeTaskId: mockActiveTaskId,
        activeSessionId: mockActiveSessionId,
      },
      taskSessions: {
        items: mockSessionItems,
      },
    }),
}));

vi.mock("@/hooks/domains/session/use-session", () => ({
  useSession: (id: string | null) => ({ session: id ? mockSession : null }),
}));

vi.mock("@/hooks/use-task", () => ({
  useTask: (id: string | null) => (id ? mockTask : null),
}));

import { useSessionState } from "./use-session-state";

const createMockSession = (
  id: string,
  taskId: string,
  state: TaskSession["state"] = "IDLE",
): TaskSession =>
  ({
    id,
    task_id: taskId,
    state,
    error_message: "",
  }) as TaskSession;

describe("useSessionState", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockActiveTaskId = null;
    mockActiveSessionId = null;
    mockSessionItems = {};
    mockSession = null;
    mockTask = null;
  });

  describe("resolvedSessionId", () => {
    it("uses sessionId directly when provided", () => {
      mockActiveTaskId = "task-1";
      mockActiveSessionId = "session-old";
      mockSessionItems = { "session-old": createMockSession("session-old", "task-old") };

      const { result } = renderHook(() => useSessionState("session-explicit"));

      expect(result.current.resolvedSessionId).toBe("session-explicit");
    });

    it("uses activeSessionId when sessionId is null and session belongs to active task", () => {
      mockActiveTaskId = "task-1";
      mockActiveSessionId = "session-1";
      mockSessionItems = { "session-1": createMockSession("session-1", "task-1") };

      const { result } = renderHook(() => useSessionState(null));

      expect(result.current.resolvedSessionId).toBe("session-1");
    });

    it("returns null when sessionId is null and activeSessionId belongs to different task", () => {
      mockActiveTaskId = "task-2";
      mockActiveSessionId = "session-1";
      mockSessionItems = { "session-1": createMockSession("session-1", "task-1") };

      const { result } = renderHook(() => useSessionState(null));

      expect(result.current.resolvedSessionId).toBeNull();
    });

    it("returns null when sessionId is null and session not yet in store", () => {
      mockActiveTaskId = "task-1";
      mockActiveSessionId = "session-1";
      mockSessionItems = {}; // Session not loaded yet

      const { result } = renderHook(() => useSessionState(null));

      expect(result.current.resolvedSessionId).toBeNull();
    });

    it("returns null when sessionId is null and activeSessionId is null", () => {
      mockActiveTaskId = "task-1";
      mockActiveSessionId = null;
      mockSessionItems = {};

      const { result } = renderHook(() => useSessionState(null));

      expect(result.current.resolvedSessionId).toBeNull();
    });
  });

  describe("session flags", () => {
    it("sets isStarting when session state is STARTING", () => {
      mockSession = createMockSession("session-1", "task-1", "STARTING");

      const { result } = renderHook(() => useSessionState("session-1"));

      expect(result.current.isStarting).toBe(true);
      expect(result.current.isWorking).toBe(true);
    });

    it("sets isAgentBusy when session state is RUNNING", () => {
      mockSession = createMockSession("session-1", "task-1", "RUNNING");

      const { result } = renderHook(() => useSessionState("session-1"));

      expect(result.current.isAgentBusy).toBe(true);
      expect(result.current.isWorking).toBe(true);
    });

    it("sets isFailed when session state is FAILED", () => {
      mockSession = createMockSession("session-1", "task-1", "FAILED");

      const { result } = renderHook(() => useSessionState("session-1"));

      expect(result.current.isFailed).toBe(true);
    });
  });
});
