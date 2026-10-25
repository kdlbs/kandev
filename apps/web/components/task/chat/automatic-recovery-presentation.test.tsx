import { afterEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import type { AppState } from "@/lib/state/store";
import type { SessionRecoveryOwner } from "@/lib/session-recovery-presentation";
import type { SessionRecoveryActions } from "@/hooks/domains/session/use-session-recovery-actions";
import { useAutomaticRecoveryChatOwner } from "@/hooks/domains/session/use-automatic-recovery-chat-owner";
import { SessionRecoveryFeedback } from "../ensure-session-error";
import { TaskLaunchErrorProvider } from "../task-launch-error-context";
import { SessionRecoveryCard } from "./session-recovery-card";

vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast: vi.fn() }) }));
afterEach(cleanup);
const RECOVERY_CARD = "session-recovery-card";

const recovery: SessionRecoveryOwner = {
  requestIdentity: { taskId: "task", sessionId: "session", generation: 1, attemptId: 1 },
  resumptionState: "error",
  error: "Session recovery failed",
  notice: null,
  recoveryFailure: {
    outcome: "recovery_failed",
    resumeError: "Resume transport failed\nAuthorization: Bearer secret-fixture",
    restoreError: "Workspace transport failed",
  },
  resumeSession: vi.fn(),
};
const actions = {
  busyAction: null,
  recoveryError: null,
  guardDetails: null,
  branchDetails: null,
  recoveryNotice: null,
  manualRecoveryFailure: null,
  handleRecover: vi.fn(),
} as unknown as SessionRecoveryActions;

function Presentation({ automatic }: { automatic: SessionRecoveryOwner }) {
  const ownedByChat = useAutomaticRecoveryChatOwner({
    taskId: "task",
    sessionId: "session",
    recovery: automatic,
  });
  return (
    <TaskLaunchErrorProvider
      value={{ taskId: "task", workspaceId: "workspace", automaticRecovery: automatic }}
    >
      <SessionRecoveryFeedback {...automatic} ownedByChat={ownedByChat} onRetry={vi.fn()} />
      <SessionRecoveryCard
        model={{ sessionId: "session", kind: "generic", summary: "Connection lost" }}
        actions={actions}
        onNewSession={vi.fn()}
      />
    </TaskLaunchErrorProvider>
  );
}

function show(automatic = recovery) {
  return render(
    <StateProvider
      initialState={
        {
          taskSessions: {
            items: {
              session: {
                id: "session",
                task_id: "task",
                state: "FAILED",
                agent_profile_id: "profile",
              },
            },
          },
          agentProfiles: { items: [{ id: "profile" }] },
        } as unknown as Partial<AppState>
      }
    >
      <Presentation automatic={automatic} />
    </StateProvider>,
  );
}

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.1
it("automatic recovery without bootstrap metadata has one active owner and both safe causes", () => {
  show();
  expect(screen.getAllByTestId(RECOVERY_CARD)).toHaveLength(1);
  expect(screen.queryByTestId("session-recovery-error")).toBeNull();
  fireEvent.click(screen.getByText("Technical details", { exact: true }));
  const details = screen.getByTestId(RECOVERY_CARD).textContent;
  expect(details).toContain("Resume transport failed");
  expect(details).toContain("Workspace transport failed");
  expect(details).not.toContain("secret-fixture");
});

it("keeps unmatched automatic failures in the outer fallback", () => {
  show({ ...recovery, requestIdentity: { ...recovery.requestIdentity!, sessionId: "other" } });
  expect(screen.getByTestId("session-recovery-error")).toBeTruthy();
  expect(screen.getByTestId(RECOVERY_CARD).textContent).not.toContain("Resume transport failed");
});

it("disables both recovery actions during the owned automatic attempt", () => {
  show({ ...recovery, resumptionState: "resuming", recoveryFailure: null, error: null });
  expect(screen.getByTestId("recovery-resume-button")).toHaveProperty("disabled", true);
  expect(screen.getByTestId("recovery-fresh-button")).toHaveProperty("disabled", true);
});

it("retains workspace-only success as a notice in the stopped-agent card", () => {
  show({
    ...recovery,
    notice: "Workspace restored; agent remains stopped",
    error: null,
    recoveryFailure: { outcome: "workspace_read_only", resumeError: "Resume transport failed" },
  });
  expect(screen.getByTestId(RECOVERY_CARD).textContent).toContain(
    "Workspace restored; agent remains stopped",
  );
  expect(screen.queryByTestId("session-recovery-error")).toBeNull();
});
