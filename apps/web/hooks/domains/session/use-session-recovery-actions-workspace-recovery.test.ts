import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useSessionRecoveryActions } from "./use-session-recovery-actions";
import type { WorkspaceRecoveryProjection } from "@/lib/types/http";

type RecoveryState = {
  taskSessions: {
    items: Record<
      string,
      {
        task_id: string;
        task_environment_id?: string;
        workspace_recovery?: WorkspaceRecoveryProjection | null;
      }
    >;
  };
  kanban: { tasks: { id: string; isFromOffice?: boolean }[] };
  quickChat: { sessions: { kind: "chat" | "config"; sessionId: string; taskId?: string }[] };
  agentProfiles: { items: { id: string; agent_id: string; agent_name: string }[] };
  setWorkspaceRecoveryProjection: ReturnType<typeof vi.fn>;
};

const mocks = vi.hoisted(() => ({
  requestSessionRecover: vi.fn(),
  getWorkspaceRecoveryStatus: vi.fn(),
  setWorkspaceRecoveryProjection: vi.fn(),
  managedCloneRelocationRecoveryDetails: vi.fn().mockReturnValue(null),
  appState: null as unknown as RecoveryState,
}));

vi.mock("@/lib/services/session-recovery-service", () => ({
  asRecoveryError: (error: unknown, fallback: string) =>
    error instanceof Error ? error : new Error(fallback),
  branchRecoveryDetails: () => null,
  managedCloneRelocationRecoveryDetails: mocks.managedCloneRelocationRecoveryDetails,
  sessionRecoveryGuardDetails: () => null,
  sessionRecoveryGuardMessage: () => "",
  requestSessionRecover: mocks.requestSessionRecover,
  restoreSessionWorkspace: vi.fn(),
  getWorkspaceRecoveryStatus: mocks.getWorkspaceRecoveryStatus,
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: never) => unknown) => selector(mocks.appState as never),
}));

const TASK_ID = "task-1";
const SESSION_ID = "session-1";
const ENVIRONMENT_ID = "environment-1";
const RECOVERY_STAMP = "managed-stamp-2";
const OLD_RELOCATION_STAMP = "old-relocation-stamp";

function emptyRecoveryState(): RecoveryState {
  return {
    taskSessions: { items: {} },
    kanban: { tasks: [] },
    quickChat: { sessions: [] },
    agentProfiles: { items: [] },
    setWorkspaceRecoveryProjection: mocks.setWorkspaceRecoveryProjection,
  };
}

function selectRecoveryEnvironment(sessionId = SESSION_ID) {
  mocks.appState = {
    ...emptyRecoveryState(),
    taskSessions: {
      items: { [sessionId]: { task_id: TASK_ID, task_environment_id: ENVIRONMENT_ID } },
    },
  };
}

function workspaceRecoveryProjection(
  overrides: Partial<WorkspaceRecoveryProjection> = {},
): WorkspaceRecoveryProjection {
  return {
    task_id: TASK_ID,
    environment_id: ENVIRONMENT_ID,
    session_id: SESSION_ID,
    operation_id: "operation-1",
    attempt_id: "attempt-1",
    ownership_generation: "generation-1",
    revision: "2",
    kind: "managed_clone_relocation",
    error_stamp: RECOVERY_STAMP,
    state: "running",
    phase: "restoring",
    repository_id: "repository-1",
    repository_position: 1,
    repository_total: 2,
    completed_slots: 0,
    workspace_complete: false,
    agent_ready: false,
    runner_live: true,
    started_at: "2026-10-05T12:00:00Z",
    updated_at: "2026-10-05T12:01:00Z",
    ...overrides,
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  mocks.appState = emptyRecoveryState();
});

