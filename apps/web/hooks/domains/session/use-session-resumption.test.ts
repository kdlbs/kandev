/* eslint-disable max-lines -- session resumption cases share one lifecycle harness. */

import { act, renderHook, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { buildRestoreWorkspaceRequest } from "@/lib/services/session-launch-helpers";
import { sanitizeWorkspaceRestorationDetails } from "@/lib/state/slices/session-runtime/workspace-restoration";
import { createAppStore } from "@/lib/state/store";

const mockRequest = vi.fn();
const mockSetTaskSession = vi.fn();
const mockSetSessionAgentctlStatus = vi.fn();
const mockSetResumeSkipped = vi.fn();
let mockConnectionStatus = "connected";
let mockPreventAutoStart = false;
let mockSessionItems: Record<string, { started_at?: string; updated_at?: string; state?: string }> =
  {};
let mockWorkspaceRestorationEnabled = false;
let mockWorkspaceStore = createAppStore();

function getMockState() {
  const workspaceState = mockWorkspaceStore.getState();
  return {
    connection: { status: mockConnectionStatus },
    taskSessions: { items: mockSessionItems },
    setTaskSession: mockSetTaskSession,
    setSessionAgentctlStatus: mockSetSessionAgentctlStatus,
    setResumeSkipped: mockSetResumeSkipped,
    userSettings: { preventAutoStartAgentOnOpen: mockPreventAutoStart },
    tasks: { resumeSkippedSessionIds: {} },
    sessionAgentctl: workspaceState.sessionAgentctl,
    ...(mockWorkspaceRestorationEnabled
      ? {
          environmentIdBySessionId: { [SESSION_ID]: "environment-1" },
          workspaceRestoration: workspaceState.workspaceRestoration,
          beginWorkspaceRestoration: workspaceState.beginWorkspaceRestoration,
          completeWorkspaceRestoration: workspaceState.completeWorkspaceRestoration,
          failWorkspaceRestoration: workspaceState.failWorkspaceRestoration,
          clearWorkspaceRestoration: workspaceState.clearWorkspaceRestoration,
          bumpWorkspaceFilesRefresh: workspaceState.bumpWorkspaceFilesRefresh,
        }
      : {}),
  };
}

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ request: mockRequest }),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: Record<string, unknown>) => unknown) => selector(getMockState()),
  useAppStoreApi: () => ({
    getState: getMockState,
  }),
}));

const SESSION_ID = "s1";
const TASK_ID = "t1";
const FAILED_STATE = "FAILED";
const LAUNCH_ACTION = "session.launch";
const STATUS_ACTION = "task.session.status";
const STARTED_AT = "2026-01-01T00:00:00.000Z";
const LATER_AT = "2026-01-02T00:00:00.000Z";
const RESUME_TRANSPORT_ERROR = "Resume transport failed";
const WORKSPACE_RESTORE_ERROR = "Workspace restore failed";

afterEach(() => {
  vi.useRealTimers();
  mockRequest.mockReset();
  mockWorkspaceRestorationEnabled = false;
  mockWorkspaceStore = createAppStore();
});

import {
  decideResumeAction,
  resumeWithSilentFallback,
  useSessionResumption,
  type ResumeStateSetter,
  type ResumptionState,
  type SessionRecoveryFailure,
} from "./use-session-resumption";
import { WebSocketRequestTimeoutError } from "@/lib/ws/client";
import { resumeViaLaunch } from "./use-session-resumption-operations";
import { buildGuardedSetters } from "./use-session-resumption-request-guard";

type SetterCalls = {
  resumptionStates: ResumptionState[];
  errors: (string | null)[];
  notices: (string | null)[];
  recoveryFailures: Array<SessionRecoveryFailure | null>;
  worktreePaths: (string | null)[];
  worktreeBranches: (string | null)[];
  taskSessionStates: string[];
};

function createSetters(): { setters: ResumeStateSetter; calls: SetterCalls } {
  const calls: SetterCalls = {
    resumptionStates: [],
    errors: [],
    notices: [],
    recoveryFailures: [],
    worktreePaths: [],
    worktreeBranches: [],
    taskSessionStates: [],
  };
  const setters: ResumeStateSetter = {
    setResumptionState: (s: ResumptionState) => {
      calls.resumptionStates.push(s);
    },
    setError: (e: string | null) => {
      calls.errors.push(e);
    },
    setNotice: (notice: string | null) => {
      calls.notices.push(notice);
    },
    setRecoveryFailure: (failure) => {
      calls.recoveryFailures.push(failure);
    },
    setWorktreePath: (p: string | null) => {
      calls.worktreePaths.push(p);
    },
    setWorktreeBranch: (b: string | null) => {
      calls.worktreeBranches.push(b);
    },
    setTaskSession: (s: { state: string }) => {
      calls.taskSessionStates.push(s.state);
    },
  };
  return { setters, calls };
}

