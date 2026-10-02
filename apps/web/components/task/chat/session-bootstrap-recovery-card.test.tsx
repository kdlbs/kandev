import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import { SessionBootstrapRecoveryCard } from "./session-bootstrap-recovery-card";

const recoveryActionState = vi.hoisted(() => ({
  busyAction: null as string | null,
  recoveryError: null as Error | null,
  manualRecoveryFailure: null as {
    operation: "resume" | "restore_workspace";
    sessionId: string;
    errorStamp: string | null;
    requestKey: string;
    operationId: number;
  } | null,
  branchDetails: null as {
    kind: "branch_unrecoverable";
    recovery_action: "resume_new_branch";
  } | null,
  guardDetails: null as { retryable: boolean } | null,
  recoveryNotice: null as string | null,
  managedCloneRecoveryStamp: null as string | null,
  providerRestoredResumeEligible: false,
  handleRecover: vi.fn().mockResolvedValue(true),
  handleRestore: vi.fn().mockResolvedValue(undefined),
  handleNewBranch: vi.fn().mockResolvedValue(true),
  handleManagedCloneRelocation: vi.fn().mockResolvedValue(true),
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
  recoveryActionState.guardDetails = null;
  recoveryActionState.recoveryNotice = null;
  recoveryActionState.managedCloneRecoveryStamp = null;
  recoveryActionState.providerRestoredResumeEligible = false;
  vi.clearAllMocks();
});

// eslint-disable-next-line max-lines-per-function -- recovery outcomes share one focused card harness.
describe("SessionBootstrapRecoveryCard", () => {
  it("explains skipped overrides before an eligible explicit Resume", () => {
    recoveryActionState.providerRestoredResumeEligible = true;
    render(<SessionBootstrapRecoveryCard taskId="task-1" sessionId="session-1" error={error} />);

    const disclosure = screen.getByTestId("provider-restored-resume-disclosure");
    const resume = screen.getByTestId(RESUME_BUTTON_TEST_ID);
    expect(disclosure.textContent).toBe("task:providerRestoredResumeDisclosure");
    expect(
      disclosure.compareDocumentPosition(resume) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
  });

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
    expect(screen.getByTestId("session-bootstrap-cause-details").textContent).toContain(
      error.causes[0].detail,
    );
    expect(screen.getByTestId("session-bootstrap-recovery-details").getAttribute("open")).toBe(
      null,
    );
    expect(screen.getByTestId(RESUME_BUTTON_TEST_ID).className).toContain(
      "[@media(pointer:coarse)]:h-11",
    );
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
          recoveryFailure: {
            outcome: "workspace_read_only",
            resumeError: "token=raw-resume-secret",
          },
          resumeSession: vi.fn(),
        }}
      />,
    );

    expect(screen.getByTestId("session-bootstrap-recovery-card").getAttribute("role")).toBe(
      "status",
    );
    expect(screen.getAllByText("task:resumeFailedWorkspaceReadOnly").length).toBe(1);
    fireEvent.click(
      screen.getByTestId("session-bootstrap-recovery-details").querySelector("summary")!,
    );
    expect(screen.getByTestId("session-bootstrap-cause-details").textContent).toContain(
      "task:failedToResumeSession",
    );
    expect(screen.getByTestId("session-bootstrap-cause-details").textContent).not.toContain(
      "raw-resume-secret",
    );
    expect(screen.queryByTestId("session-bootstrap-recovery-error")).toBeNull();
  });

  it("keeps a typed selection cause primary after read-only workspace recovery", () => {
    render(
      <SessionBootstrapRecoveryCard
        taskId="task-1"
        sessionId="session-1"
        error={{
          ...error,
          details: "agent_bootstrap; cause=model_unavailable",
          attempt_id: "550e8400-e29b-41d4-a716-446655440000",
          execution_id: "650e8400-e29b-41d4-a716-446655440000",
          causes: [
            {
              operation: "start",
              code: "model_unavailable",
              reason: "requested_not_advertised",
              requested_model: "claude-opus-4-8",
              prompt_not_sent: true,
              detail: "The requested model was not advertised.",
            },
          ],
        }}
        automaticRecovery={{
          resumptionState: "resumed",
          error: null,
          notice: "task:resumeFailedWorkspaceReadOnly",
          recoveryFailure: {
            outcome: "workspace_read_only",
            resumeError: "request failed",
          },
          resumeSession: vi.fn(),
        }}
      />,
    );

    expect(screen.getByText("task:sessionBootstrapModelUnavailableTitle")).toBeTruthy();
    expect(screen.getByText("task:sessionBootstrapModelUnavailableBody")).toBeTruthy();
    expect(screen.getByTestId("session-recovery-workspace-status").textContent).toBe(
      "task:sessionRecoveryWorkspaceReadOnly",
    );
    expect(screen.getByTestId("session-bootstrap-no-prompt").textContent).toBe(
      "task:sessionBootstrapNoPromptSent",
    );
    expect(screen.getByTestId("session-recovery-fresh-start-warning")).toBeTruthy();

    fireEvent.click(
      screen.getByTestId("session-bootstrap-recovery-details").querySelector("summary")!,
    );
    const details = screen.getByTestId("session-bootstrap-cause-details").textContent ?? "";
    expect(details).toContain("claude-opus-4-8");
    expect(details).toContain("The requested model was not advertised.");
    expect(details).toContain("550e8400-e29b-41d4-a716-446655440000");
    expect(details).toContain("650e8400-e29b-41d4-a716-446655440000");
    expect(details).not.toContain("agent_bootstrap; cause=model_unavailable");
  });

  it("names the attempted fallback model when its application fails", () => {
    render(
      <SessionBootstrapRecoveryCard
        taskId="task-1"
        sessionId="session-1"
        error={{
          ...error,
          causes: [
            {
              operation: "start",
              code: "model_selection_failed",
              reason: "application_failed",
              requested_model: "primary",
              attempted_model: "alternate",
              detail: "The model could not be applied.",
            } as unknown as (typeof error.causes)[number],
          ],
        }}
      />,
    );

    expect(screen.getByTestId("session-bootstrap-recovery-card").textContent).toContain(
      "task:sessionBootstrapModelSelectionFailedBody",
    );
    expect(screen.getByTestId("session-bootstrap-recovery-card").textContent).not.toContain(
      'could not apply the selected model "primary"',
    );
    fireEvent.click(
      screen.getByTestId("session-bootstrap-recovery-details").querySelector("summary")!,
    );
    const details = screen.getByTestId("session-bootstrap-cause-details").textContent ?? "";
    expect(details).toContain("primary");
    expect(details).toContain("alternate");
  });

  it("keeps distinct branch guidance visible in the read-only result", () => {
    recoveryActionState.branchDetails = {
      kind: "branch_unrecoverable",
      recovery_action: "resume_new_branch",
    };
    render(
      <SessionBootstrapRecoveryCard
        taskId="task-1"
        sessionId="session-1"
        error={error}
        automaticRecovery={{
          resumptionState: "resumed",
          error: null,
          notice: "task:resumeFailedWorkspaceReadOnly",
          recoveryFailure: {
            outcome: "workspace_read_only",
            resumeError: "resume failed",
          },
          resumeSession: vi.fn(),
        }}
      />,
    );

    expect(screen.getAllByText("task:resumeFailedWorkspaceReadOnly")).toHaveLength(1);
    expect(screen.getByText("task:branchIsNoLongerAvailable")).toBeTruthy();
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
            resumeError: "token=raw-resume-secret",
            restoreError: "raw-restore-secret",
          },
          resumeSession: vi.fn(),
        }}
      />,
    );

    expect(screen.queryByRole("alert")).toBeNull();
    expect(screen.getByTestId("session-bootstrap-recovery-card")).toBeTruthy();
    fireEvent.click(
      screen.getByTestId("session-bootstrap-recovery-details").querySelector("summary")!,
    );
    const details = screen.getByTestId("session-bootstrap-cause-details");
    expect(details.textContent).toContain("task:sessionRecoveryResumeAttempt");
    expect(details.textContent).toContain("task:sessionRecoveryRestoreAttempt");
    expect(screen.getByTestId("session-bootstrap-cause-details").textContent).not.toContain(
      "raw-resume-secret",
    );
    expect(screen.queryByText("raw-restore-secret")).toBeNull();
  });

  it("uses the shared action row for repeated manual retry without raw backend output", () => {
    recoveryActionState.recoveryError = new Error("raw-backend-error".repeat(20));
    recoveryActionState.manualRecoveryFailure = {
      operation: "resume",
      sessionId: "session-1",
      errorStamp: "bootstrap-1",
      requestKey: "task-1\u0000session-1\u0000bootstrap-1",
      operationId: 1,
    };
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
    expect(screen.getByTestId("recovery-fresh-button").getAttribute("disabled")).not.toBeNull();
    expect(recoveryActionState.handleRecover).not.toHaveBeenCalled();
    expect(automaticResume).not.toHaveBeenCalled();
  });

  it("offers only a confirmed relocation for a managed clone mismatch", () => {
    render(
      <SessionBootstrapRecoveryCard
        taskId="task-1"
        sessionId="session-1"
        error={{ ...error, category: "managed_clone_relocation_required" }}
      />,
    );

    expect(screen.getByText("task:managedCloneRelocationTitle")).toBeTruthy();
    expect(screen.getByTestId("managed-clone-relocate-button")).toBeTruthy();
    expect(screen.queryByTestId(RESUME_BUTTON_TEST_ID)).toBeNull();
    expect(screen.queryByTestId("recovery-restore-workspace-button")).toBeNull();
    expect(screen.queryByTestId("recovery-fresh-button")).toBeNull();
    fireEvent.click(screen.getByTestId("managed-clone-relocate-button"));
    fireEvent.click(screen.getByTestId("managed-clone-relocation-confirm"));
    expect(recoveryActionState.handleManagedCloneRelocation).toHaveBeenCalledOnce();
  });

  it("changes a cancelled legacy restore failure to one relocation action", () => {
    recoveryActionState.manualRecoveryFailure = { operation: "restore_workspace" };
    recoveryActionState.recoveryError = new Error("workspace needs relocation");
    const { rerender } = render(
      <SessionBootstrapRecoveryCard taskId="task-1" sessionId="session-1" error={error} />,
    );
    expect(screen.getByTestId("recovery-restore-workspace-button")).toBeTruthy();
    expect(screen.getByTestId(RESUME_BUTTON_TEST_ID)).toBeTruthy();

    recoveryActionState.managedCloneRecoveryStamp = "managed-stamp-2";
    rerender(
      <SessionBootstrapRecoveryCard
        taskId="task-1"
        sessionId="session-1"
        error={{
          ...error,
          stamp: "managed-stamp-2",
          category: "managed_clone_relocation_required",
        }}
      />,
    );

    expect(screen.getAllByTestId("managed-clone-relocate-button")).toHaveLength(1);
    expect(screen.queryByTestId(RESUME_BUTTON_TEST_ID)).toBeNull();
    expect(screen.queryByTestId("recovery-fresh-button")).toBeNull();
    expect(screen.queryByTestId("recovery-restore-workspace-button")).toBeNull();
  });
});

