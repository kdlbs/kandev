import { act, renderHook } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";
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

beforeEach(() => {
  vi.clearAllMocks();
  mocks.appState = eligibleRecoveryState();
});

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
it("hides an eligible retry result when persisted recovery advances", async () => {
  mocks.appState = eligibleRecoveryState();
  const recovery = {
    phase: "uncertain",
    revision: 1,
    session_id: SESSION_ID,
    agent_execution_id: "execution",
    submission_id: "submission",
    stream_id: "stream",
    incarnation_id: "incarnation",
    harness_generation: 1,
    prompt_generation: 1,
  };
  mocks.appState.taskSessions.items[SESSION_ID].metadata = { agent_delivery_recovery: recovery };
  mocks.requestSessionRecover.mockResolvedValueOnce({
    task_id: TASK_ID,
    session_id: SESSION_ID,
    outcome: "uncertain",
    recovery_revision: 1,
  });
  const { result, rerender } = renderHook(() =>
    useSessionRecoveryActions({ taskId: TASK_ID, sessionId: SESSION_ID }),
  );
  await act(async () => {
    await result.current.handleRecover("retry_connection");
  });
  expect(result.current.deliveryRecoveryNotice).toBe("task:deliveryRecoveryuncertain");
  mocks.appState.taskSessions.items[SESSION_ID].metadata = {
    agent_delivery_recovery: { ...recovery, revision: 2 },
  };
  rerender();
  expect(result.current.deliveryRecoveryNotice).toBeNull();
});
