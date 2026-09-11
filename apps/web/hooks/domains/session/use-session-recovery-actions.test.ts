import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useSessionRecoveryActions } from "./use-session-recovery-actions";

const mocks = vi.hoisted(() => ({
  requestSessionRecover: vi.fn(),
  restoreSessionWorkspace: vi.fn(),
}));

vi.mock("@/lib/services/session-recovery-service", () => ({
  asRecoveryError: (error: unknown, fallback: string) =>
    error instanceof Error ? error : new Error(fallback),
  branchRecoveryDetails: () => null,
  contextContinuationDetails: () => null,
  requestSessionRecover: mocks.requestSessionRecover,
  restoreSessionWorkspace: mocks.restoreSessionWorkspace,
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, values?: Record<string, string>) =>
      values ? `${key}:${JSON.stringify(values)}` : key,
  }),
}));

const TASK_ID = "task-1";
const SESSION_ID = "session-1";

beforeEach(() => {
  vi.clearAllMocks();
});

describe("useSessionRecoveryActions", () => {
  it("clears busy and error state after a successful recovery", async () => {
    mocks.requestSessionRecover.mockResolvedValueOnce(undefined);
    const { result } = renderHook(() =>
      useSessionRecoveryActions({ taskId: TASK_ID, sessionId: SESSION_ID }),
    );

    let success: boolean | undefined;
    await act(async () => {
      success = await result.current.handleRecover("resume");
    });

    expect(success).toBe(true);
    expect(result.current.busyAction).toBeNull();
    expect(result.current.recoveryError).toBeNull();
    expect(mocks.requestSessionRecover).toHaveBeenCalledWith(
      TASK_ID,
      SESSION_ID,
      "resume",
      "task:failedToResumeSession",
    );
  });

  it("retains a failed action for retry and releases its busy state", async () => {
    mocks.requestSessionRecover
      .mockRejectedValueOnce(new Error("provider unavailable"))
      .mockResolvedValueOnce(undefined);
    const { result } = renderHook(() =>
      useSessionRecoveryActions({ taskId: TASK_ID, sessionId: SESSION_ID }),
    );

    await act(async () => {
      await result.current.handleRecover("resume");
    });

    expect(result.current.recoveryError?.message).toBe("provider unavailable");
    expect(result.current.busyAction).toBeNull();

    await act(async () => {
      await result.current.handleRetry();
    });

    expect(mocks.requestSessionRecover).toHaveBeenNthCalledWith(
      2,
      TASK_ID,
      SESSION_ID,
      "resume",
      "task:failedToResumeSession",
    );
    expect(result.current.recoveryError).toBeNull();
    expect(result.current.busyAction).toBeNull();
  });

  it("ignores an old response after the selected session changes", async () => {
    const deferred = Promise.withResolvers<void>();
    mocks.requestSessionRecover.mockReturnValueOnce(deferred.promise);
    const { result, rerender } = renderHook(
      ({ sessionId }: { sessionId: string }) =>
        useSessionRecoveryActions({ taskId: TASK_ID, sessionId }),
      { initialProps: { sessionId: SESSION_ID } },
    );

    act(() => {
      void result.current.handleRecover("resume");
    });
    expect(result.current.busyAction).toBe("resume");

    rerender({ sessionId: "session-2" });
    await waitFor(() => expect(result.current.busyAction).toBeNull());

    await act(async () => {
      deferred.resolve();
      await deferred.promise;
    });

    expect(result.current.recoveryError).toBeNull();
    expect(result.current.busyAction).toBeNull();
  });
});