// eslint-disable-next-line max-lines-per-function -- test describe block, splitting hurts readability
describe("resumeWithSilentFallback", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockConnectionStatus = "connected";
    mockPreventAutoStart = false;
    mockSessionItems = {};
    // tryLaunch logs caught errors via console.error; silence in tests so the
    // expected error paths don't pollute the test output.
    vi.spyOn(console, "error").mockImplementation(() => {});
  });

  it("uses resume on first try when it succeeds, never calling restore_workspace", async () => {
    mockRequest.mockResolvedValueOnce({
      success: true,
      task_id: TASK_ID,
      session_id: SESSION_ID,
      state: "STARTING",
      worktree_path: "/wt/foo",
      worktree_branch: "feature/foo",
    });
    const { setters, calls } = createSetters();

    await resumeWithSilentFallback(TASK_ID, SESSION_ID, null, setters);

    expect(mockRequest).toHaveBeenCalledTimes(1);
    expect(mockRequest).toHaveBeenCalledWith(
      LAUNCH_ACTION,
      expect.objectContaining({ intent: "resume", session_id: SESSION_ID }),
      expect.any(Number),
    );
    expect(calls.resumptionStates).toContain("resumed");
    expect(calls.errors).not.toContain(expect.any(String));
    expect(calls.worktreePaths).toContain("/wt/foo");
  });

  it("publishes STARTING before a delayed resume request resolves", async () => {
    const launchRequest = Promise.withResolvers<unknown>();
    mockRequest.mockReturnValueOnce(launchRequest.promise);
    const { setters, calls } = createSetters();

    const resume = resumeWithSilentFallback(TASK_ID, SESSION_ID, null, setters);
    await waitFor(() => expect(calls.taskSessionStates).toContain("STARTING"));

    launchRequest.resolve({
      success: true,
      task_id: TASK_ID,
      session_id: SESSION_ID,
      state: "WAITING_FOR_INPUT",
    });
    await resume;
  });

  it("does not project STARTING over a newer live session state", async () => {
    mockRequest
      .mockResolvedValueOnce({ success: false, error: RESUME_TRANSPORT_ERROR })
      .mockResolvedValueOnce({ success: false, error: WORKSPACE_RESTORE_ERROR });
    const { setters, calls } = createSetters();
    setters.getLiveSession = () => ({ state: "RUNNING", updated_at: LATER_AT });

    await resumeWithSilentFallback(TASK_ID, SESSION_ID, { state: "IDLE" }, setters);

    expect(calls.taskSessionStates).toEqual([]);
  });

  it("does not roll back an authoritative STARTING update that arrives before rejection", async () => {
    const launchRequest = Promise.withResolvers<unknown>();
    mockRequest.mockReturnValueOnce(launchRequest.promise).mockResolvedValueOnce({
      success: false,
      error: WORKSPACE_RESTORE_ERROR,
    });
    let liveSession: {
      state: string;
      started_at: string;
      updated_at: string;
      queue_incarnation_id: string;
      resume_projection_id?: string;
    } = {
      state: "IDLE",
      started_at: STARTED_AT,
      updated_at: STARTED_AT,
      queue_incarnation_id: "inc-1",
    };
    const { setters, calls } = createSetters();
    setters.getLiveSession = () => liveSession;
    const setTaskSession = setters.setTaskSession;
    setters.setTaskSession = (next) => {
      setTaskSession(next);
      liveSession = { ...liveSession, ...next };
    };

    const resume = resumeWithSilentFallback(TASK_ID, SESSION_ID, liveSession, setters);
    await waitFor(() => expect(calls.taskSessionStates).toContain("STARTING"));

    liveSession = {
      ...liveSession,
      state: "STARTING",
      updated_at: LATER_AT,
      resume_projection_id: undefined,
    };
    launchRequest.resolve({ success: false, error: RESUME_TRANSPORT_ERROR });
    await resume;

    expect(calls.taskSessionStates).toEqual(["STARTING"]);
    expect(liveSession.state).toBe("STARTING");
    expect(liveSession.updated_at).toBe(LATER_AT);
  });

  it("rolls back the optimistic STARTING state when both launch attempts fail", async () => {
    mockRequest
      .mockResolvedValueOnce({ success: false, error: RESUME_TRANSPORT_ERROR })
      .mockResolvedValueOnce({ success: false, error: WORKSPACE_RESTORE_ERROR });
    let liveSession: { state: string; started_at: string; updated_at: string } = {
      state: "IDLE",
      started_at: STARTED_AT,
      updated_at: STARTED_AT,
    };
    const { setters, calls } = createSetters();
    setters.getLiveSession = () => liveSession;
    const setTaskSession = setters.setTaskSession;
    setters.setTaskSession = (next) => {
      setTaskSession(next);
      liveSession = next;
    };

    await resumeWithSilentFallback(TASK_ID, SESSION_ID, liveSession, setters);

    expect(calls.taskSessionStates).toEqual(["STARTING", "IDLE"]);
    expect(liveSession.state).toBe("IDLE");
  });

  it("falls back to restore_workspace silently when resume returns success=false", async () => {
    // 1st call: resume fails. 2nd call: restore_workspace succeeds.
    mockRequest
      .mockResolvedValueOnce({
        success: false,
        task_id: TASK_ID,
        state: FAILED_STATE,
        error: RESUME_TRANSPORT_ERROR,
      })
      .mockResolvedValueOnce({
        success: true,
        task_id: TASK_ID,
        session_id: SESSION_ID,
        state: FAILED_STATE,
        worktree_path: "/wt/foo",
      });
    const { setters, calls } = createSetters();

    await resumeWithSilentFallback(TASK_ID, SESSION_ID, null, setters);

    expect(mockRequest).toHaveBeenCalledTimes(2);
    expect(mockRequest).toHaveBeenNthCalledWith(
      1,
      LAUNCH_ACTION,
      expect.objectContaining({ intent: "resume" }),
      expect.any(Number),
    );
    expect(mockRequest).toHaveBeenNthCalledWith(
      2,
      LAUNCH_ACTION,
      expect.objectContaining({ intent: "restore_workspace" }),
      expect.any(Number),
    );
    // Final state is "resumed" (from successful restore), no error surfaced.
    expect(calls.resumptionStates.at(-1)).toBe("resumed");
    expect(calls.errors.filter((e) => typeof e === "string")).toHaveLength(0);
    expect(calls.notices.at(-1)).toBe("Workspace restored in read-only mode");
    expect(calls.recoveryFailures.at(-1)).toEqual({
      outcome: "workspace_read_only",
      resumeError: RESUME_TRANSPORT_ERROR,
    });
  });

  it("falls back to restore_workspace silently when resume throws", async () => {
    mockRequest.mockRejectedValueOnce(new Error("ws timeout")).mockResolvedValueOnce({
      success: true,
      task_id: TASK_ID,
      session_id: SESSION_ID,
      state: FAILED_STATE,
    });
    const { setters, calls } = createSetters();

    await resumeWithSilentFallback(TASK_ID, SESSION_ID, null, setters);

    expect(mockRequest).toHaveBeenCalledTimes(2);
    expect(calls.resumptionStates.at(-1)).toBe("resumed");
    expect(calls.errors.filter((e) => typeof e === "string")).toHaveLength(0);
  });

  it("surfaces an error only when BOTH resume and restore_workspace fail", async () => {
    mockRequest
      .mockResolvedValueOnce({
        success: false,
        task_id: TASK_ID,
        state: FAILED_STATE,
        error: RESUME_TRANSPORT_ERROR,
      })
      .mockResolvedValueOnce({
        success: false,
        task_id: TASK_ID,
        state: FAILED_STATE,
        error: WORKSPACE_RESTORE_ERROR,
      });
    const { setters, calls } = createSetters();

    await resumeWithSilentFallback(TASK_ID, SESSION_ID, null, setters);

    expect(mockRequest).toHaveBeenCalledTimes(2);
    expect(calls.resumptionStates.at(-1)).toBe("error");
    expect(calls.errors.at(-1)).toBe("Session recovery failed");
    expect(calls.recoveryFailures.at(-1)).toEqual({
      outcome: "recovery_failed",
      resumeError: RESUME_TRANSPORT_ERROR,
      restoreError: WORKSPACE_RESTORE_ERROR,
    });
  });

  it("surfaces an error when both resume and restore_workspace throw", async () => {
    mockRequest
      .mockRejectedValueOnce(new Error("ws closed"))
      .mockRejectedValueOnce(new Error("still closed"));
    const { setters, calls } = createSetters();

    await resumeWithSilentFallback(TASK_ID, SESSION_ID, null, setters);

    expect(mockRequest).toHaveBeenCalledTimes(2);
    expect(calls.resumptionStates.at(-1)).toBe("error");
    expect(calls.errors.at(-1)).toBe("Session recovery failed");
    expect(calls.recoveryFailures.at(-1)).toEqual({
      outcome: "recovery_failed",
      resumeError: "ws closed",
      restoreError: "still closed",
    });
  });

  it("does not optimistically seed agentctl ready after restore_workspace succeeds", async () => {
    mockRequest
      .mockResolvedValueOnce({ success: false, task_id: TASK_ID, state: FAILED_STATE })
      .mockResolvedValueOnce({
        success: true,
        task_id: TASK_ID,
        session_id: SESSION_ID,
        state: FAILED_STATE,
      });
    const { setters } = createSetters();
    const setAgentctlReady = vi.fn();
    setters.setAgentctlReady = setAgentctlReady;

    await resumeWithSilentFallback(TASK_ID, SESSION_ID, null, setters);

    expect(setAgentctlReady).not.toHaveBeenCalled();
  });

  it("keeps workspace-only restore failures out of global recovery feedback", async () => {
    mockRequest.mockResolvedValueOnce({
      success: false,
      task_id: TASK_ID,
      session_id: SESSION_ID,
      state: FAILED_STATE,
      error: WORKSPACE_RESTORE_ERROR,
    });
    const { setters, calls } = createSetters();
    const attempt = {
      taskId: TASK_ID,
      sessionId: SESSION_ID,
      environmentId: "environment-1",
      revision: 1,
      status: "pending" as const,
    };
    const fail = vi.fn(() => true);
    const complete = vi.fn(() => true);
    setters.workspaceRestoration = {
      begin: () => attempt,
      complete,
      fail,
      clear: vi.fn(() => true),
    };

    const restored = await resumeViaLaunch(buildRestoreWorkspaceRequest, {
      taskId: TASK_ID,
      sessionId: SESSION_ID,
      session: null,
      setters,
      canContinue: () => true,
    });

    expect(restored).toBe(false);
    expect(fail).toHaveBeenCalledWith(attempt, expect.any(Error));
    expect(complete).not.toHaveBeenCalled();
    expect(calls.errors.at(-1)).toBeNull();
    expect(calls.recoveryFailures.at(-1)).toBeNull();
  });

  it("clears a stale workspace attempt when restore rejects after navigation", async () => {
    let rejectRestore: ((error: Error) => void) | undefined;
    mockRequest.mockReturnValueOnce(
      new Promise((_, reject) => {
        rejectRestore = reject;
      }),
    );
    const { setters } = createSetters();
    const attempt = {
      taskId: TASK_ID,
      sessionId: SESSION_ID,
      environmentId: "environment-1",
      revision: 1,
      status: "pending" as const,
    };
    const clear = vi.fn(() => true);
    setters.workspaceRestoration = {
      begin: () => attempt,
      complete: vi.fn(() => true),
      fail: vi.fn(() => true),
      clear,
    };
    let current = true;
    const restorePromise = resumeViaLaunch(buildRestoreWorkspaceRequest, {
      taskId: TASK_ID,
      sessionId: SESSION_ID,
      session: null,
      setters,
      canContinue: () => current,
    });

    current = false;
    rejectRestore?.(new Error("late restore failure"));

    expect(await restorePromise).toBe(false);
    expect(clear).toHaveBeenCalledWith(attempt);
  });

  it("does not seed agentctl ready when resume succeeds (new execution will emit its own events)", async () => {
    mockRequest.mockResolvedValueOnce({
      success: true,
      task_id: TASK_ID,
      session_id: SESSION_ID,
      state: "STARTING",
    });
    const { setters } = createSetters();
    const setAgentctlReady = vi.fn();
    setters.setAgentctlReady = setAgentctlReady;

    await resumeWithSilentFallback(TASK_ID, SESSION_ID, null, setters);

    expect(setAgentctlReady).not.toHaveBeenCalled();
  });
});

