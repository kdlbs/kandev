import { describe, it, expect, vi, beforeEach } from "vitest";
import { resumeWithSilentFallback } from "./use-session-resumption-operations";
import type { ResumeStateSetter, ResumptionState } from "./use-session-resumption";
import { WebSocketRequestError } from "@/lib/ws/client";

const mockRequest = vi.fn();

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ request: mockRequest }),
}));

const SESSION_ID = "s1";
const TASK_ID = "t1";

type SetterCalls = {
  resumptionStates: ResumptionState[];
  errors: (string | null)[];
};

function createSetters(): { setters: ResumeStateSetter; calls: SetterCalls } {
  const calls: SetterCalls = { resumptionStates: [], errors: [] };
  const setters: ResumeStateSetter = {
    setResumptionState: (s) => calls.resumptionStates.push(s),
    setError: (e) => calls.errors.push(e),
    setNotice: () => {},
    setWorktreePath: () => {},
    setWorktreeBranch: () => {},
    setTaskSession: () => {},
  };
  return { setters, calls };
}

describe("resumeWithSilentFallback recovery guard refusals", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    // tryLaunch logs caught errors via console.error; silence in tests so the
    // expected error paths don't pollute the test output.
    vi.spyOn(console, "error").mockImplementation(() => {});
  });

  it("skips the restore_workspace fallback and shows the retryable guard message", async () => {
    mockRequest.mockRejectedValueOnce(
      new WebSocketRequestError("blocked", "CONFLICT", {
        kind: "session_recovery_in_progress",
        retryable: true,
        session_id: SESSION_ID,
      }),
    );
    const { setters, calls } = createSetters();

    const ok = await resumeWithSilentFallback(TASK_ID, SESSION_ID, null, setters);

    expect(ok).toBe(false);
    expect(mockRequest).toHaveBeenCalledTimes(1);
    expect(calls.resumptionStates.at(-1)).toBe("error");
    expect(calls.errors.at(-1)).toBe(
      "This session's agent is still being reconciled after the backend restarted. This clears automatically; try again in a few seconds.",
    );
  });

  it("shows the non-retryable guard message when resume is refused by an unstoppable prior agent", async () => {
    mockRequest.mockRejectedValueOnce(
      new WebSocketRequestError("blocked", "UNAVAILABLE", {
        kind: "session_recovery_unstoppable",
        retryable: false,
        session_id: SESSION_ID,
      }),
    );
    const { setters, calls } = createSetters();

    const ok = await resumeWithSilentFallback(TASK_ID, SESSION_ID, null, setters);

    expect(ok).toBe(false);
    expect(mockRequest).toHaveBeenCalledTimes(1);
    expect(calls.errors.at(-1)).toBe(
      "This session's previous agent process couldn't be stopped after the backend restarted. Restart the backend to clear this, then try again.",
    );
  });
});
