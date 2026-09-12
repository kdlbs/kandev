import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { WebSocketRequestError } from "@/lib/ws/client";

const mockRequest = vi.fn();
const mockSetTaskSession = vi.fn();
const mockSetSessionAgentctlStatus = vi.fn();
const mockSetResumeSkipped = vi.fn();
let mockPreventAutoStart = false;
let mockSessionItems: Record<string, { started_at?: string; updated_at?: string; state?: string }> =
  {};

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ request: mockRequest }),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: Record<string, unknown>) => unknown) =>
    selector({
      connection: { status: "connected" },
      taskSessions: { items: mockSessionItems },
      setTaskSession: mockSetTaskSession,
      setSessionAgentctlStatus: mockSetSessionAgentctlStatus,
      setResumeSkipped: mockSetResumeSkipped,
      userSettings: { preventAutoStartAgentOnOpen: mockPreventAutoStart },
      tasks: { resumeSkippedSessionIds: {} },
    }),
  useAppStoreApi: () => ({
    getState: () => ({ taskSessions: { items: mockSessionItems } }),
  }),
}));

import {
  resumeWithSilentFallback,
  useSessionResumption,
  type ResumeStateSetter,
} from "./use-session-resumption";

const TASK_ID = "task-1";
const SESSION_ID = "session-1";

beforeEach(() => {
  vi.clearAllMocks();
  mockPreventAutoStart = false;
  mockSessionItems = { [SESSION_ID]: { started_at: "2026-09-08T17:00:00.000Z" } };
});

function status(overrides: Record<string, unknown> = {}) {
  return {
    task_id: TASK_ID,
    session_id: SESSION_ID,
    state: "WAITING_FOR_INPUT",
    updated_at: "2026-09-08T18:00:00.000Z",
    is_agent_running: false,
    is_resumable: false,
    needs_resume: false,
    ...overrides,
  };
}

function createSetters(): { setters: ResumeStateSetter; errors: (string | null)[] } {
  const errors: (string | null)[] = [];
  return {
    errors,
    setters: {
      setResumptionState: vi.fn(),
      setError: (error) => errors.push(error),
      setNotice: vi.fn(),
      setWorktreePath: vi.fn(),
      setWorktreeBranch: vi.fn(),
      setTaskSession: mockSetTaskSession,
    },
  };
}

describe("useSessionResumption archive lifecycle", () => {
  it("defers all automatic operations while archive state is unknown or archived", async () => {
    const { rerender } = renderHook(
      ({ archived }: { archived: boolean | null }) =>
        useSessionResumption(TASK_ID, SESSION_ID, archived),
      { initialProps: { archived: null as boolean | null } },
    );

    await act(async () => {
      await Promise.resolve();
    });
    expect(mockRequest).not.toHaveBeenCalled();

    rerender({ archived: true });
    await act(async () => {
      await Promise.resolve();
    });
    expect(mockRequest).not.toHaveBeenCalled();
  });

  it("checks once after an active task is confirmed by unarchive", async () => {
    mockRequest.mockResolvedValue(status());
    const { rerender } = renderHook(
      ({ archived }: { archived: boolean | null }) =>
        useSessionResumption(TASK_ID, SESSION_ID, archived),
      { initialProps: { archived: true } },
    );

    rerender({ archived: false });
    await waitFor(() => expect(mockRequest).toHaveBeenCalledTimes(1));
    rerender({ archived: false });
    await act(async () => {
      await Promise.resolve();
    });

    expect(mockRequest).toHaveBeenCalledWith(
      "task.session.status",
      {
        task_id: TASK_ID,
        session_id: SESSION_ID,
      },
      10000,
    );
  });

  it("does not start an agent after unarchive when automatic start is disabled", async () => {
    mockPreventAutoStart = true;
    mockRequest.mockResolvedValue(
      status({
        is_resumable: true,
        needs_resume: true,
        resume_reason: "archive_cancelled_resumable",
      }),
    );

    renderHook(() => useSessionResumption(TASK_ID, SESSION_ID, false));
    await waitFor(() => expect(mockRequest).toHaveBeenCalledTimes(1));

    expect(mockRequest).toHaveBeenCalledTimes(1);
    expect(mockSetResumeSkipped).toHaveBeenCalledWith(SESSION_ID, true);
  });
});

