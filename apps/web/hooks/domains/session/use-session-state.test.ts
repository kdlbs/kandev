import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook } from "@testing-library/react";
import type { TaskSession } from "@/lib/types/http";

// Mock state
let mockActiveTaskId: string | null = null;
let mockActiveSessionId: string | null = null;
let mockSessionItems: Record<string, TaskSession> = {};
let mockSession: TaskSession | null = null;
let mockTask: { id: string; description: string } | null = null;
let mockPrepareProgress: Record<string, { status: string }> = {};

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
      prepareProgress: {
        bySessionId: mockPrepareProgress,
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
  state: TaskSession["state"] = "CREATED",
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
    mockPrepareProgress = {};
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

    it("uses taskIdHint while an explicit session row is hydrating", () => {
      mockSession = null;

      const { result } = renderHook(() => useSessionState("session-new", { taskIdHint: "task-1" }));

      expect(result.current.resolvedSessionId).toBe("session-new");
      expect(result.current.taskId).toBe("task-1");
    });

    it("prefers the hydrated session task over taskIdHint", () => {
      mockSession = createMockSession("session-1", "task-2");

      const { result } = renderHook(() => useSessionState("session-1", { taskIdHint: "task-1" }));

      expect(result.current.taskId).toBe("task-2");
    });
  });

  describe("session flags", () => {
    it("sets isStarting when session state is STARTING", () => {
      mockSession = createMockSession("session-1", "task-1", "STARTING");

      const { result } = renderHook(() => useSessionState("session-1"));

      expect(result.current.isStarting).toBe(true);
      expect(result.current.isWorking).toBe(true);
      expect(result.current.isAgentBusy).toBe(true);
    });

    it("does not set isStarting when session state is CREATED", () => {
      mockSession = createMockSession("session-1", "task-1", "CREATED");

      const { result } = renderHook(() => useSessionState("session-1"));

      // CREATED stays directly promptable so its first chat can start the agent.
      expect(result.current.isStarting).toBe(false);
      expect(result.current.isAgentBusy).toBe(false);
      expect(result.current.inputMode).toBe("direct");
    });

    it("does not set isStarting when WAITING_FOR_INPUT", () => {
      mockSession = createMockSession("session-1", "task-1", "WAITING_FOR_INPUT");

      const { result } = renderHook(() => useSessionState("session-1"));

      expect(result.current.isStarting).toBe(false);
      expect(result.current.isWorking).toBe(false);
    });

    it("sets isStarting while environment preparation is still live", () => {
      mockSession = createMockSession("session-1", "task-1", "WAITING_FOR_INPUT");
      mockPrepareProgress = { "session-1": { status: "preparing" } };

      const { result } = renderHook(() => useSessionState("session-1"));

      expect(result.current.isStarting).toBe(true);
      expect(result.current.isWorking).toBe(true);
    });

    it("sets isAgentBusy when session state is RUNNING", () => {
      mockSession = createMockSession("session-1", "task-1", "RUNNING");

      const { result } = renderHook(() => useSessionState("session-1"));

      expect(result.current.isAgentBusy).toBe(true);
      expect(result.current.isWorking).toBe(true);
      expect(result.current.inputMode).toBe("queue");
    });

    it("sets isFailed when session state is FAILED", () => {
      mockSession = createMockSession("session-1", "task-1", "FAILED");

      const { result } = renderHook(() => useSessionState("session-1"));

      expect(result.current.isFailed).toBe(true);
    });
  });
});

describe("useSessionState — fine-grained busy signal (foreground_activity)", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockActiveTaskId = null;
    mockActiveSessionId = null;
    mockSessionItems = {};
    mockSession = null;
    mockTask = null;
    mockPrepareProgress = {};
  });

  // ADR-0038. (a) generating gates the composer;
  // (b) background-idle accepts input but stays working; (c) fully idle.
  it("(a) RUNNING+generating gates the composer", () => {
    mockSession = {
      ...createMockSession("session-1", "task-1", "RUNNING"),
      foreground_activity: "generating",
    };

    const { result } = renderHook(() => useSessionState("session-1"));

    // Gated (busy) AND working.
    expect(result.current.isAgentBusy).toBe(true);
    expect(result.current.isWorking).toBe(true);
  });

  it("(b) RUNNING+background accepts input while the working affordance stays up", () => {
    mockSession = {
      ...createMockSession("session-1", "task-1", "RUNNING"),
      foreground_activity: "background",
    };

    const { result } = renderHook(() => useSessionState("session-1"));

    // Composer enabled (not busy) — the message sends instead of queueing —
    // yet still working, never "done".
    expect(result.current.isAgentBusy).toBe(false);
    expect(result.current.isWorking).toBe(true);
    expect(result.current.inputMode).toBe("direct");
  });

  it("settled+background accepts input while detached work keeps the affordance active", () => {
    mockSession = {
      ...createMockSession("session-1", "task-1", "WAITING_FOR_INPUT"),
      foreground_activity: "background",
    };

    const { result } = renderHook(() => useSessionState("session-1"));

    expect(result.current.isWorking).toBe(true);
    expect(result.current.isAgentBusy).toBe(false);
  });

  it("settled+generating is idle because generating only owns a live foreground turn", () => {
    mockSession = {
      ...createMockSession("session-1", "task-1", "WAITING_FOR_INPUT"),
      foreground_activity: "generating",
    };

    const { result } = renderHook(() => useSessionState("session-1"));

    expect(result.current.isWorking).toBe(false);
    expect(result.current.isAgentBusy).toBe(false);
  });
});
