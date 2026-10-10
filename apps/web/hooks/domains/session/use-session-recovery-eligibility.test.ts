import { act, renderHook } from "@testing-library/react";
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
