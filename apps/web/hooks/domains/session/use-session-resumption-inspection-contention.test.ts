import { beforeEach, describe, expect, it, vi } from "vitest";
import { WebSocketRequestError } from "@/lib/ws/client";
import { resumeWithSilentFallback } from "./use-session-resumption-operations";
import type { ResumeStateSetter, ResumptionState } from "./use-session-resumption";

const mockRequest = vi.fn();

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ request: mockRequest }),
}));

const TASK_ID = "task-contention";
const SESSION_ID = "session-contention";
const INSPECTION_BUSY = "recovery_inspection_busy";

type LiveSession = {
  state: string;
  started_at: string;
  updated_at: string;
  queue_incarnation_id: string;
  resume_projection_id?: string;
};

function makeSetters(state = "WAITING_FOR_INPUT") {
  let liveSession: LiveSession = {
    state,
    started_at: "2026-10-08T10:00:00Z",
    updated_at: "2026-10-08T10:01:00Z",
    queue_incarnation_id: "incarnation-1",
  };
  const taskSessionStates: string[] = [];
  const notices: (string | null)[] = [];
  const errors: (string | null)[] = [];
  const recoveryFailures: unknown[] = [];
  const resumptionStates: ResumptionState[] = [];
  const setTaskSession: ResumeStateSetter["setTaskSession"] = (session) => {
    taskSessionStates.push(session.state);
    liveSession = { ...liveSession, ...session };
  };
  const setters: ResumeStateSetter = {
    setResumptionState: (state) => resumptionStates.push(state),
    setError: (error) => errors.push(error),
    setNotice: (notice) => notices.push(notice),
    setRecoveryFailure: (failure) => recoveryFailures.push(failure),
    setWorktreePath: () => {},
    setWorktreeBranch: () => {},
    setTaskSession,
    setTaskSessionUnscoped: setTaskSession,
    getLiveSession: () => liveSession,
  };
  return {
    setters,
    getSession: () => liveSession,
    taskSessionStates,
    notices,
    errors,
    recoveryFailures,
    resumptionStates,
  };
}

describe("resume inspection contention", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.spyOn(console, "error").mockImplementation(() => {});
  });

  it("rolls back the optimistic state and offers a same-session retry without restore fallback", async () => {
    mockRequest.mockRejectedValueOnce(
      new WebSocketRequestError("workspace recovery inspection is busy", "CONFLICT", {
        kind: INSPECTION_BUSY,
      }),
    );
    const calls = makeSetters();

    const resumed = await resumeWithSilentFallback(
      TASK_ID,
      SESSION_ID,
      calls.getSession(),
      calls.setters,
    );

    expect(resumed).toBe(false);
    expect(mockRequest).toHaveBeenCalledTimes(1);
    expect(mockRequest.mock.calls[0]?.[0]).toBe("session.launch");
    expect(calls.taskSessionStates).toEqual([]);
    expect(calls.getSession()).toMatchObject({
      state: "WAITING_FOR_INPUT",
      queue_incarnation_id: "incarnation-1",
    });
    expect(calls.resumptionStates.at(-1)).toBe("error");
    expect(calls.notices.at(-1)).toBe(
      "This workspace is still being checked. Try resuming the session again.",
    );
    expect(calls.errors.at(-1)).toBeNull();
    expect(calls.recoveryFailures.every((failure) => failure === null)).toBe(true);
  });

  it("rolls back only its own optimistic STARTING projection", async () => {
    mockRequest.mockRejectedValueOnce(
      new WebSocketRequestError("workspace recovery inspection is busy", "CONFLICT", {
        kind: INSPECTION_BUSY,
      }),
    );
    const calls = makeSetters("IDLE");

    await resumeWithSilentFallback(TASK_ID, SESSION_ID, calls.getSession(), calls.setters);

    expect(calls.taskSessionStates).toEqual(["STARTING", "IDLE"]);
    expect(calls.getSession()).toMatchObject({
      state: "IDLE",
      queue_incarnation_id: "incarnation-1",
    });
    expect(mockRequest).toHaveBeenCalledTimes(1);
  });
});
