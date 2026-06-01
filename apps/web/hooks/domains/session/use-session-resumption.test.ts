import { renderHook, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";

const mockRequest = vi.fn();
const mockMergeTaskSession = vi.fn();
let mockConnectionStatus = "connected";
// The TaskSession the migrated hook reads via useTaskSessionById (TQ-backed).
let mockSession: { started_at?: string; updated_at?: string } | null = null;

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ request: mockRequest }),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: Record<string, unknown>) => unknown) =>
    selector({ connection: { status: mockConnectionStatus } }),
}));

// The migrated hook reads the session from the TQ by-id cache and writes
// resumed state back through mergeTaskSessionIntoCache (not the Zustand slice).
vi.mock("@/hooks/domains/session/use-task-session-by-id", () => ({
  useTaskSessionById: () => mockSession,
}));

vi.mock("@/lib/query/cache/task-session-cache", () => ({
  mergeTaskSessionIntoCache: (...args: unknown[]) => mockMergeTaskSession(...args),
}));

vi.mock("@tanstack/react-query", () => ({
  useQueryClient: () => ({}),
}));

import {
  resumeWithSilentFallback,
  useSessionResumption,
  type ResumeStateSetter,
  type ResumptionState,
} from "./use-session-resumption";

type SetterCalls = {
  resumptionStates: ResumptionState[];
  errors: (string | null)[];
  worktreePaths: (string | null)[];
  worktreeBranches: (string | null)[];
  taskSessionStates: string[];
};

