import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { WorkspaceUnavailable } from "./workspace-unavailable";

afterEach(cleanup);

describe("WorkspaceUnavailable", () => {
  it("keeps the raw session error behind a collapsed disclosure", () => {
    render(
      <WorkspaceUnavailable error="fatal: unable to access github.com: Could not resolve host" />,
    );

    expect(screen.getByText("Workspace unavailable")).toBeTruthy();
    expect(screen.queryByText("Session failed")).toBeNull();
    const details = screen.getByText(DETAILS_LABEL).closest("details");
    expect(details?.open).toBe(false);

    fireEvent.click(screen.getByText(DETAILS_LABEL));
    expect(details?.open).toBe(true);
    const error = screen.getByText(/Could not resolve host/);
    expect(error).toBeTruthy();
    expect(error.className).not.toContain("max-h-48");
    expect(error.className).not.toContain("overflow-y-auto");
  });

  it("shows a retry action and scoped diagnostics for workspace restoration", () => {
    const onRetry = vi.fn();
    render(
      <WorkspaceUnavailable
        restoration={{
          taskId: "task-1",
          sessionId: "session-1",
          environmentId: "environment-1",
          revision: 1,
          status: "error",
          details: "workspace admission failed",
        }}
        onRetry={onRetry}
      />,
    );

    expect(screen.getByText("Couldn't reconnect to this task's workspace.")).toBeTruthy();
    fireEvent.click(screen.getByTestId(WORKSPACE_RETRY_ID));
    expect(onRetry).toHaveBeenCalledTimes(1);
    expect(screen.getByText(DETAILS_LABEL)).toBeTruthy();
  });

  it("disables retry while restoration is pending", () => {
    render(
      <WorkspaceUnavailable
        restoration={{
          taskId: "task-1",
          sessionId: "session-1",
          environmentId: "environment-1",
          revision: 1,
          status: "pending",
        }}
        onRetry={vi.fn()}
        retryDisabled
      />,
    );

    expect(screen.getByText("Reconnecting to this workspace...")).toBeTruthy();
    expect(screen.getByTestId(WORKSPACE_RETRY_ID)).toHaveProperty("disabled", true);
  });
});

const WORKSPACE_RETRY_ID = "workspace-retry";

const ownerContext = vi.hoisted(() => ({ current: null as unknown }));
vi.mock("./task-launch-error-context", () => ({
  useTaskLaunchErrorContext: () => ownerContext.current,
}));

it("routes an explicitly correlated restore failure to its visible recovery owner", async () => {
  ownerContext.current = {
    taskId: "task-1",
    automaticRecovery: {
      recoveryFailure: {
        outcome: "recovery_failed",
        workspaceAttemptId: "attempt-1",
      },
    },
  };
  const restoration = {
    taskId: "task-1",
    sessionId: "session-1",
    environmentId: "env-1",
    attemptId: "attempt-1",
    revision: 1,
    status: "error" as const,
    details: "restore failed",
  };
  const { rerender } = render(
    <>
      <div id="session-recovery-owner-attempt-1" tabIndex={-1}>
        Recovery owner
      </div>
      <WorkspaceUnavailable restoration={restoration} onRetry={vi.fn()} />
    </>,
  );
  expect(await screen.findByRole("link", { name: VIEW_RECOVERY })).toBeTruthy();
  expect(screen.queryByTestId(WORKSPACE_RETRY_ID)).toBeNull();
  const owner = document.getElementById("session-recovery-owner-attempt-1")!;
  Object.defineProperty(owner, "checkVisibility", { value: () => false });
  fireEvent.click(screen.getByRole("link", { name: VIEW_RECOVERY }));
  expect(screen.getByTestId(WORKSPACE_RETRY_ID)).toBeTruthy();
  rerender(
    <WorkspaceUnavailable
      restoration={{ ...restoration, attemptId: "independent-attempt" }}
      onRetry={vi.fn()}
    />,
  );
  expect(screen.getByTestId(WORKSPACE_RETRY_ID)).toBeTruthy();
  ownerContext.current = null;
});

it("navigates a dependent failed-session pane to Chat while preserving independent errors", () => {
  const revealSessionRecovery = vi.fn();
  ownerContext.current = {
    taskId: "task-1",
    statusSummary: {
      active_error: { scope: "session", session_id: "session-1", stamp: "current" },
    },
    revealSessionRecovery,
  };
  const { rerender } = render(
    <WorkspaceUnavailable failedSessionId="session-1" error="dependent failure" />,
  );
  fireEvent.click(screen.getByRole("link", { name: VIEW_RECOVERY }));
  expect(revealSessionRecovery).toHaveBeenCalledWith("session-1");
  expect(screen.queryByText(DETAILS_LABEL)).toBeNull();
  rerender(<WorkspaceUnavailable failedSessionId="session-2" error="independent failure" />);
  expect(screen.queryByRole("link", { name: VIEW_RECOVERY })).toBeNull();
  expect(screen.getByText(DETAILS_LABEL)).toBeTruthy();
  ownerContext.current = null;
});

const DETAILS_LABEL = "Technical details";

const VIEW_RECOVERY = "View recovery";
