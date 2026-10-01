import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useSessionRecoveryActions } from "./use-session-recovery-actions";

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
};

const mocks = vi.hoisted(() => ({
  requestSessionRecover: vi.fn(),
  restoreSessionWorkspace: vi.fn(),
  managedCloneRelocationRecoveryDetails: vi.fn().mockReturnValue(null),
  appState: null as unknown as RecoveryEligibilityState,
}));

vi.mock("@/lib/services/session-recovery-service", () => ({
  asRecoveryError: (error: unknown, fallback: string) =>
    error instanceof Error ? error : new Error(fallback),
  branchRecoveryDetails: () => null,
  managedCloneRelocationRecoveryDetails: mocks.managedCloneRelocationRecoveryDetails,
  sessionRecoveryGuardDetails: () => null,
  sessionRecoveryGuardMessage: () => "",
  requestSessionRecover: mocks.requestSessionRecover,
  restoreSessionWorkspace: mocks.restoreSessionWorkspace,
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
  };
}

describe("provider-restored resume eligibility", () => {
  it("sends the recovery policy for an eligible failed Auggie ACP task session", async () => {
    mocks.appState = eligibleRecoveryState();
    const { result } = renderHook(() =>
      useSessionRecoveryActions({ taskId: TASK_ID, sessionId: SESSION_ID }),
    );

    expect(result.current.providerRestoredResumeEligible).toBe(true);
    await act(async () => {
      await result.current.handleRecover("resume");
    });

    expect(mocks.requestSessionRecover).toHaveBeenCalledWith({
      taskId: TASK_ID,
      sessionId: SESSION_ID,
      action: "resume",
      failureMessage: FAILED_TO_RESUME_MESSAGE_KEY,
      settingsPolicy: "provider_restored",
    });
  });

  it.each([
    [
      "empty execution profile ID falls back to the agent profile",
      (state: RecoveryEligibilityState) => {
        state.taskSessions.items[SESSION_ID].execution_profile_id = "";
        state.taskSessions.items[SESSION_ID].agent_profile_id = "profile-1";
      },
    ],
    [
      "ACP metadata native session ID",
      (state: RecoveryEligibilityState) => {
        delete state.taskSessions.items[SESSION_ID].downstream_acp_session_id;
        state.taskSessions.items[SESSION_ID].metadata = {
          acp: { session_id: "native-session-from-metadata" },
        };
      },
    ],
  ])("supports %s", async (_description, updateState) => {
    const state = eligibleRecoveryState();
    updateState(state);
    mocks.appState = state;
    const { result } = renderHook(() =>
      useSessionRecoveryActions({ taskId: TASK_ID, sessionId: SESSION_ID }),
    );

    expect(result.current.providerRestoredResumeEligible).toBe(true);
  });

  it("supports a Quick Chat task session when the canonical task is not in kanban", async () => {
    const state = eligibleRecoveryState();
    state.kanban.tasks = [];
    state.quickChat.sessions = [{ kind: "chat", sessionId: SESSION_ID, taskId: TASK_ID }];
    mocks.appState = state;
    const { result } = renderHook(() =>
      useSessionRecoveryActions({ taskId: TASK_ID, sessionId: SESSION_ID }),
    );

    expect(result.current.providerRestoredResumeEligible).toBe(true);
    await act(async () => {
      await result.current.handleRecover("resume");
    });
    expect(mocks.requestSessionRecover).toHaveBeenCalledWith({
      taskId: TASK_ID,
      sessionId: SESSION_ID,
      action: "resume",
      failureMessage: FAILED_TO_RESUME_MESSAGE_KEY,
      settingsPolicy: "provider_restored",
    });
  });
});

describe("provider-restored resume eligibility exclusions", () => {
  it.each([
    [
      "Office task",
      (state: RecoveryEligibilityState) => (state.kanban.tasks[0].isFromOffice = true),
    ],
    [
      "nonfailed session",
      (state: RecoveryEligibilityState) => (state.taskSessions.items[SESSION_ID].state = "RUNNING"),
    ],
    [
      "other provider",
      (state: RecoveryEligibilityState) => (state.agentProfiles.items[0].agent_name = "claude-acp"),
    ],
    [
      "passthrough session",
      (state: RecoveryEligibilityState) =>
        (state.taskSessions.items[SESSION_ID].is_passthrough = true),
    ],
    [
      "passthrough profile",
      (state: RecoveryEligibilityState) => (state.agentProfiles.items[0].cli_passthrough = true),
    ],
    [
      "missing native session token",
      (state: RecoveryEligibilityState) =>
        delete state.taskSessions.items[SESSION_ID].downstream_acp_session_id,
    ],
    ["missing task", (state: RecoveryEligibilityState) => (state.kanban.tasks = [])],
    [
      "mismatched Quick Chat ownership",
      (state: RecoveryEligibilityState) => {
        state.kanban.tasks = [];
        state.quickChat.sessions = [{ kind: "chat", sessionId: SESSION_ID, taskId: "task-2" }];
      },
    ],
    [
      "session owned by another task",
      (state: RecoveryEligibilityState) =>
        (state.taskSessions.items[SESSION_ID].task_id = "task-2"),
    ],
    [
      "missing provider profile",
      (state: RecoveryEligibilityState) => (state.agentProfiles.items = []),
    ],
  ])("omits the recovery policy for an ineligible %s", async (_reason, makeIneligible) => {
    const state = eligibleRecoveryState();
    makeIneligible(state);
    mocks.appState = state;
    const { result } = renderHook(() =>
      useSessionRecoveryActions({ taskId: TASK_ID, sessionId: SESSION_ID }),
    );

    expect(result.current.providerRestoredResumeEligible).toBe(false);
    await act(async () => {
      await result.current.handleRecover("resume");
    });

    expect(mocks.requestSessionRecover).toHaveBeenCalledWith({
      taskId: TASK_ID,
      sessionId: SESSION_ID,
      action: "resume",
      failureMessage: FAILED_TO_RESUME_MESSAGE_KEY,
    });
  });

  it("omits the recovery policy for other actions even when resume is eligible", async () => {
    mocks.appState = eligibleRecoveryState();
    const { result } = renderHook(() =>
      useSessionRecoveryActions({ taskId: TASK_ID, sessionId: SESSION_ID }),
    );

    await act(async () => {
      await result.current.handleRecover("fresh_start");
    });

    expect(mocks.requestSessionRecover).toHaveBeenCalledWith({
      taskId: TASK_ID,
      sessionId: SESSION_ID,
      action: "fresh_start",
      failureMessage: FAILED_TO_RESUME_MESSAGE_KEY,
    });
  });
});

// eslint-disable-next-line max-lines-per-function -- recovery retry scenarios share one hook harness.
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
    expect(result.current.manualRecoveryFailure).toEqual({ operation: "resume" });
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
    expect(result.current.manualRecoveryFailure).toEqual({ operation: "restore_workspace" });
    expect(result.current.recoveryError?.message).toBe(rawError);
    expect(result.current.busyAction).toBeNull();

    await act(async () => {
      await result.current.handleRestore();
    });
    expect(mocks.restoreSessionWorkspace).toHaveBeenCalledTimes(2);
    expect(result.current.manualRecoveryFailure).toEqual({ operation: "restore_workspace" });
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
    await waitFor(() => expect(result.current.recoveryError).toBeNull());
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
