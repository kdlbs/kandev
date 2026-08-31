import { act, renderHook, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";

const mockRequest = vi.fn();
const mockSetTaskSession = vi.fn();
const mockSetSessionAgentctlStatus = vi.fn();
const mockSetResumeSkipped = vi.fn();
let mockConnectionStatus = "connected";
let mockPreventAutoStart = false;
let mockSessionItems: Record<string, { started_at?: string; updated_at?: string; state?: string }> =
  {};

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ request: mockRequest }),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: Record<string, unknown>) => unknown) =>
    selector({
      connection: { status: mockConnectionStatus },
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

const SESSION_ID = "s1";
const TASK_ID = "t1";
const FAILED_STATE = "FAILED";
const LAUNCH_ACTION = "session.launch";
const STATUS_ACTION = "task.session.status";
const STARTED_AT = "2026-01-01T00:00:00.000Z";
const LATER_AT = "2026-01-02T00:00:00.000Z";

import {
  resumeWithSilentFallback,
  useSessionResumption,
  type ResumeStateSetter,
  type ResumptionState,
} from "./use-session-resumption";

type SetterCalls = {
  resumptionStates: ResumptionState[];
  errors: (string | null)[];
  notices: (string | null)[];
  worktreePaths: (string | null)[];
  worktreeBranches: (string | null)[];
  taskSessionStates: string[];
};

function createSetters(): { setters: ResumeStateSetter; calls: SetterCalls } {
  const calls: SetterCalls = {
    resumptionStates: [],
    errors: [],
    notices: [],
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

  it("falls back to restore_workspace silently when resume returns success=false", async () => {
    // 1st call: resume fails. 2nd call: restore_workspace succeeds.
    mockRequest
      .mockResolvedValueOnce({
        success: false,
        task_id: TASK_ID,
        state: FAILED_STATE,
        error: "Resume transport failed",
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
    expect(calls.notices.at(-1)).toBe(
      "Resume failed: Resume transport failed. The workspace was restored in read-only mode.",
    );
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
        error: "Resume transport failed",
      })
      .mockResolvedValueOnce({
        success: false,
        task_id: TASK_ID,
        state: FAILED_STATE,
        error: "Workspace restore failed",
      });
    const { setters, calls } = createSetters();

    await resumeWithSilentFallback(TASK_ID, SESSION_ID, null, setters);

    expect(mockRequest).toHaveBeenCalledTimes(2);
    expect(calls.resumptionStates.at(-1)).toBe("error");
    expect(calls.errors.at(-1)).toBe(
      "Resume failed: Resume transport failed. Workspace restore failed: Workspace restore failed.",
    );
  });

  it("surfaces an error when both resume and restore_workspace throw", async () => {
    mockRequest
      .mockRejectedValueOnce(new Error("ws closed"))
      .mockRejectedValueOnce(new Error("still closed"));
    const { setters, calls } = createSetters();

    await resumeWithSilentFallback(TASK_ID, SESSION_ID, null, setters);

    expect(mockRequest).toHaveBeenCalledTimes(2);
    expect(calls.resumptionStates.at(-1)).toBe("error");
    expect(calls.errors.at(-1)).toBe(
      "Resume failed: ws closed. Workspace restore failed: still closed.",
    );
  });

  it("seeds agentctl ready when restore_workspace fallback succeeds", async () => {
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

    expect(setAgentctlReady).toHaveBeenCalledTimes(1);
    expect(setAgentctlReady).toHaveBeenCalledWith(SESSION_ID);
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
    expect(mockRequest).toHaveBeenLastCalledWith(STATUS_ACTION, {
      task_id: TASK_ID,
      session_id: SESSION_ID,
    });
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
      expect(mockRequest).toHaveBeenCalledWith(STATUS_ACTION, {
        task_id: TASK_ID,
        session_id: SESSION_ID,
      });
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
      expect(mockRequest).toHaveBeenCalledWith(STATUS_ACTION, {
        task_id: "t1",
        session_id: "s1",
      });
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
      expect(mockRequest).toHaveBeenCalledWith(STATUS_ACTION, {
        task_id: "t1",
        session_id: "s1",
      });
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
      expect(mockRequest).toHaveBeenCalledWith(STATUS_ACTION, {
        task_id: "t1",
        session_id: "s1",
      });
    });
    // WAITING_FOR_INPUT means the agent is alive; a stale older response must
    // not downgrade it to a stopped-looking state.
    expect(mockSetTaskSession).not.toHaveBeenCalledWith(expect.objectContaining({ state: "IDLE" }));
  });
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
});