describe("useSessionResumption archive generation guards", () => {
  it("ignores a status response that belongs to the pre-archive generation", async () => {
    const statusRequest = Promise.withResolvers<unknown>();
    mockRequest.mockReturnValueOnce(statusRequest.promise);
    const { rerender } = renderHook(
      ({ archived }: { archived: boolean | null }) =>
        useSessionResumption(TASK_ID, SESSION_ID, archived),
      { initialProps: { archived: false } },
    );
    await waitFor(() => expect(mockRequest).toHaveBeenCalledTimes(1));

    rerender({ archived: true });
    await act(async () => {
      statusRequest.resolve(status({ is_resumable: true, needs_resume: true }));
      await statusRequest.promise;
    });

    expect(mockRequest).toHaveBeenCalledTimes(1);
    expect(mockSetTaskSession).not.toHaveBeenCalled();
  });

  it("does not start the read-only fallback after archiving during resume", async () => {
    const resumeRequest = Promise.withResolvers<unknown>();
    mockRequest
      .mockResolvedValueOnce(status({ is_resumable: true, needs_resume: true }))
      .mockReturnValueOnce(resumeRequest.promise);
    const { rerender } = renderHook(
      ({ archived }: { archived: boolean }) => useSessionResumption(TASK_ID, SESSION_ID, archived),
      { initialProps: { archived: false } },
    );
    await waitFor(() => expect(mockRequest).toHaveBeenCalledTimes(2));

    rerender({ archived: true });
    await act(async () => {
      resumeRequest.resolve({ success: false, error: "resume failed" });
      await resumeRequest.promise;
    });

    expect(mockRequest).toHaveBeenCalledTimes(2);
  });

  it("does not apply a status refresh that resolves after archiving", async () => {
    const refreshRequest = Promise.withResolvers<unknown>();
    mockRequest
      .mockResolvedValueOnce(status({ is_resumable: true, needs_resume: true }))
      .mockResolvedValueOnce({ success: true, state: "STARTING" })
      .mockReturnValueOnce(refreshRequest.promise);
    const { rerender } = renderHook(
      ({ archived }: { archived: boolean }) => useSessionResumption(TASK_ID, SESSION_ID, archived),
      { initialProps: { archived: false } },
    );
    await waitFor(() => expect(mockRequest).toHaveBeenCalledTimes(3));
    const appliedSessionCount = mockSetTaskSession.mock.calls.length;

    rerender({ archived: true });
    await act(async () => {
      refreshRequest.resolve(
        status({ state: "COMPLETED", updated_at: "2026-09-08T19:00:00.000Z" }),
      );
      await refreshRequest.promise;
    });

    expect(mockSetTaskSession).toHaveBeenCalledTimes(appliedSessionCount);
  });

  it("starts a new generation when the same task is unarchived and archived again", async () => {
    const firstStatus = Promise.withResolvers<unknown>();
    const secondStatus = Promise.withResolvers<unknown>();
    mockRequest.mockReturnValueOnce(firstStatus.promise).mockReturnValueOnce(secondStatus.promise);
    const { rerender } = renderHook(
      ({ archived }: { archived: boolean }) => useSessionResumption(TASK_ID, SESSION_ID, archived),
      { initialProps: { archived: false } },
    );
    await waitFor(() => expect(mockRequest).toHaveBeenCalledTimes(1));

    rerender({ archived: true });
    rerender({ archived: false });
    await waitFor(() => expect(mockRequest).toHaveBeenCalledTimes(2));
    rerender({ archived: true });

    await act(async () => {
      firstStatus.resolve(status({ is_resumable: true, needs_resume: true }));
      secondStatus.resolve(status({ is_resumable: true, needs_resume: true }));
      await Promise.all([firstStatus.promise, secondStatus.promise]);
    });

    expect(mockRequest).toHaveBeenCalledTimes(2);
    expect(mockSetTaskSession).not.toHaveBeenCalled();
  });
});

describe("resumeWithSilentFallback archive conflict", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("consumes the typed archive conflict without restoring or showing an error", async () => {
    mockRequest.mockRejectedValueOnce(
      new WebSocketRequestError("Task is archived", "CONFLICT", { kind: "task_archived" }),
    );
    const { setters, errors } = createSetters();

    await resumeWithSilentFallback(TASK_ID, SESSION_ID, null, setters);

    expect(mockRequest).toHaveBeenCalledTimes(1);
    expect(errors.filter((error) => error !== null)).toHaveLength(0);
  });
});