describe("workspace recovery status reconciliation", () => {
  it("reconciles a lost relocation response with durable runner progress", async () => {
    selectRecoveryEnvironment();
    const projection = workspaceRecoveryProjection();
    mocks.requestSessionRecover.mockRejectedValueOnce(new Error("connection closed"));
    mocks.getWorkspaceRecoveryStatus.mockResolvedValueOnce(projection);
    const { result } = renderHook(() =>
      useSessionRecoveryActions({
        taskId: TASK_ID,
        sessionId: SESSION_ID,
        errorStamp: RECOVERY_STAMP,
      }),
    );

    let success: boolean | undefined;
    await act(async () => {
      success = await result.current.handleManagedCloneRelocation();
    });

    expect(success).toBe(false);
    expect(mocks.getWorkspaceRecoveryStatus).toHaveBeenCalledWith(
      TASK_ID,
      SESSION_ID,
      "task:workspaceRecoveryStatusUnavailable",
    );
    expect(mocks.setWorkspaceRecoveryProjection).toHaveBeenCalledWith([SESSION_ID], projection);
    expect(result.current.recoveryError).toBeNull();
    expect(result.current.workspaceRecoveryStatusCheck).toBe("idle");
  });

  it("keeps repair controls unresolved until an explicit status check succeeds", async () => {
    selectRecoveryEnvironment();
    mocks.requestSessionRecover.mockRejectedValueOnce(new Error("connection closed"));
    mocks.getWorkspaceRecoveryStatus
      .mockRejectedValueOnce(new Error("status unavailable"))
      .mockResolvedValueOnce(workspaceRecoveryProjection());
    const { result } = renderHook(() =>
      useSessionRecoveryActions({
        taskId: TASK_ID,
        sessionId: SESSION_ID,
        errorStamp: RECOVERY_STAMP,
      }),
    );

    await act(async () => {
      await result.current.handleManagedCloneRelocation();
    });
    expect(result.current.workspaceRecoveryStatusCheck).toBe("unresolved");
    expect(result.current.recoveryError).toBeNull();

    await act(async () => {
      await result.current.checkWorkspaceRecoveryStatus();
    });
    expect(mocks.setWorkspaceRecoveryProjection).toHaveBeenCalledWith(
      [SESSION_ID],
      expect.objectContaining({ runner_live: true }),
    );
    expect(result.current.workspaceRecoveryStatusCheck).toBe("idle");
  });

  it("does not clear the unresolved guard for a status projection with another binding", async () => {
    mocks.getWorkspaceRecoveryStatus.mockResolvedValueOnce(
      workspaceRecoveryProjection({ task_id: "another-task" }),
    );
    const { result } = renderHook(() =>
      useSessionRecoveryActions({ taskId: TASK_ID, sessionId: SESSION_ID }),
    );

    await act(async () => {
      await result.current.checkWorkspaceRecoveryStatus();
    });

    expect(result.current.workspaceRecoveryStatusCheck).toBe("unresolved");
    expect(mocks.setWorkspaceRecoveryProjection).not.toHaveBeenCalled();
  });

  it("does not mistake completed file migration for agent readiness", async () => {
    selectRecoveryEnvironment();
    const projection = workspaceRecoveryProjection({
      state: "failed",
      phase: "resuming",
      runner_live: false,
      workspace_complete: true,
      completed_slots: 2,
      reason_code: "resume_not_completed",
    });
    mocks.requestSessionRecover.mockRejectedValueOnce(new Error("resume failed"));
    mocks.getWorkspaceRecoveryStatus.mockResolvedValueOnce(projection);
    const { result } = renderHook(() =>
      useSessionRecoveryActions({
        taskId: TASK_ID,
        sessionId: SESSION_ID,
        errorStamp: RECOVERY_STAMP,
      }),
    );

    await act(async () => {
      await result.current.handleManagedCloneRelocation();
    });

    expect(mocks.setWorkspaceRecoveryProjection).toHaveBeenCalledWith([SESSION_ID], projection);
    expect(result.current.recoveryError).toBeNull();
    expect(projection.workspace_complete).toBe(true);
    expect(projection.agent_ready).toBe(false);
  });
});

describe("workspace recovery environment binding", () => {
  it("accepts an initiating session projection for a sibling in the same task environment", async () => {
    const siblingId = "session-sibling";
    const projection = workspaceRecoveryProjection();
    mocks.appState = {
      ...emptyRecoveryState(),
      taskSessions: {
        items: {
          [SESSION_ID]: { task_id: TASK_ID, task_environment_id: ENVIRONMENT_ID },
          [siblingId]: {
            task_id: TASK_ID,
            task_environment_id: ENVIRONMENT_ID,
            workspace_recovery: projection,
          },
        },
      },
    };
    // The status endpoint is environment-scoped and returns the initiating session's ID.
    mocks.getWorkspaceRecoveryStatus.mockResolvedValueOnce(projection);
    const { result } = renderHook(() =>
      useSessionRecoveryActions({ taskId: TASK_ID, sessionId: siblingId }),
    );

    let status: Awaited<ReturnType<typeof result.current.checkWorkspaceRecoveryStatus>> | undefined;
    await act(async () => {
      status = await result.current.checkWorkspaceRecoveryStatus();
    });

    expect(result.current.workspaceRecovery).toEqual(projection);
    expect(status).toEqual({ resolved: true, projection });
    expect(mocks.setWorkspaceRecoveryProjection).toHaveBeenCalledWith([siblingId], projection);
    expect(result.current.workspaceRecoveryStatusCheck).toBe("idle");
  });

  it("rejects foreign environments and older ownership generations", async () => {
    const siblingId = "session-sibling";
    mocks.appState = {
      ...emptyRecoveryState(),
      taskSessions: {
        items: {
          [siblingId]: {
            task_id: TASK_ID,
            task_environment_id: ENVIRONMENT_ID,
            workspace_recovery: workspaceRecoveryProjection({
              ownership_generation: "8",
              revision: "1",
            }),
          },
        },
      },
    };
    const { result } = renderHook(() =>
      useSessionRecoveryActions({ taskId: TASK_ID, sessionId: siblingId }),
    );

    mocks.getWorkspaceRecoveryStatus.mockResolvedValueOnce(
      workspaceRecoveryProjection({ environment_id: "environment-other" }),
    );
    await act(async () => {
      await result.current.checkWorkspaceRecoveryStatus();
    });
    expect(result.current.workspaceRecoveryStatusCheck).toBe("unresolved");
    expect(mocks.setWorkspaceRecoveryProjection).not.toHaveBeenCalled();

    mocks.getWorkspaceRecoveryStatus.mockResolvedValueOnce(
      workspaceRecoveryProjection({ ownership_generation: "7", revision: "99" }),
    );
    await act(async () => {
      await result.current.checkWorkspaceRecoveryStatus();
    });
    expect(result.current.workspaceRecoveryStatusCheck).toBe("unresolved");
    expect(mocks.setWorkspaceRecoveryProjection).not.toHaveBeenCalled();
  });
});

