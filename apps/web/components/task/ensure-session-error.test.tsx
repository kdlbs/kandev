import { describe, it, expect, afterEach, vi } from "vitest";
import { render, screen, cleanup, fireEvent } from "@testing-library/react";

import {
  describeEnsureError,
  EnsureSessionErrorBanner,
  EnsureSessionErrorEmptyState,
  SessionRecoveryFeedback,
} from "./ensure-session-error";

afterEach(cleanup);

const ENSURE_RETRY_TEST_ID = "ensure-session-error-retry";
const RESUME_FAILURE_DETAIL = "resume failed: provider request abc-123";
const RESTORE_FAILURE_DETAIL = "restore failed: workspace /tmp/task-123";

describe("describeEnsureError", () => {
  it("returns null when there is no error", () => {
    expect(describeEnsureError(null)).toBeNull();
  });

  it("detects the missing-agent-profile case and links to workspace settings", () => {
    const info = describeEnsureError(
      new Error("agent_profile_id is required to start agent"),
      "ws-1",
    );
    expect(info).not.toBeNull();
    expect(info?.isAgentProfileMissing).toBe(true);
    expect(info?.action).toEqual({
      label: "Open workspace settings",
      href: "/settings/workspaces/ws-1",
    });
  });

  it("returns a missing-agent-profile descriptor without an action when workspaceId is absent", () => {
    const info = describeEnsureError(new Error("agent_profile_id is required to start agent"));
    expect(info?.isAgentProfileMissing).toBe(true);
    expect(info?.action).toBeNull();
  });

  it("falls through to the generic error for unrelated failures", () => {
    const info = describeEnsureError(new Error("websocket disconnected"), "ws-1");
    expect(info?.isAgentProfileMissing).toBe(false);
    expect(info?.title).toBe("Couldn't start a session");
    expect(info?.detail).toContain("websocket disconnected");
    expect(info?.action).toBeNull();
  });

  it("does not misclassify unrelated errors that merely mention agent_profile_id", () => {
    const info = describeEnsureError(new Error("invalid agent_profile_id format"), "ws-1");
    expect(info?.isAgentProfileMissing).toBe(false);
    expect(info?.title).toBe("Couldn't start a session");
  });

  it("uses a fallback detail when the underlying error has no message", () => {
    const info = describeEnsureError(new Error(""));
    expect(info?.detail).toMatch(/backend rejected/i);
  });
});

