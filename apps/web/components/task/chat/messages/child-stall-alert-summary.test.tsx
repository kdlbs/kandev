import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { ChildStallAlertSummary, childStallAlertsFromMetadata } from "./child-stall-alert-summary";

afterEach(cleanup);

const LIVE_ALERT = {
  child_task_id: "child-1",
  child_task_title: "Write the parser",
  child_session_id: "child-session-1",
  turn_id: "turn-0123456789",
  step_id: "step-1",
  step_name: "Implement",
  cause: "input_required",
};

describe("childStallAlertsFromMetadata", () => {
  it("returns the single descriptor of a production-shaped alert", () => {
    const alerts = childStallAlertsFromMetadata({
      child_stall_alert: true,
      child_stall_alerts: [LIVE_ALERT],
      sender_task_id: "child-1",
    });
    expect(alerts).toEqual([LIVE_ALERT]);
  });

  it("keeps every descriptor of a folded alert in order", () => {
    const second = { child_task_id: "child-2", turn_id: "turn-2", cause: "quota" };
    expect(
      childStallAlertsFromMetadata({
        child_stall_alert: true,
        child_stall_alerts: [LIVE_ALERT, second],
      }),
    ).toEqual([LIVE_ALERT, second]);
  });

  it("returns nothing for a message that is not an alert or has no list", () => {
    expect(childStallAlertsFromMetadata(undefined)).toEqual([]);
    expect(childStallAlertsFromMetadata({ child_stall_alerts: [LIVE_ALERT] })).toEqual([]);
    expect(childStallAlertsFromMetadata({ child_stall_alert: true })).toEqual([]);
    expect(
      childStallAlertsFromMetadata({ child_stall_alert: true, child_stall_alerts: [] }),
    ).toEqual([]);
  });

  it("drops malformed descriptors", () => {
    expect(
      childStallAlertsFromMetadata({
        child_stall_alert: true,
        child_stall_alerts: [
          null,
          { child_task_id: "" },
          { child_task_id: "child-3", cause: 7 },
          { child_task_id: "child-4", child_session_id: 9 },
          LIVE_ALERT,
        ],
      }),
    ).toEqual([LIVE_ALERT]);
  });
});

describe("ChildStallAlertSummary", () => {
  it("renders nothing without alerts", () => {
    const { container } = render(<ChildStallAlertSummary alerts={[]} />);
    expect(container.firstChild).toBeNull();
  });

  it("links each alert to the stalled child session with step, cause, and turn", () => {
    render(<ChildStallAlertSummary alerts={[LIVE_ALERT]} />);
    const link = screen.getByTestId("child-stall-alert-link");
    expect(link.getAttribute("href")).toBe("/t/child-1?sessionId=child-session-1");
    expect(link.textContent).toContain("Write the parser");
    const row = screen.getByTestId("child-stall-alert");
    expect(row.textContent).toContain("Step: Implement");
    expect(row.textContent).toContain("Cause: Input required");
    expect(row.textContent).toContain("Turn: turn-012");
  });

  it("links to the child task when the descriptor has no session", () => {
    render(<ChildStallAlertSummary alerts={[{ child_task_id: "child-2" }]} />);
    expect(screen.getByTestId("child-stall-alert-link").getAttribute("href")).toBe("/t/child-2");
    expect(screen.getByTestId("child-stall-alert-link").textContent).toContain("child-2");
  });

  it.each([
    ["input_required", "Input required"],
    ["quota", "Usage limit"],
    ["execution_error", "Execution error"],
    ["missing_completion_signal", "No completion signal"],
    ["something_new", "something_new"],
  ])("labels the %s cause", (cause, label) => {
    render(<ChildStallAlertSummary alerts={[{ child_task_id: "child-1", cause }]} />);
    expect(screen.getByTestId("child-stall-alert").textContent).toContain(`Cause: ${label}`);
  });

  it("renders one row per folded alert", () => {
    render(
      <ChildStallAlertSummary
        alerts={[LIVE_ALERT, { child_task_id: "child-2", turn_id: "turn-2", cause: "quota" }]}
      />,
    );
    expect(screen.getAllByTestId("child-stall-alert")).toHaveLength(2);
  });
});
