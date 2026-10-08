import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { WebSocketRequestError } from "@/lib/ws/client";
import { useSessionRecoveryActions } from "./use-session-recovery-actions";

const mocks = vi.hoisted(() => ({
  requestSessionRecover: vi.fn(),
  restoreSessionWorkspace: vi.fn(),
  getWorkspaceRecoveryStatus: vi.fn(),
}));

vi.mock("@/lib/services/session-recovery-service", () => ({
  asRecoveryError: (error: unknown, fallback: string) =>
    error instanceof Error ? error : new Error(fallback),
  branchRecoveryDetails: () => null,
  managedCloneRelocationRecoveryDetails: () => null,
  recoveryInspectionBusyDetails: (error: unknown) => {
    if (
      !(error instanceof WebSocketRequestError) ||
      error.details?.kind !== "recovery_inspection_busy"
    )
      return null;
    return { kind: "recovery_inspection_busy" };
  },
  recoveryInspectionBusyMessage: () =>
    "This workspace is still being checked. Try resuming the session again.",
  sessionRecoveryGuardDetails: () => null,
  sessionRecoveryGuardMessage: () => "",
  requestSessionRecover: mocks.requestSessionRecover,
  restoreSessionWorkspace: mocks.restoreSessionWorkspace,
  getWorkspaceRecoveryStatus: mocks.getWorkspaceRecoveryStatus,
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: never) => unknown) =>
    selector({
      taskSessions: { items: {} },
      kanban: { tasks: [] },
      quickChat: { sessions: [] },
      agentProfiles: { items: [] },
      repositories: { itemsByWorkspaceId: {} },
      setWorkspaceRecoveryProjection: vi.fn(),
    } as never),
}));

const TASK_ID = "task-contention";
const SESSION_ID = "session-contention";
const BUSY_COPY = "This workspace is still being checked. Try resuming the session again.";

describe("manual recovery inspection contention", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("keeps only same-session Resume available and retires the notice after matching success", async () => {
    mocks.requestSessionRecover
      .mockRejectedValueOnce(
        new WebSocketRequestError("workspace recovery inspection is busy", "CONFLICT", {
          kind: "recovery_inspection_busy",
        }),
      )
      .mockResolvedValueOnce(undefined);
    const { result } = renderHook(() =>
      useSessionRecoveryActions({ taskId: TASK_ID, sessionId: SESSION_ID }),
    );

    await act(async () => {
      await result.current.handleRecover("resume");
    });

    expect(result.current.recoveryNotice).toBe(BUSY_COPY);
    expect(result.current.recoveryNoticeKind).toBe("inspection_busy");
    expect(result.current.recoveryError).toBeNull();
    expect(result.current.manualRecoveryFailure).toBeNull();
    expect(result.current.lastFailedAction).toBe("resume");
    expect(mocks.restoreSessionWorkspace).not.toHaveBeenCalled();

    await act(async () => {
      await result.current.handleRetry();
    });

    expect(mocks.requestSessionRecover).toHaveBeenNthCalledWith(2, {
      taskId: TASK_ID,
      sessionId: SESSION_ID,
      action: "resume",
      failureMessage: "task:failedToResumeSession",
    });
    expect(result.current.recoveryNotice).toBeNull();
    expect(result.current.recoveryNoticeKind).toBeNull();
    expect(result.current.manualRecoveryFailure).toBeNull();
  });
});
