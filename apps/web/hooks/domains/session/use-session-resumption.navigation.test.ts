/* eslint-disable max-lines-per-function -- navigation cases share one lifecycle harness. */

import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const mockRequest = vi.fn();
const mockSetTaskSession = vi.fn();
const mockSetSessionAgentctlStatus = vi.fn();
const mockSetResumeSkipped = vi.fn();
let sessionItems: Record<
  string,
  {
    started_at: string;
    updated_at: string;
    state: string;
    queue_incarnation_id: string;
    resume_projection_id?: string;
  }
>;

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
const STARTED_AT = "2026-01-01T00:00:00.000Z";

import { useSessionResumption } from "./use-session-resumption";

describe("useSessionResumption task navigation", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    sessionItems = {
      s1: {
        started_at: STARTED_AT,
        updated_at: STARTED_AT,
        state: "IDLE",
        queue_incarnation_id: "inc-1",
      },
      s2: {
        started_at: STARTED_AT,
        updated_at: STARTED_AT,
        state: "IDLE",
        queue_incarnation_id: "inc-2",
      },
    };
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

  it("cleans the owned projection after navigation invalidates a failed resume", async () => {
    const launchRequest = Promise.withResolvers<unknown>();
    mockRequest
      .mockResolvedValueOnce({
        session_id: "s1",
        task_id: TASK_ID,
        state: "IDLE",
        is_agent_running: false,
        is_resumable: false,
        needs_resume: false,
        updated_at: STARTED_AT,
      })
      .mockReturnValueOnce(launchRequest.promise)
      .mockResolvedValueOnce({
        session_id: "s2",
        task_id: TASK_ID,
        state: "IDLE",
        is_agent_running: false,
        is_resumable: false,
        needs_resume: false,
        updated_at: STARTED_AT,
      })
      .mockResolvedValueOnce({
        session_id: "s1",
        task_id: TASK_ID,
        state: "IDLE",
        is_agent_running: false,
        is_resumable: false,
        needs_resume: false,
        updated_at: STARTED_AT,
      });
    mockSetTaskSession.mockImplementation((next) => {
      const current = sessionItems[next.id as string];
      const merged = { ...current, ...next };
      if (!Object.prototype.hasOwnProperty.call(next, "resume_projection_id")) {
        delete merged.resume_projection_id;
      }
      sessionItems[next.id as string] = merged;
    });

    const { result, rerender } = renderHook(
      ({ sessionId }: { sessionId: string }) => useSessionResumption(TASK_ID, sessionId),
      { initialProps: { sessionId: "s1" } },
    );

    await waitFor(() => expect(mockRequest).toHaveBeenCalledTimes(1));
    let resumePromise!: Promise<boolean>;
    await act(async () => {
      resumePromise = result.current.resumeSession();
    });
    await waitFor(() => expect(sessionItems.s1.state).toBe("STARTING"));

    rerender({ sessionId: "s2" });
    await waitFor(() => expect(mockRequest).toHaveBeenCalledTimes(3));

    await act(async () => {
      launchRequest.reject(new Error("resume disconnected"));
      await resumePromise;
    });

    expect(sessionItems.s1.state).toBe("IDLE");
    expect(sessionItems.s1.resume_projection_id).toBeUndefined();
    expect(sessionItems.s2.state).toBe("IDLE");

    rerender({ sessionId: "s1" });
    await waitFor(() => expect(mockRequest).toHaveBeenCalledTimes(4));
    expect(sessionItems.s1.state).toBe("IDLE");
    expect(sessionItems.s2.state).toBe("IDLE");
  });
});