function createSetters(): { setters: ResumeStateSetter; calls: SetterCalls } {
  const calls: SetterCalls = {
    resumptionStates: [],
    errors: [],
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
    mockSession = null;
    // tryLaunch logs caught errors via console.error; silence in tests so the
    // expected error paths don't pollute the test output.
    vi.spyOn(console, "error").mockImplementation(() => {});
  });

  it("uses resume on first try when it succeeds, never calling restore_workspace", async () => {
    mockRequest.mockResolvedValueOnce({
      success: true,
      task_id: "t1",
      session_id: "s1",
      state: "STARTING",
      worktree_path: "/wt/foo",
      worktree_branch: "feature/foo",
    });
    const { setters, calls } = createSetters();

    await resumeWithSilentFallback("t1", "s1", null, setters);

    expect(mockRequest).toHaveBeenCalledTimes(1);
    expect(mockRequest).toHaveBeenCalledWith(
      "session.launch",
      expect.objectContaining({ intent: "resume", session_id: "s1" }),
      expect.any(Number),
    );
    expect(calls.resumptionStates).toContain("resumed");
    expect(calls.errors).not.toContain(expect.any(String));
    expect(calls.worktreePaths).toContain("/wt/foo");
  });

  it("falls back to restore_workspace silently when resume returns success=false", async () => {
    // 1st call: resume fails. 2nd call: restore_workspace succeeds.
    mockRequest
      .mockResolvedValueOnce({ success: false, task_id: "t1", state: "FAILED" })
      .mockResolvedValueOnce({
        success: true,
        task_id: "t1",
        session_id: "s1",
        state: "FAILED",
        worktree_path: "/wt/foo",
      });
    const { setters, calls } = createSetters();

    await resumeWithSilentFallback("t1", "s1", null, setters);

    expect(mockRequest).toHaveBeenCalledTimes(2);
    expect(mockRequest).toHaveBeenNthCalledWith(
      1,
      "session.launch",
      expect.objectContaining({ intent: "resume" }),
      expect.any(Number),
    );
    expect(mockRequest).toHaveBeenNthCalledWith(
      2,
      "session.launch",
      expect.objectContaining({ intent: "restore_workspace" }),
      expect.any(Number),
    );
    // Final state is "resumed" (from successful restore), no error surfaced.
    expect(calls.resumptionStates.at(-1)).toBe("resumed");
    expect(calls.errors.filter((e) => typeof e === "string")).toHaveLength(0);
  });

  it("falls back to restore_workspace silently when resume throws", async () => {
    mockRequest
      .mockRejectedValueOnce(new Error("ws timeout"))
      .mockResolvedValueOnce({ success: true, task_id: "t1", session_id: "s1", state: "FAILED" });
    const { setters, calls } = createSetters();

    await resumeWithSilentFallback("t1", "s1", null, setters);

    expect(mockRequest).toHaveBeenCalledTimes(2);
    expect(calls.resumptionStates.at(-1)).toBe("resumed");
    expect(calls.errors.filter((e) => typeof e === "string")).toHaveLength(0);
  });

  it("surfaces an error only when BOTH resume and restore_workspace fail", async () => {
    mockRequest
      .mockResolvedValueOnce({ success: false, task_id: "t1", state: "FAILED" })
      .mockResolvedValueOnce({ success: false, task_id: "t1", state: "FAILED" });
    const { setters, calls } = createSetters();

    await resumeWithSilentFallback("t1", "s1", null, setters);

    expect(mockRequest).toHaveBeenCalledTimes(2);
    expect(calls.resumptionStates.at(-1)).toBe("error");
    expect(calls.errors.at(-1)).toBe(
      "Failed to resume session — workspace restore also unavailable",
    );
  });

  it("surfaces an error when both resume and restore_workspace throw", async () => {
    mockRequest
      .mockRejectedValueOnce(new Error("ws closed"))
      .mockRejectedValueOnce(new Error("still closed"));
    const { setters, calls } = createSetters();

    await resumeWithSilentFallback("t1", "s1", null, setters);

    expect(mockRequest).toHaveBeenCalledTimes(2);
    expect(calls.resumptionStates.at(-1)).toBe("error");
    expect(calls.errors.at(-1)).toBe(
      "Failed to resume session — workspace restore also unavailable",
    );
  });

  it("seeds agentctl ready when restore_workspace fallback succeeds", async () => {
    mockRequest
      .mockResolvedValueOnce({ success: false, task_id: "t1", state: "FAILED" })
      .mockResolvedValueOnce({
        success: true,
        task_id: "t1",
        session_id: "s1",
        state: "FAILED",
      });
    const { setters } = createSetters();
    const setAgentctlReady = vi.fn();
    setters.setAgentctlReady = setAgentctlReady;

    await resumeWithSilentFallback("t1", "s1", null, setters);

    expect(setAgentctlReady).toHaveBeenCalledTimes(1);
    expect(setAgentctlReady).toHaveBeenCalledWith("s1");
  });

  it("does not seed agentctl ready when resume succeeds (new execution will emit its own events)", async () => {
    mockRequest.mockResolvedValueOnce({
      success: true,
      task_id: "t1",
      session_id: "s1",
      state: "STARTING",
    });
    const { setters } = createSetters();
    const setAgentctlReady = vi.fn();
    setters.setAgentctlReady = setAgentctlReady;

    await resumeWithSilentFallback("t1", "s1", null, setters);

    expect(setAgentctlReady).not.toHaveBeenCalled();
  });
});

describe("useSessionResumption", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockConnectionStatus = "connected";
    mockSession = {
      started_at: "2026-01-01T00:00:00.000Z",
    };
  });

  it("does not mint client timestamps when status has no updated_at", async () => {
    mockRequest.mockResolvedValueOnce({
      session_id: "s1",
      task_id: "t1",
      state: "WAITING_FOR_INPUT",
      is_agent_running: false,
      is_resumable: false,
      needs_resume: false,
    });

    renderHook(() => useSessionResumption("t1", "s1"));

    // The migrated hook writes resumed state via mergeTaskSessionIntoCache(qc, session).
    await waitFor(() => expect(mockMergeTaskSession).toHaveBeenCalled());
    expect(mockMergeTaskSession).toHaveBeenCalledWith(
      expect.anything(),
      expect.objectContaining({ updated_at: "" }),
    );
  });
});