// eslint-disable-next-line max-lines-per-function -- status and recovery cases share one lifecycle harness.
describe("useSessionResumption", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockConnectionStatus = "connected";
    mockPreventAutoStart = false;
    mockSessionItems = {
      s1: {
        started_at: STARTED_AT,
      },
    };
  });

  it("does not mint client timestamps when status has no updated_at", async () => {
    mockRequest.mockResolvedValueOnce({
      session_id: SESSION_ID,
      task_id: TASK_ID,
      state: "WAITING_FOR_INPUT",
      is_agent_running: false,
      is_resumable: false,
      needs_resume: false,
    });

    renderHook(() => useSessionResumption(TASK_ID, SESSION_ID));

    await waitFor(() => expect(mockSetTaskSession).toHaveBeenCalled());
    expect(mockSetTaskSession).toHaveBeenCalledWith(expect.objectContaining({ updated_at: "" }));
  });

  it("retries a timed out status check before processing the response", async () => {
    vi.useFakeTimers();
    mockRequest
      .mockRejectedValueOnce(new WebSocketRequestTimeoutError(STATUS_ACTION))
      .mockResolvedValueOnce({
        session_id: SESSION_ID,
        task_id: TASK_ID,
        state: "WAITING_FOR_INPUT",
        is_agent_running: false,
        is_resumable: false,
        needs_resume: false,
      });

    renderHook(() => useSessionResumption(TASK_ID, SESSION_ID));
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1000);
    });

    expect(mockRequest).toHaveBeenCalledTimes(2);
    expect(mockRequest).toHaveBeenNthCalledWith(
      1,
      STATUS_ACTION,
      { task_id: TASK_ID, session_id: SESSION_ID },
      10000,
    );
    expect(mockSetTaskSession).toHaveBeenCalledWith(
      expect.objectContaining({ state: "WAITING_FOR_INPUT" }),
    );
  });

  it("surfaces a status timeout as status-unavailable feedback after the retry budget", async () => {
    vi.useFakeTimers();
    mockRequest
      .mockRejectedValueOnce(new WebSocketRequestTimeoutError(STATUS_ACTION))
      .mockRejectedValueOnce(new WebSocketRequestTimeoutError(STATUS_ACTION));

    const { result } = renderHook(() => useSessionResumption(TASK_ID, SESSION_ID));
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
      await vi.advanceTimersByTimeAsync(1000);
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(result.current.resumptionState).toBe("error");
    expect(result.current.error).toBeNull();
    expect(result.current.recoveryFailure).toEqual({
      outcome: "status_unavailable",
      kind: "timeout",
      statusError: "WebSocket request timed out: task.session.status",
    });
    expect(mockRequest).toHaveBeenCalledTimes(2);

    mockRequest.mockResolvedValueOnce({
      session_id: SESSION_ID,
      task_id: TASK_ID,
      state: "RUNNING",
      is_agent_running: true,
      is_resumable: false,
      needs_resume: false,
    });
    await act(async () => {
      await result.current.retrySessionStatus();
    });

    expect(mockRequest).toHaveBeenCalledTimes(3);
    expect(mockRequest).not.toHaveBeenCalledWith(
      LAUNCH_ACTION,
      expect.anything(),
      expect.anything(),
    );
    expect(result.current.recoveryFailure).toBeNull();
    expect(result.current.resumptionState).toBe("running");
  });

  it("keeps a workspace restore failure as launch feedback with a launch retry", async () => {
    mockWorkspaceRestorationEnabled = true;
    mockRequest
      .mockResolvedValueOnce({
        session_id: SESSION_ID,
        task_id: TASK_ID,
        state: "WAITING_FOR_INPUT",
        is_agent_running: false,
        is_resumable: false,
        needs_resume: false,
        needs_workspace_restore: true,
      })
      .mockRejectedValueOnce(new Error(WORKSPACE_RESTORE_ERROR));

    const { result } = renderHook(() => useSessionResumption(TASK_ID, SESSION_ID));

    await waitFor(() => expect(result.current.workspaceRestoration.status).toBe("error"));
    expect(result.current.workspaceRestoration.attempt?.details).toBe(WORKSPACE_RESTORE_ERROR);
    expect(result.current.resumptionState).toBe("error");
    expect(result.current.error).toBeNull();
    expect(result.current.recoveryFailure).toBeNull();
    expect(mockRequest).toHaveBeenNthCalledWith(
      2,
      LAUNCH_ACTION,
      expect.objectContaining({ intent: "restore_workspace" }),
      expect.any(Number),
    );

    mockRequest.mockResolvedValueOnce({
      success: true,
      task_id: TASK_ID,
      session_id: SESSION_ID,
      state: "STARTING",
    });
    await act(async () => {
      await result.current.resumeSession();
    });

    expect(mockRequest).toHaveBeenNthCalledWith(
      3,
      LAUNCH_ACTION,
      expect.objectContaining({ intent: "resume" }),
      expect.any(Number),
    );
  });

  it("keeps a backend status error visible after a timeout retry without launching", async () => {
    vi.useFakeTimers();
    mockRequest
      .mockRejectedValueOnce(new WebSocketRequestTimeoutError(STATUS_ACTION))
      .mockRejectedValueOnce(new WebSocketRequestTimeoutError(STATUS_ACTION));

    const { result } = renderHook(() => useSessionResumption(TASK_ID, SESSION_ID));
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
      await vi.advanceTimersByTimeAsync(1000);
      await Promise.resolve();
      await Promise.resolve();
    });

    mockRequest.mockResolvedValueOnce({
      session_id: SESSION_ID,
      task_id: TASK_ID,
      state: "WAITING_FOR_INPUT",
      is_agent_running: false,
      is_resumable: false,
      needs_resume: false,
      error: "session not found",
    });
    await act(async () => {
      await result.current.retrySessionStatus();
    });

    expect(result.current.resumptionState).toBe("error");
    expect(result.current.error).toBe("session not found");
    expect(result.current.recoveryFailure).toBeNull();
    expect(mockRequest.mock.calls.filter(([action]) => action === LAUNCH_ACTION)).toHaveLength(0);
  });

  it("refreshes the status and embedded editor capability after a successful resume", async () => {
    mockRequest
      .mockResolvedValueOnce({
        session_id: SESSION_ID,
        task_id: TASK_ID,
        state: "WAITING_FOR_INPUT",
        is_agent_running: false,
        is_resumable: true,
        needs_resume: true,
        capabilities: { embedded_vscode: false },
      })
      .mockResolvedValueOnce({
        success: true,
        task_id: TASK_ID,
        session_id: SESSION_ID,
        state: "STARTING",
      })
      .mockResolvedValueOnce({
        session_id: SESSION_ID,
        task_id: TASK_ID,
        state: "STARTING",
        is_agent_running: true,
        is_resumable: false,
        needs_resume: false,
        capabilities: { embedded_vscode: true },
      });

    const { result } = renderHook(() => useSessionResumption(TASK_ID, SESSION_ID));

    await waitFor(() => {
      expect(result.current.sessionStatus?.capabilities?.embedded_vscode).toBe(true);
    });
    expect(mockRequest).toHaveBeenLastCalledWith(
      STATUS_ACTION,
      {
        task_id: TASK_ID,
        session_id: SESSION_ID,
      },
      10000,
    );
  });

  it("clears stale feedback when an external recovery makes the live session active", async () => {
    mockPreventAutoStart = true;
    mockSessionItems = {
      s1: {
        started_at: STARTED_AT,
        updated_at: STARTED_AT,
        state: FAILED_STATE,
      },
    };
    mockRequest.mockResolvedValueOnce({
      session_id: SESSION_ID,
      task_id: TASK_ID,
      state: FAILED_STATE,
      is_agent_running: false,
      is_resumable: true,
      needs_resume: true,
      updated_at: STARTED_AT,
    });

    const { result, rerender } = renderHook(() => useSessionResumption(TASK_ID, SESSION_ID));
    await waitFor(() => expect(mockSetResumeSkipped).toHaveBeenCalledWith(SESSION_ID, true));

    mockRequest.mockResolvedValueOnce({
      success: false,
      task_id: TASK_ID,
      session_id: SESSION_ID,
      state: FAILED_STATE,
      error: "Stale automatic recovery failure",
    });
    await act(async () => {
      await result.current.resumeSession();
    });
    expect(result.current.error).toBe("Stale automatic recovery failure");

    mockSessionItems.s1.state = "RUNNING";
    rerender();

    await waitFor(() => expect(result.current.error).toBeNull());
    expect(result.current.notice).toBeNull();
  });
});

