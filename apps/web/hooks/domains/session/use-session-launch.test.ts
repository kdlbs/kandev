import { act, renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { useSessionLaunch } from "./use-session-launch";
import { WebSocketRequestError } from "@/lib/ws/client";

const mockLaunchSession = vi.fn();

vi.mock("@/lib/services/session-launch-service", () => ({
  launchSession: (...args: unknown[]) => mockLaunchSession(...args),
}));

describe("useSessionLaunch", () => {
  it("surfaces the recovery guard's retryable message instead of the raw backend error", async () => {
    mockLaunchSession.mockRejectedValueOnce(
      new WebSocketRequestError("blocked", "CONFLICT", {
        kind: "session_recovery_in_progress",
        retryable: true,
        session_id: "s1",
      }),
    );
    const { result } = renderHook(() => useSessionLaunch());

    await act(async () => {
      await result.current.launch({ task_id: "t1" });
    });

    expect(result.current.error).toBe(
      "This session's agent is still being reconciled after the backend restarted. This clears automatically; try again in a few seconds.",
    );
  });

  it("surfaces the recovery guard's non-retryable message", async () => {
    mockLaunchSession.mockRejectedValueOnce(
      new WebSocketRequestError("blocked", "UNAVAILABLE", {
        kind: "session_recovery_unstoppable",
        retryable: false,
        session_id: "s1",
      }),
    );
    const { result } = renderHook(() => useSessionLaunch());

    await act(async () => {
      await result.current.launch({ task_id: "t1" });
    });

    expect(result.current.error).toBe(
      "This session's previous agent process couldn't be stopped after the backend restarted. Restart the backend to clear this, then try again.",
    );
  });

  it("falls back to the raw error message for an unrelated failure", async () => {
    mockLaunchSession.mockRejectedValueOnce(new Error("boom"));
    const { result } = renderHook(() => useSessionLaunch());

    await act(async () => {
      await result.current.launch({ task_id: "t1" });
    });

    expect(result.current.error).toBe("boom");
  });
});
