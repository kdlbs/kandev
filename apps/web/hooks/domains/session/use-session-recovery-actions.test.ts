import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useSessionRecoveryActions } from "./use-session-recovery-actions";
import type { WorkspaceRecoveryProjection } from "@/lib/types/http";

type RecoveryEligibilityState = {
  taskSessions: {
    items: Record<
      string,
      {
        task_id: string;
        state: string;
        agent_profile_id?: string;
        execution_profile_id?: string;
        downstream_acp_session_id?: string;
        is_passthrough?: boolean;
        metadata?: Record<string, unknown> | null;
        task_environment_id?: string;
        workspace_recovery?: WorkspaceRecoveryProjection | null;
        session_recovery_blocks?: Array<{
          id: string;
          incarnation_id: string;
          expected_generation: number;
          reason: string;
          consumer_reference: string;
        }>;
      }
    >;
  };
  kanban: { tasks: { id: string; isFromOffice?: boolean }[] };
  quickChat: {
    sessions: { kind: "chat" | "config"; sessionId: string; taskId?: string }[];
  };
  agentProfiles: {
    items: {
      id: string;
      agent_id: string;
      agent_name: string;
      cli_passthrough: boolean;
    }[];
  };
  setWorkspaceRecoveryProjection: ReturnType<typeof vi.fn>;
};

const mocks = vi.hoisted(() => ({
  requestSessionRecover: vi.fn(),
  restoreSessionWorkspace: vi.fn(),
  getWorkspaceRecoveryStatus: vi.fn(),
  setWorkspaceRecoveryProjection: vi.fn(),
  managedCloneRelocationRecoveryDetails: vi.fn().mockReturnValue(null),
  appState: null as unknown as RecoveryEligibilityState,
}));

vi.mock("@/lib/services/session-recovery-service", () => ({
  asRecoveryError: (error: unknown, fallback: string) =>
    error instanceof Error ? error : new Error(fallback),
  branchRecoveryDetails: () => null,
  managedCloneRelocationRecoveryDetails: mocks.managedCloneRelocationRecoveryDetails,
  recoveryInspectionBusyDetails: () => null,
  recoveryInspectionBusyMessage: () => "",
  sessionRecoveryGuardDetails: () => null,
  sessionRecoveryGuardMessage: () => "",
  contextContinuationDetails: () => null,
  sessionDeliveryRecoveryMessage: (result: { outcome: string }) =>
    `task:deliveryRecovery${result.outcome}`,
  sessionDeliveryRecoveryReasonMessage: (reason: string) => `task:deliveryRecovery:${reason}`,
  requestSessionRecover: mocks.requestSessionRecover,
  restoreSessionWorkspace: mocks.restoreSessionWorkspace,
  getWorkspaceRecoveryStatus: mocks.getWorkspaceRecoveryStatus,
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, values?: Record<string, string>) =>
      values ? `${key}:${JSON.stringify(values)}` : key,
  }),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: never) => unknown) => selector(mocks.appState as never),
}));

const TASK_ID = "task-1";
const SESSION_ID = "session-1";
const FAILED_TO_RESUME_MESSAGE_KEY = "task:failedToResumeSession";
const LEGACY_ERROR_STAMP = "legacy-stamp";
const PROVIDER_UNAVAILABLE = "provider unavailable";
const MANAGED_CLONE_RECOVERY_STAMP = "managed-stamp-2";
const MANAGED_CLONE_RELOCATION_ERROR = "workspace needs relocation";

beforeEach(() => {
  vi.clearAllMocks();
  mocks.appState = emptyRecoveryEligibilityState();
});

function emptyRecoveryEligibilityState(): RecoveryEligibilityState {
  return {
    taskSessions: { items: {} },
    kanban: { tasks: [] },
    quickChat: { sessions: [] },
    agentProfiles: { items: [] },
    setWorkspaceRecoveryProjection: mocks.setWorkspaceRecoveryProjection,
  };
}