describe("useSessionResumption prevent-auto-start gate", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockConnectionStatus = "connected";
    mockPreventAutoStart = true;
    mockSessionItems = {
      s1: {
        started_at: STARTED_AT,
        updated_at: STARTED_AT,
        state: "WAITING_FOR_INPUT",
      },
    };
  });

  it("skips the auto-resume, records the skip, and stays idle when the preference is on", async () => {
    mockRequest.mockResolvedValueOnce({
      session_id: SESSION_ID,
      task_id: TASK_ID,
      state: "WAITING_FOR_INPUT",
      is_agent_running: false,
      is_resumable: true,
      needs_resume: true,
      updated_at: STARTED_AT,
    });

    const { result } = renderHook(() => useSessionResumption(TASK_ID, SESSION_ID));

    await waitFor(() => {
      expect(mockSetResumeSkipped).toHaveBeenCalledWith(SESSION_ID, true);
    });
    // No resume/restore launch attempt happened.
    const launchCalls = mockRequest.mock.calls.filter(([action]) => action === LAUNCH_ACTION);
    expect(launchCalls).toHaveLength(0);
    expect(result.current.resumptionState).toBe("idle");
  });

  it("auto-resumes when the preference is off (unchanged behavior)", async () => {
    mockPreventAutoStart = false;
    mockRequest
      .mockResolvedValueOnce({
        session_id: SESSION_ID,
        task_id: TASK_ID,
        state: "WAITING_FOR_INPUT",
        is_agent_running: false,
        is_resumable: true,
        needs_resume: true,
        updated_at: STARTED_AT,
      })
      .mockResolvedValueOnce({
        success: true,
        task_id: TASK_ID,
        session_id: SESSION_ID,
        state: "STARTING",
      });

    renderHook(() => useSessionResumption(TASK_ID, SESSION_ID));

    await waitFor(() => {
      expect(mockRequest).toHaveBeenCalledWith(
        LAUNCH_ACTION,
        expect.objectContaining({ session_id: SESSION_ID, intent: "resume" }),
        expect.any(Number),
      );
    });
    expect(mockSetResumeSkipped).not.toHaveBeenCalled();
  });

  it("does not record the skip when the live session is already RUNNING (stale status race)", async () => {
    // A status response taken before the agent started can arrive while the
    // live row is RUNNING. The skip must not be recorded, or a Start button
    // would appear beside a running agent.
    mockSessionItems = {
      s1: {
        started_at: STARTED_AT,
        updated_at: LATER_AT,
        state: "RUNNING",
      },
    };
    mockRequest.mockResolvedValueOnce({
      session_id: SESSION_ID,
      task_id: TASK_ID,
      state: "IDLE",
      is_agent_running: false,
      is_resumable: true,
      needs_resume: true,
      updated_at: STARTED_AT, // older than the live RUNNING row
    });

    renderHook(() => useSessionResumption(TASK_ID, SESSION_ID));

    await waitFor(() => {
      expect(mockRequest).toHaveBeenCalledWith(
        STATUS_ACTION,
        {
          task_id: TASK_ID,
          session_id: SESSION_ID,
        },
        10000,
      );
    });
    // The skip branch must consult the live row (RUNNING) and refuse.
    expect(mockSetResumeSkipped).not.toHaveBeenCalled();
    expect(mockSetTaskSession).not.toHaveBeenCalled();
  });
});

