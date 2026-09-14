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
    const details = screen.getByText("Technical details").closest("details");
    expect(details?.open).toBe(false);

    fireEvent.click(screen.getByText("Technical details"));
    expect(details?.open).toBe(true);
    const error = screen.getByText(/Could not resolve host/);
    expect(error).toBeTruthy();
    expect(error.className).toContain("max-h-48");
    expect(error.className).toContain("overflow-y-auto");
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
    fireEvent.click(screen.getByTestId("workspace-retry"));
    expect(onRetry).toHaveBeenCalledTimes(1);
    expect(screen.getByText("Technical details")).toBeTruthy();
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
    expect(screen.getByTestId("workspace-retry")).toHaveProperty("disabled", true);
  });
});
