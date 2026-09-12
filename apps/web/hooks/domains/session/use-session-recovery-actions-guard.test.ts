import { act, renderHook, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { useSessionRecoveryActions } from "./use-session-recovery-actions";
import { WebSocketRequestError } from "@/lib/ws/client";

const mockRequestSessionRecover = vi.fn();
const mockRestoreSessionWorkspace = vi.fn();

vi.mock("@/lib/services/session-recovery-service", async () => {
  const actual = await vi.importActual<typeof import("@/lib/services/session-recovery-service")>(
    "@/lib/services/session-recovery-service",
  );
  return {
    ...actual,
    requestSessionRecover: (...args: unknown[]) => mockRequestSessionRecover(...args),
    restoreSessionWorkspace: (...args: unknown[]) => mockRestoreSessionWorkspace(...args),
  };
});

const TASK_ID = "t1";
const SESSION_ID = "s1";

describe("useSessionRecoveryActions", () => {
  it("shows the retryable guard message and clears branch details when resume is guard-refused", async () => {
    mockRequestSessionRecover.mockRejectedValueOnce(
      new WebSocketRequestError("blocked", "CONFLICT", {
        kind: "session_recovery_in_progress",
        retryable: true,
        session_id: SESSION_ID,
      }),
    );
    const { result } = renderHook(() =>
      useSessionRecoveryActions({ taskId: TASK_ID, sessionId: SESSION_ID }),
    );

    await act(async () => {
      await result.current.handleRecover("resume");
    });

    expect(result.current.guardDetails?.kind).toBe("session_recovery_in_progress");
    expect(result.current.branchDetails).toBeNull();
    expect(result.current.recoveryError?.message).toBe(
      "This session's agent is still being reconciled after the backend restarted. This clears automatically; try again in a few seconds.",
    );
  });

  it("shows the non-retryable guard message when the previous agent could not be stopped", async () => {
    mockRequestSessionRecover.mockRejectedValueOnce(
      new WebSocketRequestError("blocked", "UNAVAILABLE", {
        kind: "session_recovery_unstoppable",
        retryable: false,
        session_id: SESSION_ID,
      }),
    );
    const { result } = renderHook(() =>
      useSessionRecoveryActions({ taskId: TASK_ID, sessionId: SESSION_ID }),
    );

    await act(async () => {
      await result.current.handleRecover("fresh_start");
    });

    expect(result.current.guardDetails?.kind).toBe("session_recovery_unstoppable");
    expect(result.current.recoveryError?.message).toBe(
      "This session's previous agent process couldn't be stopped after the backend restarted. Restart the backend to clear this, then try again.",
    );
  });

  it("clears guard details after a subsequent recover call succeeds", async () => {
    mockRequestSessionRecover
      .mockRejectedValueOnce(
        new WebSocketRequestError("blocked", "CONFLICT", {
          kind: "session_recovery_in_progress",
          retryable: true,
          session_id: SESSION_ID,
        }),
      )
      .mockResolvedValueOnce(undefined);
    const { result } = renderHook(() =>
      useSessionRecoveryActions({ taskId: TASK_ID, sessionId: SESSION_ID }),
    );

    await act(async () => {
      await result.current.handleRecover("resume");
    });
    expect(result.current.guardDetails).not.toBeNull();

    await act(async () => {
      await result.current.handleRecover("resume");
    });

    expect(result.current.guardDetails).toBeNull();
    expect(result.current.recoveryError).toBeNull();
  });

  it("clears guard details when the selected session changes", async () => {
    mockRequestSessionRecover.mockRejectedValueOnce(
      new WebSocketRequestError("blocked", "CONFLICT", {
        kind: "session_recovery_in_progress",
        retryable: true,
        session_id: SESSION_ID,
      }),
    );
    const { result, rerender } = renderHook(
      ({ sessionId }: { sessionId: string }) =>
        useSessionRecoveryActions({ taskId: TASK_ID, sessionId }),
      { initialProps: { sessionId: SESSION_ID } },
    );

    await act(async () => {
      await result.current.handleRecover("resume");
    });
    expect(result.current.guardDetails).not.toBeNull();

    rerender({ sessionId: "s2" });

    await waitFor(() => expect(result.current.guardDetails).toBeNull());
    expect(result.current.recoveryError).toBeNull();
  });
});