// eslint-disable-next-line max-lines-per-function -- test describe block, splitting hurts readability
describe("useSessionResumption monotonic terminal hydration", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockConnectionStatus = "connected";
    mockPreventAutoStart = false;
  });

  it("rejects an older terminal status over a live terminal state", async () => {
    mockSessionItems = {
      s1: {
        started_at: STARTED_AT,
        updated_at: LATER_AT,
        state: "FAILED",
      },
    };
    mockRequest.mockResolvedValueOnce({
      session_id: "s1",
      task_id: "t1",
      state: "COMPLETED",
      is_agent_running: false,
      is_resumable: false,
      needs_resume: false,
      updated_at: STARTED_AT, // older than the live FAILED
    });

    renderHook(() => useSessionResumption("t1", "s1"));

    await waitFor(() => {
      expect(mockRequest).toHaveBeenCalledWith(
        STATUS_ACTION,
        {
          task_id: "t1",
          session_id: "s1",
        },
        10000,
      );
    });
    // The stale COMPLETED must not overwrite the newer live FAILED.
    expect(mockSetTaskSession).not.toHaveBeenCalledWith(
      expect.objectContaining({ state: "COMPLETED" }),
    );
  });

  it("rejects an older non-terminal status over a live terminal state", async () => {
    mockSessionItems = {
      s1: {
        started_at: "2026-01-01T00:00:00.000Z",
        updated_at: LATER_AT,
        state: "FAILED",
      },
    };
    mockRequest.mockResolvedValueOnce({
      session_id: "s1",
      task_id: "t1",
      state: "STARTING",
      is_agent_running: false,
      is_resumable: false,
      needs_resume: false,
      updated_at: STARTED_AT, // older than the live FAILED
    });

    renderHook(() => useSessionResumption("t1", "s1"));

    await waitFor(() => {
      expect(mockRequest).toHaveBeenCalledWith(
        STATUS_ACTION,
        {
          task_id: "t1",
          session_id: "s1",
        },
        10000,
      );
    });
    // The stale STARTING must not overwrite the newer live FAILED.
    expect(mockSetTaskSession).not.toHaveBeenCalledWith(
      expect.objectContaining({ state: "STARTING" }),
    );
  });

  it("accepts a newer terminal status over a live terminal state", async () => {
    mockSessionItems = {
      s1: {
        started_at: STARTED_AT,
        updated_at: STARTED_AT,
        state: "FAILED",
      },
    };
    mockRequest.mockResolvedValueOnce({
      session_id: "s1",
      task_id: "t1",
      state: "COMPLETED",
      is_agent_running: false,
      is_resumable: false,
      needs_resume: false,
      updated_at: LATER_AT, // newer
    });

    renderHook(() => useSessionResumption("t1", "s1"));

    await waitFor(() => {
      expect(mockSetTaskSession).toHaveBeenCalledWith(
        expect.objectContaining({ state: "COMPLETED" }),
      );
    });
  });

  it("rejects an older status over a live WAITING_FOR_INPUT state", async () => {
    mockSessionItems = {
      s1: {
        started_at: STARTED_AT,
        updated_at: LATER_AT,
        state: "WAITING_FOR_INPUT",
      },
    };
    mockRequest.mockResolvedValueOnce({
      session_id: "s1",
      task_id: "t1",
      state: "IDLE",
      is_agent_running: false,
      is_resumable: true,
      needs_resume: true,
      updated_at: STARTED_AT, // older than the live WAITING_FOR_INPUT row
    });

    renderHook(() => useSessionResumption("t1", "s1"));

    await waitFor(() => {
      expect(mockRequest).toHaveBeenCalledWith(
        STATUS_ACTION,
        {
          task_id: "t1",
          session_id: "s1",
        },
        10000,
      );
    });
    // WAITING_FOR_INPUT means the agent is alive; a stale older response must
    // not downgrade it to a stopped-looking state.
    expect(mockSetTaskSession).not.toHaveBeenCalledWith(expect.objectContaining({ state: "IDLE" }));
  });
});