// eslint-disable-next-line max-lines-per-function -- test describe block, splitting hurts readability
describe("EnsureSessionErrorBanner", () => {
  it("renders nothing when there is no error", () => {
    const { container } = render(
      <EnsureSessionErrorBanner error={null} onRetry={() => {}} workspaceId="ws-1" />,
    );
    expect(container.firstChild).toBeNull();
  });

  it("renders the missing-agent-profile message and a settings link", () => {
    render(
      <EnsureSessionErrorBanner
        error={new Error("agent_profile_id is required to start agent")}
        onRetry={() => {}}
        workspaceId="ws-1"
      />,
    );
    expect(screen.getByText("No agent profile configured")).toBeTruthy();
    const link = screen.getByTestId("ensure-session-error-action") as HTMLAnchorElement;
    expect(link.getAttribute("href")).toBe("/settings/workspaces/ws-1");
  });

  it("invokes onRetry when the retry button is clicked", () => {
    const onRetry = vi.fn();
    render(
      <EnsureSessionErrorBanner error={new Error("websocket disconnected")} onRetry={onRetry} />,
    );
    fireEvent.click(screen.getByTestId(ENSURE_RETRY_TEST_ID));
    expect(onRetry).toHaveBeenCalledTimes(1);
  });

  it("disables retry while another recovery action is pending", () => {
    const onRetry = vi.fn();
    render(
      <EnsureSessionErrorBanner
        error={new Error("recovery pending")}
        onRetry={onRetry}
        retryDisabled
      />,
    );
    const retry = screen.getByTestId(ENSURE_RETRY_TEST_ID) as HTMLButtonElement;
    expect(retry.disabled).toBe(true);

    fireEvent.click(retry);
    expect(onRetry).not.toHaveBeenCalled();
  });

  it("keeps automatic recovery causes behind a collapsed labeled disclosure", () => {
    render(
      <EnsureSessionErrorBanner
        error={new Error("Recovery could not complete")}
        onRetry={() => {}}
        recoveryFailure={{
          outcome: "recovery_failed",
          resumeError: RESUME_FAILURE_DETAIL,
          restoreError: RESTORE_FAILURE_DETAIL,
        }}
      />,
    );

    expect(screen.getByText("Session recovery failed")).toBeTruthy();
    expect(screen.getByText("The session could not be recovered.")).toBeTruthy();
    const details = screen.getByTestId("session-recovery-details") as HTMLDetailsElement;
    expect(details.open).toBe(false);

    fireEvent.click(screen.getByTestId("session-recovery-details-summary"));

    expect(details.open).toBe(true);
    expect(screen.getByText("Resume attempt")).toBeTruthy();
    expect(screen.getByText(RESUME_FAILURE_DETAIL)).toBeTruthy();
    expect(screen.getByText("Workspace restore attempt")).toBeTruthy();
    expect(screen.getByText(RESTORE_FAILURE_DETAIL)).toBeTruthy();
  });

  it("disables retry while automatic recovery is in flight", () => {
    render(
      <SessionRecoveryFeedback
        error="Recovery could not complete"
        notice={null}
        onRetry={() => {}}
        recoveryFailure={{
          outcome: "recovery_failed",
          resumeError: "resume failed",
          restoreError: "restore failed",
        }}
        retryDisabled
      />,
    );

    expect((screen.getByTestId(ENSURE_RETRY_TEST_ID) as HTMLButtonElement).disabled).toBe(true);
  });

  it("keeps a read-only fallback notice compact while retaining its resume cause in details", () => {
    render(
      <SessionRecoveryFeedback
        error={null}
        notice="Workspace restored in read-only mode"
        onRetry={() => {}}
        recoveryFailure={{
          outcome: "workspace_read_only",
          resumeError: RESUME_FAILURE_DETAIL,
        }}
      />,
    );

    expect(screen.getByText("Workspace restored in read-only mode")).toBeTruthy();
    const details = screen.getByTestId("session-recovery-details") as HTMLDetailsElement;
    expect(details.open).toBe(false);
    expect(screen.queryByText("Workspace restore attempt")).toBeNull();

    fireEvent.click(screen.getByTestId("session-recovery-details-summary"));

    expect(details.open).toBe(true);
    expect(screen.getByText("Resume attempt")).toBeTruthy();
    expect(screen.getByText(RESUME_FAILURE_DETAIL)).toBeTruthy();
  });

  it("renders status failures as compact retryable feedback with collapsed details", () => {
    const onRetry = vi.fn();
    render(
      <SessionRecoveryFeedback
        error={null}
        notice={null}
        onRetry={onRetry}
        recoveryFailure={{
          outcome: "status_unavailable",
          kind: "timeout",
          statusError: "WebSocket request timed out: task.session.status",
        }}
      />,
    );

    expect(screen.getByText("Session status is unavailable.")).toBeTruthy();
    expect(screen.queryByText("Couldn't start a session")).toBeNull();
    const details = screen.getByTestId("session-status-details") as HTMLDetailsElement;
    expect(details.open).toBe(false);
    fireEvent.click(screen.getByTestId("session-status-details-summary"));
    expect(screen.getByText("WebSocket request timed out: task.session.status")).toBeTruthy();
    fireEvent.click(screen.getByTestId("session-status-retry"));
    expect(onRetry).toHaveBeenCalledTimes(1);
  });

  it("renders page-wide feedback when automatic recovery and workspace restoration fail", () => {
    render(
      <SessionRecoveryFeedback
        error="Recovery could not complete"
        notice={null}
        onRetry={() => {}}
        recoveryFailure={{
          outcome: "recovery_failed",
          resumeError: RESUME_FAILURE_DETAIL,
          restoreError: RESTORE_FAILURE_DETAIL,
        }}
      />,
    );

    expect(screen.getByTestId("session-recovery-error")).toBeTruthy();
    expect(screen.getByText("Session recovery failed")).toBeTruthy();
    expect(screen.getByText("The session could not be recovered.")).toBeTruthy();
    expect(screen.queryByTestId("session-recovery-notice")).toBeNull();
  });
});

describe("EnsureSessionErrorEmptyState", () => {
  it("renders the centered preview-ensure-error layout with a retry", () => {
    const onRetry = vi.fn();
    render(
      <EnsureSessionErrorEmptyState
        error={new Error("boom")}
        onRetry={onRetry}
        workspaceId={null}
      />,
    );
    expect(screen.getByTestId("preview-ensure-error")).toBeTruthy();
    fireEvent.click(screen.getByTestId(ENSURE_RETRY_TEST_ID));
    expect(onRetry).toHaveBeenCalledTimes(1);
  });
});
