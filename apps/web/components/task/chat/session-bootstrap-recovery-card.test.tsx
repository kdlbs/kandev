import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import { SessionBootstrapRecoveryCard } from "./session-bootstrap-recovery-card";

const recoveryActionState = vi.hoisted(() => ({
  busyAction: null as string | null,
  recoveryError: null as Error | null,
  manualRecoveryFailure: null as { operation: "resume" | "restore_workspace" } | null,
  branchDetails: null,
  recoveryNotice: null as string | null,
  handleRecover: vi.fn().mockResolvedValue(true),
  handleRestore: vi.fn().mockResolvedValue(undefined),
  handleNewBranch: vi.fn().mockResolvedValue(true),
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

vi.mock("@/components/task/new-session-dialog", () => ({
  NewSessionDialog: () => null,
}));

vi.mock("@/components/task/chat/session-stopped-banner", () => ({
  useSessionProfileExists: () => true,
}));

vi.mock("@/hooks/domains/session/use-session-recovery-actions", () => ({
  useSessionRecoveryActions: () => recoveryActionState,
}));

vi.mock("@kandev/ui/tooltip", () => ({
  Tooltip: ({ children }: { children: ReactNode }) => <>{children}</>,
  TooltipContent: ({ children }: { children: ReactNode }) => <>{children}</>,
  TooltipTrigger: ({ children }: { children: ReactNode }) => <>{children}</>,
}));

const error = {
  session_id: "session-1",
  stamp: "bootstrap-1",
  occurred_at: "2026-09-11T10:00:00Z",
  preview: "The agent could not start.",
  details: "agent_bootstrap; cause=permission_denied",
  phase: "bootstrap",
  category: "generic_launch_failure",
  causes: [
    {
      operation: "resume",
      code: "permission_denied",
      detail: "The required contribution access was denied.",
    },
  ],
};
const RESUME_BUTTON_TEST_ID = "recovery-resume-button";

/* eslint-disable sonarjs/no-duplicate-string -- repeated recovery keys make each outcome assertion explicit. */

afterEach(() => {
  cleanup();
  recoveryActionState.busyAction = null;
  recoveryActionState.recoveryError = null;
  recoveryActionState.manualRecoveryFailure = null;
  recoveryActionState.branchDetails = null;
  recoveryActionState.recoveryNotice = null;
  vi.clearAllMocks();
});

// eslint-disable-next-line max-lines-per-function -- recovery outcomes share one focused card harness.
describe("SessionBootstrapRecoveryCard", () => {
  it("shows safe cause details and exposes mobile-sized recovery actions", () => {
    render(
      <SessionBootstrapRecoveryCard
        taskId="task-1"
        sessionId="session-1"
        workspaceId="workspace-1"
        error={error}
      />,
    );

    expect(screen.getByTestId("session-bootstrap-recovery-card")).toBeTruthy();
    expect(screen.getByText("task:sessionBootstrapCausePermissionDenied")).toBeTruthy();
    expect(screen.getByText(error.causes[0].detail)).toBeTruthy();
    expect(screen.getByTestId("session-bootstrap-recovery-details").getAttribute("open")).toBe(
      null,
    );
    for (const testId of [
      RESUME_BUTTON_TEST_ID,
      "recovery-restore-workspace-button",
      "recovery-fresh-button",
    ]) {
      expect(screen.getByTestId(testId).className).toContain("[@media(pointer:coarse)]:min-h-11");
    }

    fireEvent.click(screen.getByTestId(RESUME_BUTTON_TEST_ID));
    fireEvent.click(screen.getByTestId("recovery-restore-workspace-button"));
    fireEvent.click(screen.getByTestId("recovery-fresh-button"));

    expect(recoveryActionState.handleRecover).toHaveBeenNthCalledWith(1, "resume");
    expect(recoveryActionState.handleRestore).toHaveBeenCalledTimes(1);
    expect(recoveryActionState.handleRecover).toHaveBeenNthCalledWith(2, "fresh_start");
  });

  it("keeps the automatic read-only result inside the shared informational card", () => {
    render(
      <SessionBootstrapRecoveryCard
        taskId="task-1"
        sessionId="session-1"
        error={error}
        automaticRecovery={{
          resumptionState: "resumed",
          error: null,
          notice: "task:resumeFailedWorkspaceReadOnly",
          recoveryFailure: { outcome: "workspace_read_only", resumeError: "raw-resume-secret" },
          resumeSession: vi.fn(),
        }}
      />,
    );

    expect(screen.getByRole("status")).toBeTruthy();
    expect(screen.getAllByText("task:resumeFailedWorkspaceReadOnly").length).toBe(2);
    fireEvent.click(
      screen.getByTestId("session-bootstrap-recovery-details").querySelector("summary")!,
    );
    expect(screen.getByText("task:failedToResumeSession")).toBeTruthy();
    expect(screen.queryByText("raw-resume-secret")).toBeNull();
    expect(screen.queryByTestId("session-bootstrap-recovery-error")).toBeNull();
  });

  it("keeps automatic resume and restore failures as labeled sanitized causes", () => {
    render(
      <SessionBootstrapRecoveryCard
        taskId="task-1"
        sessionId="session-1"
        error={error}
        automaticRecovery={{
          resumptionState: "error",
          error: "task:sessionRecoveryFailed",
          notice: null,
          recoveryFailure: {
            outcome: "recovery_failed",
            resumeError: "raw-resume-secret",
            restoreError: "raw-restore-secret",
          },
          resumeSession: vi.fn(),
        }}
      />,
    );

    expect(screen.getByRole("alert")).toBeTruthy();
    fireEvent.click(
      screen.getByTestId("session-bootstrap-recovery-details").querySelector("summary")!,
    );
    const details = screen.getByTestId("session-bootstrap-cause-details");
    expect(details.textContent).toContain("task:sessionRecoveryResumeAttempt");
    expect(details.textContent).toContain("task:sessionRecoveryRestoreAttempt");
    expect(screen.queryByText("raw-resume-secret")).toBeNull();
    expect(screen.queryByText("raw-restore-secret")).toBeNull();
  });

  it("uses the shared action row for repeated manual retry without raw backend output", () => {
    recoveryActionState.recoveryError = new Error("raw-backend-error".repeat(20));
    recoveryActionState.manualRecoveryFailure = { operation: "resume" };
    render(<SessionBootstrapRecoveryCard taskId="task-1" sessionId="session-1" error={error} />);

    expect(screen.queryByText(/raw-backend-error/)).toBeNull();
    expect(screen.queryByTestId("session-bootstrap-recovery-error")).toBeNull();
    fireEvent.click(screen.getByTestId(RESUME_BUTTON_TEST_ID));
    fireEvent.click(screen.getByTestId(RESUME_BUTTON_TEST_ID));
    expect(recoveryActionState.handleRecover).toHaveBeenCalledWith("resume");
    expect(recoveryActionState.handleRecover).toHaveBeenCalledTimes(2);
  });

  it("disables manual actions while automatic recovery is in flight", () => {
    const automaticResume = vi.fn();
    render(
      <SessionBootstrapRecoveryCard
        taskId="task-1"
        sessionId="session-1"
        error={error}
        automaticRecovery={{
          resumptionState: "resuming",
          error: null,
          notice: null,
          recoveryFailure: null,
          resumeSession: automaticResume,
        }}
      />,
    );

    expect(screen.getByTestId(RESUME_BUTTON_TEST_ID).getAttribute("disabled")).not.toBeNull();
    expect(
      screen.getByTestId("recovery-restore-workspace-button").getAttribute("disabled"),
    ).not.toBeNull();
    expect(recoveryActionState.handleRecover).not.toHaveBeenCalled();
    expect(automaticResume).not.toHaveBeenCalled();
  });
});