function eligibleRecoveryState(): RecoveryEligibilityState {
  return {
    taskSessions: {
      items: {
        [SESSION_ID]: {
          task_id: TASK_ID,
          state: "FAILED",
          execution_profile_id: "profile-1",
          agent_profile_id: "profile-other",
          downstream_acp_session_id: "native-session-1",
          task_environment_id: "environment-1",
        },
      },
    },
    kanban: { tasks: [{ id: TASK_ID, isFromOffice: false }] },
    quickChat: { sessions: [] },
    agentProfiles: {
      items: [
        {
          id: "profile-1",
          agent_id: "agent-uuid-auggie",
          agent_name: "auggie",
          cli_passthrough: false,
        },
        {
          id: "profile-other",
          agent_id: "agent-uuid-claude",
          agent_name: "claude-acp",
          cli_passthrough: false,
        },
      ],
    },
    setWorkspaceRecoveryProjection: mocks.setWorkspaceRecoveryProjection,
  };
}

// eslint-disable-next-line max-lines-per-function -- recovery retry scenarios share one hook harness.
describe("useSessionRecoveryActions", () => {
  it("fences a delayed block retry result after the committed recovery identity replaces it", async () => {
    const state = eligibleRecoveryState();
    state.taskSessions.items[SESSION_ID].session_recovery_blocks = [
      {
        id: "block-1",
        incarnation_id: "incarnation-1",
        expected_generation: 7,
        reason: "unresolved_durable_work",
        consumer_reference: "agent_delivery",
      },
    ];
    mocks.appState = state;
    let finishRequest!: (value: unknown) => void;
    mocks.requestSessionRecover.mockReturnValue(
      new Promise((resolve) => {
        finishRequest = resolve;
      }),
    );
    const { result, rerender } = renderHook(() =>
      useSessionRecoveryActions({ taskId: TASK_ID, sessionId: SESSION_ID }),
    );
    let pending!: Promise<boolean>;
    await act(async () => {
      pending = result.current.handleRecover("retry_connection");
      await Promise.resolve();
    });

    mocks.appState.taskSessions.items[SESSION_ID].metadata = {
      agent_delivery_recovery: {
        phase: "uncertain",
        revision: 1,
        session_id: SESSION_ID,
        agent_execution_id: "",
        submission_id: "prompt:message-1",
        stream_id: "stream-1",
        incarnation_id: "incarnation-1",
        harness_generation: 7,
        prompt_generation: 0,
        reconstruction: { process_identity_known: false },
      },
    };
    mocks.appState.taskSessions.items[SESSION_ID].session_recovery_blocks = [];
    rerender();
    await act(async () => {
      finishRequest({
        task_id: TASK_ID,
        session_id: SESSION_ID,
        outcome: "blocked",
        reason: "missing_canonical_submission",
        recovery_revision: 0,
      });
      await pending;
    });

    expect(result.current.deliveryRecoveryResult).toBeNull();
    expect(result.current.deliveryRecoveryNotice).toBe(
      "task:deliveryRecovery:recovery_identity_incomplete",
    );
  });

  it("keeps the typed delivery retry outcome as localized status", async () => {
    const onDeliveryReconciled = vi.fn();
    mocks.requestSessionRecover.mockResolvedValueOnce({
      task_id: TASK_ID,
      session_id: SESSION_ID,
      outcome: "blocked",
      reason: "missing_canonical_submission",
      recovery_revision: 0,
    });
    const { result } = renderHook(() =>
      useSessionRecoveryActions({ taskId: TASK_ID, sessionId: SESSION_ID, onDeliveryReconciled }),
    );

    await act(async () => {
      await result.current.handleRecover("retry_connection");
    });

    expect(result.current.deliveryRecoveryNotice).toBe("task:deliveryRecoveryblocked");
    expect(result.current.deliveryRecoveryResult).toMatchObject({
      outcome: "blocked",
      reason: "missing_canonical_submission",
    });
    expect(result.current.recoveryError).toBeNull();
    expect(result.current.busyAction).toBeNull();
    expect(onDeliveryReconciled).toHaveBeenCalledTimes(1);
  });

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
    expect(mocks.requestSessionRecover).toHaveBeenCalledWith({
      taskId: TASK_ID,
      sessionId: SESSION_ID,
      action: "resume",
      failureMessage: FAILED_TO_RESUME_MESSAGE_KEY,
    });
  });

  it("retains a failed action for retry and releases its busy state", async () => {
    mocks.requestSessionRecover
      .mockRejectedValueOnce(new Error(PROVIDER_UNAVAILABLE))
      .mockResolvedValueOnce(undefined);
    const { result } = renderHook(() =>
      useSessionRecoveryActions({ taskId: TASK_ID, sessionId: SESSION_ID }),
    );

    await act(async () => {
      await result.current.handleRecover("resume");
    });

    expect(result.current.recoveryError?.message).toBe(PROVIDER_UNAVAILABLE);
    expect(result.current.manualRecoveryFailure).toMatchObject({
      operation: "resume",
      sessionId: SESSION_ID,
      errorStamp: null,
      requestKey: `${TASK_ID}\u0000${SESSION_ID}\u0000`,
      operationId: 1,
    });
    expect(result.current.busyAction).toBeNull();

    await act(async () => {
      await result.current.handleRetry();
    });

    expect(mocks.requestSessionRecover).toHaveBeenNthCalledWith(2, {
      taskId: TASK_ID,
      sessionId: SESSION_ID,
      action: "resume",
      failureMessage: FAILED_TO_RESUME_MESSAGE_KEY,
    });
    expect(result.current.recoveryError).toBeNull();
    expect(result.current.manualRecoveryFailure).toBeNull();
    expect(result.current.busyAction).toBeNull();
  });

  it("keeps restore failure state safe across repeated retries", async () => {
    const rawError = `backend secret ${"x".repeat(600)}`;
    mocks.restoreSessionWorkspace
      .mockRejectedValueOnce(new Error(rawError))
      .mockRejectedValueOnce(new Error(rawError));
    const { result } = renderHook(() =>
      useSessionRecoveryActions({ taskId: TASK_ID, sessionId: SESSION_ID }),
    );

    await act(async () => {
      await result.current.handleRestore();
    });
    expect(result.current.manualRecoveryFailure).toMatchObject({
      operation: "restore_workspace",
      sessionId: SESSION_ID,
      errorStamp: null,
      requestKey: `${TASK_ID}\u0000${SESSION_ID}\u0000`,
      operationId: 1,
    });
    expect(result.current.recoveryError?.message).toBe(rawError);
    expect(result.current.busyAction).toBeNull();

    await act(async () => {
      await result.current.handleRestore();
    });
    expect(mocks.restoreSessionWorkspace).toHaveBeenCalledTimes(2);
    expect(result.current.manualRecoveryFailure).toMatchObject({
      operation: "restore_workspace",
      sessionId: SESSION_ID,
      errorStamp: null,
      requestKey: `${TASK_ID}\u0000${SESSION_ID}\u0000`,
      operationId: 2,
    });
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

  it("clears recovery state when a new durable error stamp arrives", async () => {
    mocks.requestSessionRecover.mockRejectedValueOnce(new Error(PROVIDER_UNAVAILABLE));
    const { result, rerender } = renderHook(
      ({ errorStamp }: { errorStamp: string }) =>
        useSessionRecoveryActions({ taskId: TASK_ID, sessionId: SESSION_ID, errorStamp }),
      { initialProps: { errorStamp: "bootstrap-1" } },
    );

    await act(async () => {
      await result.current.handleRecover("resume");
    });
    expect(result.current.recoveryError?.message).toBe(PROVIDER_UNAVAILABLE);

    rerender({ errorStamp: "bootstrap-2" });
    expect(result.current.recoveryError).toBeNull();
    expect(result.current.manualRecoveryFailure).toBeNull();
    await waitFor(() => expect(result.current.recoveryError).toBeNull());
  });

  it("uses restore relocation details without another resume", async () => {
    mocks.restoreSessionWorkspace.mockRejectedValueOnce(new Error(MANAGED_CLONE_RELOCATION_ERROR));
    mocks.managedCloneRelocationRecoveryDetails.mockReturnValueOnce({
      kind: "managed_clone_relocation_required",
      error_stamp: MANAGED_CLONE_RECOVERY_STAMP,
      recovery_action: "relocate_and_resume",
    });
    const { result } = renderHook(() =>
      useSessionRecoveryActions({
        taskId: TASK_ID,
        sessionId: SESSION_ID,
        errorStamp: LEGACY_ERROR_STAMP,
      }),
    );

    await act(async () => {
      await result.current.handleRestore();
    });

    expect(result.current.managedCloneRecoveryStamp).toBe(MANAGED_CLONE_RECOVERY_STAMP);
    expect(result.current.manualRecoveryFailure).toMatchObject({ operation: "restore_workspace" });
    expect(mocks.requestSessionRecover).not.toHaveBeenCalled();
    await act(async () => {
      await result.current.handleManagedCloneRelocation();
    });
    expect(mocks.requestSessionRecover).toHaveBeenCalledWith({
      taskId: TASK_ID,
      sessionId: SESSION_ID,
      action: "relocate_and_resume",
      failureMessage: FAILED_TO_RESUME_MESSAGE_KEY,
      errorStamp: MANAGED_CLONE_RECOVERY_STAMP,
    });
  });

  it("ignores a late restore relocation response after the session changes", async () => {
    const pending = Promise.withResolvers<void>();
    mocks.restoreSessionWorkspace.mockReturnValueOnce(pending.promise);
    mocks.managedCloneRelocationRecoveryDetails.mockReturnValueOnce({
      kind: "managed_clone_relocation_required",
      error_stamp: LEGACY_ERROR_STAMP,
      recovery_action: "relocate_and_resume",
    });
    const { result, rerender } = renderHook(
      ({ sessionId }: { sessionId: string }) =>
        useSessionRecoveryActions({ taskId: TASK_ID, sessionId, errorStamp: LEGACY_ERROR_STAMP }),
      { initialProps: { sessionId: SESSION_ID } },
    );

    let restore!: Promise<void>;
    act(() => {
      restore = result.current.handleRestore();
    });
    rerender({ sessionId: "successor-session" });
    await act(async () => {
      pending.reject(new Error(MANAGED_CLONE_RELOCATION_ERROR));
      await restore;
    });

    expect(result.current.managedCloneRecoveryStamp).toBeNull();
    expect(result.current.manualRecoveryFailure).toBeNull();
  });

  it("keeps a matching restore relocation response after the server advances the error stamp", async () => {
    const deferred = Promise.withResolvers<void>();
    mocks.restoreSessionWorkspace.mockReturnValueOnce(deferred.promise);
    mocks.managedCloneRelocationRecoveryDetails.mockReturnValueOnce({
      kind: "managed_clone_relocation_required",
      error_stamp: MANAGED_CLONE_RECOVERY_STAMP,
      recovery_action: "relocate_and_resume",
    });
    const { result, rerender } = renderHook(
      ({ errorStamp }: { errorStamp: string }) =>
        useSessionRecoveryActions({ taskId: TASK_ID, sessionId: SESSION_ID, errorStamp }),
      { initialProps: { errorStamp: "legacy-stamp" } },
    );

    let restore!: Promise<void>;
    act(() => {
      restore = result.current.handleRestore();
    });
    rerender({ errorStamp: MANAGED_CLONE_RECOVERY_STAMP });

    await act(async () => {
      deferred.reject(new Error(MANAGED_CLONE_RELOCATION_ERROR));
      await restore;
    });

    expect(result.current.managedCloneRecoveryStamp).toBe(MANAGED_CLONE_RECOVERY_STAMP);
    expect(result.current.manualRecoveryFailure).toMatchObject({ operation: "restore_workspace" });
    expect(result.current.recoveryError?.message).toBe(MANAGED_CLONE_RELOCATION_ERROR);
  });

  it("surfaces a matching relocation requirement after the server advances the error stamp", async () => {
    const deferred = Promise.withResolvers<void>();
    mocks.requestSessionRecover.mockReturnValueOnce(deferred.promise);
    mocks.managedCloneRelocationRecoveryDetails.mockReturnValueOnce({
      kind: "managed_clone_relocation_required",
      error_stamp: MANAGED_CLONE_RECOVERY_STAMP,
    });
    const { result, rerender } = renderHook(
      ({ errorStamp }: { errorStamp: string }) =>
        useSessionRecoveryActions({ taskId: TASK_ID, sessionId: SESSION_ID, errorStamp }),
      { initialProps: { errorStamp: "old-stamp" } },
    );

    let attempt!: Promise<boolean>;
    act(() => {
      attempt = result.current.handleRecover("resume");
    });
    rerender({ errorStamp: MANAGED_CLONE_RECOVERY_STAMP });

    await act(async () => {
      deferred.reject(new Error(MANAGED_CLONE_RELOCATION_ERROR));
      await attempt;
    });

    expect(result.current.managedCloneRecoveryStamp).toBe(MANAGED_CLONE_RECOVERY_STAMP);
    expect(result.current.recoveryError?.message).toBe(MANAGED_CLONE_RELOCATION_ERROR);
  });

  it("uses the response stamp for the confirmed relocation action", async () => {
    mocks.requestSessionRecover
      .mockRejectedValueOnce(new Error(MANAGED_CLONE_RELOCATION_ERROR))
      .mockResolvedValueOnce(undefined);
    mocks.managedCloneRelocationRecoveryDetails.mockReturnValueOnce({
      kind: "managed_clone_relocation_required",
      error_stamp: MANAGED_CLONE_RECOVERY_STAMP,
    });
    const { result } = renderHook(() =>
      useSessionRecoveryActions({
        taskId: TASK_ID,
        sessionId: SESSION_ID,
        errorStamp: "old-stamp",
      }),
    );

    await act(async () => {
      await result.current.handleRecover("resume");
    });
    expect(result.current.managedCloneRecoveryStamp).toBe(MANAGED_CLONE_RECOVERY_STAMP);
    await act(async () => {
      await result.current.handleManagedCloneRelocation();
    });
    expect(mocks.requestSessionRecover).toHaveBeenLastCalledWith({
      taskId: TASK_ID,
      sessionId: SESSION_ID,
      action: "relocate_and_resume",
      failureMessage: FAILED_TO_RESUME_MESSAGE_KEY,
      errorStamp: MANAGED_CLONE_RECOVERY_STAMP,
    });
  });

  it("clears the authorization stamp when the server reports it is stale", async () => {
    mocks.requestSessionRecover
      .mockRejectedValueOnce(new Error(MANAGED_CLONE_RELOCATION_ERROR))
      .mockRejectedValueOnce(new Error("relocation authorization is stale"));
    mocks.managedCloneRelocationRecoveryDetails
      .mockReturnValueOnce({
        kind: "managed_clone_relocation_required",
        error_stamp: MANAGED_CLONE_RECOVERY_STAMP,
      })
      .mockReturnValueOnce({ kind: "managed_clone_relocation_stale" });
    const { result } = renderHook(() =>
      useSessionRecoveryActions({
        taskId: TASK_ID,
        sessionId: SESSION_ID,
        errorStamp: "old-stamp",
      }),
    );

    await act(async () => {
      await result.current.handleRecover("resume");
    });
    expect(result.current.managedCloneRecoveryStamp).toBe(MANAGED_CLONE_RECOVERY_STAMP);

    await act(async () => {
      await result.current.handleManagedCloneRelocation();
    });
    expect(result.current.managedCloneRecoveryStamp).toBeNull();
  });
});

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.16
it("admits one operation across repeated taps and mounted consumers", async () => {
  const pending = Promise.withResolvers<void>();
  mocks.requestSessionRecover.mockReturnValue(pending.promise);
  const first = renderHook(() =>
    useSessionRecoveryActions({ taskId: TASK_ID, sessionId: SESSION_ID }),
  );
  const second = renderHook(() =>
    useSessionRecoveryActions({ taskId: TASK_ID, sessionId: SESSION_ID }),
  );
  let requests: Promise<unknown>[] = [];
  act(() => {
    requests = [
      first.result.current.handleRecover("resume"),
      first.result.current.handleRecover("resume"),
      second.result.current.handleRecover("fresh_start"),
    ];
  });
  const calls = mocks.requestSessionRecover.mock.calls.length;
  await act(async () => {
    pending.resolve();
    await Promise.all(requests);
  });
  expect(calls).toBe(1);
});