it("keeps a refusal prerequisite visible and exposes no recovery bypass", () => {
  recoveryActionState.guardDetails = { retryable: false };
  recoveryActionState.recoveryError = new Error("Restart the backend before retrying.");
  render(<SessionBootstrapRecoveryCard taskId="task-1" sessionId="session-1" error={error} />);
  expect(screen.getByText("Restart the backend before retrying.")).toBeTruthy();
  expect(screen.queryByTestId(RESUME_BUTTON_TEST_ID)).toBeNull();
});

it("redacts a restore failure retained alongside guard details", () => {
  recoveryActionState.guardDetails = { retryable: true };
  recoveryActionState.recoveryError = new Error("Restore failed: token=guard-secret-fixture");
  render(<SessionBootstrapRecoveryCard taskId="task-1" sessionId="session-1" error={error} />);
  expect(document.body.textContent).not.toContain("guard-secret-fixture");
});
it("keeps the resume accessible name while pending", () => {
  recoveryActionState.busyAction = "resume";
  render(<SessionBootstrapRecoveryCard taskId="task-1" sessionId="session-1" error={error} />);
  expect(screen.getByTestId(RESUME_BUTTON_TEST_ID).getAttribute("aria-label")).toBe("task:resume");
});

it("withholds restore during a retryable startup guard", () => {
  recoveryActionState.guardDetails = { retryable: true };
  recoveryActionState.recoveryError = new Error("busy");
  render(<SessionBootstrapRecoveryCard taskId="task-1" sessionId="session-1" error={error} />);
  expect(screen.queryByTestId("recovery-restore-workspace-button")).toBeNull();
  expect(screen.getByTestId(RESUME_BUTTON_TEST_ID)).toBeTruthy();
});
