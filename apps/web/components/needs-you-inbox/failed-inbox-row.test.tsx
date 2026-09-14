import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { FailedInboxRow as FailedInboxRowData } from "@/lib/types/failed-inbox";
import { FailedInboxRow } from "./failed-inbox-row";

const ORIGIN_MARKER_TESTID = "failed-inbox-origin-marker";

function row(overrides: Partial<FailedInboxRowData> = {}): FailedInboxRowData {
  return {
    task_id: "t1",
    title: "Sync repository",
    workspace_id: "w1",
    origin: "manual",
    failure_instant: "2026-09-14T00:00:00Z",
    reason: "connection refused",
    ...overrides,
  };
}

afterEach(() => cleanup());

describe("FailedInboxRow", () => {
  it("renders the title, reason, and an open-task control (AC .18)", () => {
    render(<FailedInboxRow row={row()} />);

    expect(screen.getByText("Sync repository")).not.toBeNull();
    expect(screen.getByText("connection refused")).not.toBeNull();
    const link = screen.getByTestId("failed-inbox-open-task") as HTMLAnchorElement;
    expect(link.getAttribute("href")).toBe("/t/t1");
  });

  it("renders the task-state failed marker from the shared state-icon module (AC .18a)", () => {
    const { container } = render(<FailedInboxRow row={row()} />);
    expect(container.querySelector('[data-testid="failed-inbox-status-icon"]')).not.toBeNull();
  });

  it("presents the shared thread-status vocabulary's failed status (AC .20/.20a)", () => {
    render(<FailedInboxRow row={row()} />);
    expect(screen.getByRole("img", { name: "Failed" })).not.toBeNull();
  });

  it("falls back to stated copy when the reason is empty (AC .19)", () => {
    render(<FailedInboxRow row={row({ reason: "" })} />);
    expect(screen.getByTestId("failed-inbox-reason-fallback")).not.toBeNull();
  });

  it("falls back to stated copy when the reason is only whitespace (AC .19)", () => {
    render(<FailedInboxRow row={row({ reason: "   " })} />);
    expect(screen.getByTestId("failed-inbox-reason-fallback")).not.toBeNull();
  });

  it("renders no origin marker for the manual origin (AC .9a)", () => {
    render(<FailedInboxRow row={row({ origin: "manual" })} />);
    expect(screen.queryByTestId(ORIGIN_MARKER_TESTID)).toBeNull();
  });

  it("renders no origin marker for an absent origin (AC .9a)", () => {
    render(<FailedInboxRow row={row({ origin: "" })} />);
    expect(screen.queryByTestId(ORIGIN_MARKER_TESTID)).toBeNull();
  });

  it("renders a distinct marker for a non-person origin (AC .9a)", () => {
    render(<FailedInboxRow row={row({ origin: "automation_run" })} />);
    expect(screen.getByTestId(ORIGIN_MARKER_TESTID)).not.toBeNull();
  });

  it("renders the generic marker for an unenumerated non-empty origin (AC .9a)", () => {
    render(<FailedInboxRow row={row({ origin: "something_new" })} />);
    expect(screen.getByTestId(ORIGIN_MARKER_TESTID)).not.toBeNull();
  });

  it("states the failure time is unknown rather than substituting one when unresolvable (AC .30)", () => {
    render(<FailedInboxRow row={row({ failure_instant: undefined })} />);
    expect(screen.getByTestId("failed-inbox-unknown-time")).not.toBeNull();
  });

  it("carries a session id in the task href when the row has none, since the row model carries none", () => {
    render(<FailedInboxRow row={row()} />);
    const link = screen.getByTestId("failed-inbox-open-task") as HTMLAnchorElement;
    expect(link.getAttribute("href")).not.toContain("sessionId");
  });
});