describe("useSessionResumption completed-session admission", () => {
  it("selects workspace restoration for a completed session without selecting agent resume", () => {
    const action = decideResumeAction(
      {
        session_id: SESSION_ID,
        task_id: TASK_ID,
        state: "COMPLETED",
        is_agent_running: false,
        is_resumable: true,
        needs_resume: true,
        needs_workspace_restore: true,
      },
      false,
    );

    expect(action).toBe("restore");
  });

  it.each([false, true])(
    "does not auto-resume a completed session when preventAutoStart is %s",
    async (preventAutoStart) => {
      vi.clearAllMocks();
      mockConnectionStatus = "connected";
      mockPreventAutoStart = preventAutoStart;
      mockSessionItems = {
        s1: {
          started_at: STARTED_AT,
          updated_at: LATER_AT,
          state: "COMPLETED",
        },
      };
      mockRequest.mockResolvedValueOnce({
        session_id: SESSION_ID,
        task_id: TASK_ID,
        state: "COMPLETED",
        is_agent_running: false,
        is_resumable: true,
        needs_resume: true,
        updated_at: LATER_AT,
      });

      renderHook(() => useSessionResumption(TASK_ID, SESSION_ID));

      await waitFor(() => {
        expect(mockRequest).toHaveBeenCalledWith(
          STATUS_ACTION,
          {
            task_id: TASK_ID,
            session_id: SESSION_ID,
          },
          10000,
        );
      });
      expect(mockRequest).not.toHaveBeenCalledWith(
        LAUNCH_ACTION,
        expect.anything(),
        expect.anything(),
      );
    },
  );
});

