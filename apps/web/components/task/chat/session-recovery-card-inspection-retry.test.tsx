import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import { TaskLaunchErrorProvider } from "../task-launch-error-context";
import { WebSocketRequestError } from "@/lib/ws/client";
import type { AppState } from "@/lib/state/store";
import type { SessionRecoveryOwner } from "@/lib/session-recovery-presentation";
import { useSessionRecoveryActions } from "@/hooks/domains/session/use-session-recovery-actions";
import { SessionRecoveryCard } from "./session-recovery-card";

const { BUSY_COPY, mocks } = vi.hoisted(() => ({
  BUSY_COPY: "This workspace is still being checked. Try resuming the session again.",
  mocks: {
    requestSessionRecover: vi.fn(),
    restoreSessionWorkspace: vi.fn(),
    getWorkspaceRecoveryStatus: vi.fn(),
  },
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
  recoveryInspectionBusyMessage: () => BUSY_COPY,
  sessionRecoveryGuardDetails: () => null,
  sessionRecoveryGuardMessage: () => "",
  requestSessionRecover: mocks.requestSessionRecover,
  restoreSessionWorkspace: mocks.restoreSessionWorkspace,
  getWorkspaceRecoveryStatus: mocks.getWorkspaceRecoveryStatus,
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

afterEach(cleanup);
beforeEach(() => vi.clearAllMocks());

const TASK_ID = "task-inspection-retry";
const SESSION_ID = "session-inspection-retry";
const RESUME_BUTTON = "recovery-resume-button";

function initialState(): Partial<AppState> {
  return {
    taskSessions: {
      items: {
        [SESSION_ID]: {
          id: SESSION_ID,
          task_id: TASK_ID,
          state: "FAILED",
          agent_profile_id: "profile-other",
          execution_profile_id: "profile-auggie",
          downstream_acp_session_id: "native-session",
          task_environment_id: "environment-1",
        },
      },
    } as unknown as AppState["taskSessions"],
    kanban: { tasks: [{ id: TASK_ID, isFromOffice: false }] } as AppState["kanban"],
    quickChat: { sessions: [] } as unknown as AppState["quickChat"],
    agentProfiles: {
      items: [
        {
          id: "profile-auggie",
          agent_id: "agent-auggie",
          agent_name: "auggie",
          cli_passthrough: false,
        },
        {
          id: "profile-other",
          agent_id: "agent-other",
          agent_name: "claude-acp",
          cli_passthrough: false,
        },
      ],
    } as AppState["agentProfiles"],
  };
}

function automaticOwner(
  sessionId: string,
  resumeSession: () => Promise<boolean>,
): SessionRecoveryOwner {
  return {
    requestIdentity: { taskId: TASK_ID, sessionId, generation: 1, attemptId: 1 },
    resumptionState: "idle",
    error: null,
    notice: BUSY_COPY,
    noticeKind: "inspection_busy",
    recoveryFailure: null,
    resumeSession,
  };
}

function RecoveryScreen({
  automaticRecovery,
}: {
  automaticRecovery?: SessionRecoveryOwner | null;
}) {
  const actions = useSessionRecoveryActions({ taskId: TASK_ID, sessionId: SESSION_ID });
  return (
    <TaskLaunchErrorProvider
      value={{ taskId: TASK_ID, workspaceId: "workspace-1", automaticRecovery }}
    >
      <SessionRecoveryCard
        model={{ sessionId: SESSION_ID, kind: "generic" }}
        actions={actions}
        onNewSession={vi.fn()}
      />
    </TaskLaunchErrorProvider>
  );
}

function renderRecoveryScreen(automaticRecovery?: SessionRecoveryOwner | null) {
  return render(
    <StateProvider initialState={initialState()}>
      <RecoveryScreen automaticRecovery={automaticRecovery} />
    </StateProvider>,
  );
}

async function reachManualInspectionNotice() {
  mocks.requestSessionRecover
    .mockRejectedValueOnce(
      new WebSocketRequestError("workspace recovery inspection is busy", "CONFLICT", {
        kind: "recovery_inspection_busy",
      }),
    )
    .mockResolvedValueOnce(undefined);
  fireEvent.click(screen.getByTestId(RESUME_BUTTON));
  await waitFor(() => expect(screen.getByText(BUSY_COPY)).toBeTruthy());
}

describe("SessionRecoveryCard inspection retry routing", () => {
  it.each([
    ["without an automatic owner", null],
    [
      "with a mismatched automatic owner",
      automaticOwner("another-session", vi.fn().mockResolvedValue(true)),
    ],
  ])("keeps manual inspection retry actionable %s", async (_name, owner) => {
    renderRecoveryScreen(owner);
    await reachManualInspectionNotice();

    fireEvent.click(screen.getByTestId(RESUME_BUTTON));

    await waitFor(() => expect(mocks.requestSessionRecover).toHaveBeenCalledTimes(2));
    expect(mocks.requestSessionRecover).toHaveBeenNthCalledWith(2, {
      taskId: TASK_ID,
      sessionId: SESSION_ID,
      action: "resume",
      failureMessage: "task:failedToResumeSession",
      settingsPolicy: "provider_restored",
    });
  });

  it("keeps provider-restored settings policy on the rendered manual retry", async () => {
    renderRecoveryScreen();
    await reachManualInspectionNotice();
    fireEvent.click(screen.getByTestId(RESUME_BUTTON));

    await waitFor(() => expect(mocks.requestSessionRecover).toHaveBeenCalledTimes(2));
    expect(mocks.requestSessionRecover.mock.calls[1][0]).toMatchObject({
      action: "resume",
      settingsPolicy: "provider_restored",
    });
  });

  it("uses the matching automatic owner's retry for its inspection notice", async () => {
    const resumeSession = vi.fn().mockResolvedValue(true);
    renderRecoveryScreen(automaticOwner(SESSION_ID, resumeSession));

    await act(async () => {
      fireEvent.click(screen.getByTestId(RESUME_BUTTON));
    });

    expect(resumeSession).toHaveBeenCalledTimes(1);
    expect(mocks.requestSessionRecover).not.toHaveBeenCalled();
  });
});
