import { describe, it, expect, vi, beforeEach } from "vitest";
import { act, renderHook } from "@testing-library/react";

const mockEnsureTaskSession = vi.fn();
const mockLoadSessions = vi.fn().mockResolvedValue(undefined);
let mockSessionsResult: {
  sessions: Array<{ id: string }>;
  isLoaded: boolean;
  loadSessions: (force?: boolean) => Promise<void>;
} = {
  sessions: [],
  isLoaded: true,
  loadSessions: mockLoadSessions,
};

vi.mock("@/lib/services/session-launch-service", () => ({
  ensureTaskSession: (taskId: string) => mockEnsureTaskSession(taskId),
}));

vi.mock("@/hooks/use-task-sessions", () => ({
  useTaskSessions: () => mockSessionsResult,
}));

import { useEnsureTaskSession } from "./use-ensure-task-session";

const TASK = { id: "task-1" };

function flushMicrotasks() {
  return act(() => Promise.resolve());
}

describe("useEnsureTaskSession", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockLoadSessions.mockResolvedValue(undefined);
    mockSessionsResult = { sessions: [], isLoaded: true, loadSessions: mockLoadSessions };
    mockEnsureTaskSession.mockResolvedValue({
      success: true,
      task_id: "task-1",
      session_id: "sess-new",
      state: "CREATED",
      source: "created_prepare",
      newly_created: true,
    });
  });

  it("calls the backend ensure endpoint once when the task has no sessions", async () => {
    const { result } = renderHook(() => useEnsureTaskSession(TASK));

    expect(mockEnsureTaskSession).toHaveBeenCalledTimes(1);
    expect(mockEnsureTaskSession).toHaveBeenCalledWith("task-1");
    expect(result.current.status).toBe("preparing");
    await flushMicrotasks();
    expect(result.current.status).toBe("idle");
  });

  it("force-reloads the session list after a successful ensure", async () => {
    renderHook(() => useEnsureTaskSession(TASK));
    await flushMicrotasks();
    // Two awaits: ensure().then(loadSessions(true)).then(setStatus).
    await flushMicrotasks();
    expect(mockLoadSessions).toHaveBeenCalledWith(true);
  });

  it("no-ops when the task already has a session", () => {
    mockSessionsResult = {
      sessions: [{ id: "sess-1" }],
      isLoaded: true,
      loadSessions: mockLoadSessions,
    };
    renderHook(() => useEnsureTaskSession(TASK));
    expect(mockEnsureTaskSession).not.toHaveBeenCalled();
  });

  it("no-ops while sessions are still loading", () => {
    mockSessionsResult = { sessions: [], isLoaded: false, loadSessions: mockLoadSessions };
    renderHook(() => useEnsureTaskSession(TASK));
    expect(mockEnsureTaskSession).not.toHaveBeenCalled();
  });

  it("no-ops when disabled", () => {
    renderHook(() => useEnsureTaskSession(TASK, { enabled: false }));
    expect(mockEnsureTaskSession).not.toHaveBeenCalled();
  });

  it("no-ops when task id is missing", () => {
    renderHook(() => useEnsureTaskSession(null));
    expect(mockEnsureTaskSession).not.toHaveBeenCalled();
  });

  it("is idempotent across re-renders for the same task", () => {
    const { rerender } = renderHook(() => useEnsureTaskSession(TASK));
    rerender();
    rerender();
    expect(mockEnsureTaskSession).toHaveBeenCalledTimes(1);
  });

  it("reports an error and exposes a working retry()", async () => {
    mockEnsureTaskSession.mockRejectedValueOnce(new Error("boom"));
    const { result } = renderHook(() => useEnsureTaskSession(TASK));

    expect(mockEnsureTaskSession).toHaveBeenCalledTimes(1);
    await flushMicrotasks();
    expect(result.current.status).toBe("error");
    expect(result.current.error?.message).toBe("boom");

    mockEnsureTaskSession.mockResolvedValueOnce({
      success: true,
      task_id: "task-1",
      session_id: "sess-new",
      state: "CREATED",
      source: "created_prepare",
      newly_created: true,
    });
    act(() => result.current.retry());
    expect(mockEnsureTaskSession).toHaveBeenCalledTimes(2);
    await flushMicrotasks();
    expect(result.current.status).toBe("idle");
  });

  it("calls ensure again when the task id changes", () => {
    const { rerender } = renderHook(
      ({ task }: { task: { id: string } }) => useEnsureTaskSession(task),
      { initialProps: { task: TASK } },
    );
    expect(mockEnsureTaskSession).toHaveBeenCalledTimes(1);
    rerender({ task: { id: "task-2" } });
    expect(mockEnsureTaskSession).toHaveBeenCalledTimes(2);
    expect(mockEnsureTaskSession).toHaveBeenLastCalledWith("task-2");
  });
});