describe("useSessionResumption resume-skipped clearing on running status", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockConnectionStatus = "connected";
    mockPreventAutoStart = true;
    mockSessionItems = {
      s1: {
        started_at: STARTED_AT,
        updated_at: STARTED_AT,
        state: "WAITING_FOR_INPUT",
      },
    };
  });

  it("clears the resume-skipped marker when the status confirms the agent is running", async () => {
    mockRequest.mockResolvedValueOnce({
      session_id: "s1",
      task_id: "t1",
      state: "RUNNING",
      is_agent_running: true,
      is_resumable: false,
      needs_resume: false,
      updated_at: LATER_AT,
    });

    renderHook(() => useSessionResumption("t1", "s1"));

    await waitFor(() => {
      expect(mockSetResumeSkipped).toHaveBeenCalledWith("s1", false);
    });
  });
});

// eslint-disable-next-line max-lines-per-function -- navigation and response races share one harness.
describe("useSessionResumption stale-callback guard after navigation", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockConnectionStatus = "connected";
    mockPreventAutoStart = true;
    mockSessionItems = {
      s1: {
        started_at: STARTED_AT,
        updated_at: STARTED_AT,
        state: "WAITING_FOR_INPUT",
      },
    };
  });

  it("does not record a skip or write state for the previous session when a status response lands after navigation", async () => {
    const { promise, resolve: resolveStatus } = Promise.withResolvers<unknown>();
    mockRequest.mockReturnValueOnce(promise);

    const { rerender } = renderHook(
      ({ sid }: { sid: string }) => useSessionResumption(TASK_ID, sid),
      { initialProps: { sid: SESSION_ID } },
    );

    await waitFor(() => {
      expect(mockRequest).toHaveBeenCalledWith(
        STATUS_ACTION,
        {
          task_id: TASK_ID,
          session_id: SESSION_ID,
        },
        10000,
      );
    });

    // Navigate to another session before the first status response resolves.
    rerender({ sid: "s2" });

    await act(async () => {
      resolveStatus({
        session_id: SESSION_ID,
        task_id: TASK_ID,
        state: "IDLE",
        is_agent_running: false,
        is_resumable: true,
        needs_resume: true,
        updated_at: STARTED_AT,
      });
    });

    // The stale response for the switched-away session must neither record a
    // resume-skipped marker nor write its session row.
    expect(mockSetResumeSkipped).not.toHaveBeenCalled();
    expect(mockSetTaskSession).not.toHaveBeenCalled();
  });

  it.each([
    ["success", { success: true, state: "WAITING_FOR_INPUT" }],
    ["rejection", new Error("obsolete restore rejected")],
  ] as const)(
    "cleans up an obsolete automatic restore when navigation A to B to A receives a %s response",
    async (_outcome, response) => {
      const workspaceStore = createAppStore();
      const { setters } = createSetters();
      setters.workspaceRestoration = {
        begin: (taskId, sessionId) =>
          workspaceStore.getState().beginWorkspaceRestoration(taskId, sessionId, "env-a"),
        complete: (attempt) => workspaceStore.getState().completeWorkspaceRestoration(attempt),
        fail: (attempt, details) =>
          workspaceStore
            .getState()
            .failWorkspaceRestoration(attempt, sanitizeWorkspaceRestorationDetails(details)),
        clear: (attempt) => workspaceStore.getState().clearWorkspaceRestoration(attempt),
      };
      const activeRequestRef = {
        current: { key: "task-a/session-a", generation: 1 },
      };
      const capturedRequest = activeRequestRef.current;
      const guardedSetters = buildGuardedSetters(activeRequestRef, capturedRequest, setters);
      const launchResponse = Promise.withResolvers<unknown>();
      mockRequest.mockReturnValueOnce(launchResponse.promise);

      const restore = resumeViaLaunch(buildRestoreWorkspaceRequest, {
        taskId: "task-a",
        sessionId: "session-a",
        session: null,
        setters: guardedSetters,
        canContinue: () =>
          activeRequestRef.current.key === capturedRequest.key &&
          activeRequestRef.current.generation === capturedRequest.generation,
      });

      await waitFor(() => {
        expect(
          workspaceStore.getState().workspaceRestoration.byEnvironmentId["env-a"],
        ).toMatchObject({
          taskId: "task-a",
          sessionId: "session-a",
          status: "pending",
        });
      });

      // Navigate to B before A's workspace request settles.
      activeRequestRef.current = { key: "task-b/session-b", generation: 2 };
      if (response instanceof Error) launchResponse.reject(response);
      else launchResponse.resolve(response);
      await expect(restore).resolves.toBe(false);

      // Returning to A must not find the obsolete pending attempt.
      activeRequestRef.current = { key: "task-a/session-a", generation: 3 };
      expect(
        workspaceStore.getState().workspaceRestoration.byEnvironmentId["env-a"],
      ).toBeUndefined();
      const retry = workspaceStore
        .getState()
        .beginWorkspaceRestoration("task-a", "session-a", "env-a");
      expect(retry).toMatchObject({
        taskId: "task-a",
        sessionId: "session-a",
        status: "pending",
      });
    },
  );
});