describe("sibling workspace recovery attempt reconciliation", () => {
  it("accepts a newer environment attempt when reconciling a sibling status read", async () => {
    const siblingId = "session-sibling";
    const previous = workspaceRecoveryProjection({
      operation_id: "operation-previous",
      attempt_id: "attempt-previous",
      revision: "6",
    });
    const latest = workspaceRecoveryProjection({
      operation_id: "operation-latest",
      attempt_id: "attempt-latest",
      revision: "7",
    });
    mocks.appState = {
      ...emptyRecoveryState(),
      taskSessions: {
        items: {
          [siblingId]: {
            task_id: TASK_ID,
            task_environment_id: ENVIRONMENT_ID,
            workspace_recovery: previous,
          },
        },
      },
    };
    mocks.getWorkspaceRecoveryStatus.mockResolvedValueOnce(latest);
    const { result } = renderHook(() =>
      useSessionRecoveryActions({ taskId: TASK_ID, sessionId: siblingId }),
    );

    await act(async () => {
      await result.current.checkWorkspaceRecoveryStatus();
    });

    expect(result.current.workspaceRecoveryStatusCheck).toBe("idle");
    expect(mocks.setWorkspaceRecoveryProjection).toHaveBeenCalledWith([siblingId], latest);
  });
});

describe("workspace recovery environment selection", () => {
  it("does not accept status without a selected environment binding", async () => {
    const siblingId = "session-without-environment";
    mocks.appState = {
      ...emptyRecoveryState(),
      taskSessions: {
        items: { [siblingId]: { task_id: TASK_ID } },
      },
    };
    mocks.getWorkspaceRecoveryStatus.mockResolvedValueOnce(workspaceRecoveryProjection());
    const { result } = renderHook(() =>
      useSessionRecoveryActions({ taskId: TASK_ID, sessionId: siblingId }),
    );

    await act(async () => {
      await result.current.checkWorkspaceRecoveryStatus();
    });

    expect(result.current.workspaceRecoveryStatusCheck).toBe("unresolved");
    expect(mocks.setWorkspaceRecoveryProjection).not.toHaveBeenCalled();
  });
});

describe("workspace recovery error correlation", () => {
  it("matches only the current unfinished relocation error", () => {
    const projection = workspaceRecoveryProjection({ error_stamp: OLD_RELOCATION_STAMP });
    mocks.appState = {
      ...emptyRecoveryState(),
      taskSessions: {
        items: {
          [SESSION_ID]: {
            task_id: TASK_ID,
            task_environment_id: ENVIRONMENT_ID,
            workspace_recovery: projection,
          },
        },
      },
    };
    const { result, rerender } = renderHook(
      ({ stamp }) =>
        useSessionRecoveryActions({ taskId: TASK_ID, sessionId: SESSION_ID, errorStamp: stamp }),
      { initialProps: { stamp: "new-provider-error-stamp" } },
    );

    expect(result.current.workspaceRecoveryMatchesCurrentFailure).toBe(false);

    rerender({ stamp: OLD_RELOCATION_STAMP });
    expect(result.current.workspaceRecoveryMatchesCurrentFailure).toBe(true);

    mocks.appState.taskSessions.items[SESSION_ID].workspace_recovery = workspaceRecoveryProjection({
      error_stamp: OLD_RELOCATION_STAMP,
      state: "complete",
      phase: "complete",
      workspace_complete: true,
      agent_ready: true,
      runner_live: false,
    });
    rerender({ stamp: OLD_RELOCATION_STAMP });
    expect(result.current.workspaceRecoveryMatchesCurrentFailure).toBe(false);
  });
});
