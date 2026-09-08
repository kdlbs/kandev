import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const mockRequest = vi.fn();
const mockSetTaskSession = vi.fn();
const mockSetSessionAgentctlStatus = vi.fn();
const mockSetResumeSkipped = vi.fn();
const sessionItems = {
  s1: {
    started_at: "2026-01-01T00:00:00.000Z",
    updated_at: "2026-01-01T00:00:00.000Z",
    state: "WAITING_FOR_INPUT",
  },
};

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ request: mockRequest }),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: Record<string, unknown>) => unknown) =>
    selector({
      connection: { status: "connected" },
      taskSessions: { items: sessionItems },
      setTaskSession: mockSetTaskSession,
      setSessionAgentctlStatus: mockSetSessionAgentctlStatus,
      setResumeSkipped: mockSetResumeSkipped,
      userSettings: { preventAutoStartAgentOnOpen: true },
      tasks: { resumeSkippedSessionIds: {} },
    }),
  useAppStoreApi: () => ({
    getState: () => ({ taskSessions: { items: sessionItems } }),
  }),
}));

const SESSION_ID = "s1";
const TASK_ID = "t1";
const PREVIOUS_TASK_ID = "previous-task";

import { useSessionResumption } from "./use-session-resumption";

describe("useSessionResumption task navigation", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  // @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-002.7
  it("ignores the previous task response when navigation keeps the same session id", async () => {
    const oldRequest = Promise.withResolvers<unknown>();
    const currentRequest = Promise.withResolvers<unknown>();
    mockRequest.mockReturnValueOnce(oldRequest.promise).mockReturnValueOnce(currentRequest.promise);

    const { result, rerender } = renderHook(
      ({ taskId }: { taskId: string }) => useSessionResumption(taskId, SESSION_ID),
      { initialProps: { taskId: PREVIOUS_TASK_ID } },
    );

    await waitFor(() => expect(mockRequest).toHaveBeenCalledTimes(1));
    rerender({ taskId: TASK_ID });
    await waitFor(() => expect(mockRequest).toHaveBeenCalledTimes(2));

    await act(async () => {
      oldRequest.resolve({
        session_id: SESSION_ID,
        task_id: PREVIOUS_TASK_ID,
        state: "FAILED",
        is_agent_running: false,
        is_resumable: false,
        needs_resume: false,
        error: "session does not belong to task",
        updated_at: "2026-01-03T00:00:00.000Z",
      });
    });
    await act(async () => {
      currentRequest.resolve({
        session_id: SESSION_ID,
        task_id: TASK_ID,
        state: "WAITING_FOR_INPUT",
        is_agent_running: false,
        is_resumable: false,
        needs_resume: false,
        updated_at: "2026-01-02T00:00:00.000Z",
      });
    });

    expect(result.current.error).toBeNull();
    expect(result.current.sessionStatus?.task_id).toBe(TASK_ID);
    expect(mockSetTaskSession).toHaveBeenCalledTimes(1);
    expect(mockSetTaskSession).toHaveBeenCalledWith(
      expect.objectContaining({ task_id: TASK_ID, state: "WAITING_FOR_INPUT" }),
    );
  });

  // @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-002.7
  it("ignores an obsolete response when navigation returns to the same task and session", async () => {
    const firstRequest = Promise.withResolvers<unknown>();
    const middleRequest = Promise.withResolvers<unknown>();
    const currentRequest = Promise.withResolvers<unknown>();
    mockRequest
      .mockReturnValueOnce(firstRequest.promise)
      .mockReturnValueOnce(middleRequest.promise)
      .mockReturnValueOnce(currentRequest.promise);

    const { result, rerender } = renderHook(
      ({ taskId }: { taskId: string }) => useSessionResumption(taskId, SESSION_ID),
      { initialProps: { taskId: PREVIOUS_TASK_ID } },
    );

    await waitFor(() => expect(mockRequest).toHaveBeenCalledTimes(1));
    rerender({ taskId: TASK_ID });
    await waitFor(() => expect(mockRequest).toHaveBeenCalledTimes(2));
    rerender({ taskId: PREVIOUS_TASK_ID });
    await waitFor(() => expect(mockRequest).toHaveBeenCalledTimes(3));

    await act(async () => {
      currentRequest.resolve({
        session_id: SESSION_ID,
        task_id: PREVIOUS_TASK_ID,
        state: "WAITING_FOR_INPUT",
        is_agent_running: false,
        is_resumable: false,
        needs_resume: false,
        updated_at: "2026-01-02T00:00:00.000Z",
      });
      firstRequest.resolve({
        session_id: SESSION_ID,
        task_id: PREVIOUS_TASK_ID,
        state: "FAILED",
        is_agent_running: false,
        is_resumable: false,
        needs_resume: false,
        updated_at: "2026-01-03T00:00:00.000Z",
      });
    });

    expect(result.current.error).toBeNull();
    expect(result.current.sessionStatus?.task_id).toBe(PREVIOUS_TASK_ID);
    expect(mockSetTaskSession).toHaveBeenCalledTimes(1);
    expect(mockSetTaskSession).toHaveBeenCalledWith(
      expect.objectContaining({ task_id: PREVIOUS_TASK_ID, state: "WAITING_FOR_INPUT" }),
    );
  });
});
