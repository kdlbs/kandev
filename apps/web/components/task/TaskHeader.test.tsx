/**
 * Tests for TaskHeader — pure renderer with no domain branching.
 */

import { describe, it, expect, afterEach } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";
import { TaskHeader } from "./TaskHeader";

afterEach(cleanup);

describe("TaskHeader", () => {
  it("renders title plus identifier when provided", () => {
    render(<TaskHeader title="Implement feature" identifier="ABC-123" />);
    expect(screen.getByText("Implement feature")).toBeTruthy();
    expect(screen.getByText("ABC-123")).toBeTruthy();
  });

  it("renders title alone when identifier is absent", () => {
    render(<TaskHeader title="Implement feature" />);
    expect(screen.getByText("Implement feature")).toBeTruthy();
  });

  it("renders the state pill when state is provided", () => {
    render(<TaskHeader title="t" state="IN_PROGRESS" />);
    expect(screen.getByText("IN_PROGRESS")).toBeTruthy();
  });

  it("renders assignee name when provided", () => {
    render(<TaskHeader title="t" assigneeName="Alice" />);
    expect(screen.getByText("Alice")).toBeTruthy();
  });

  // §spec:task-level-indicator: the open-task header text badge reflects the
  // task-level activity aggregate — background-running reads distinctly and never
  // as a done state, even when the coarse workflow state is COMPLETED.
  it("reflects background-running in the badge instead of a done coarse state", () => {
    render(<TaskHeader title="t" state="COMPLETED" foregroundActivity="background" />);
    expect(screen.getByText("Background running")).toBeTruthy();
    expect(screen.queryByText("COMPLETED")).toBeNull();
  });

  it("reflects generating in the badge over a done coarse state", () => {
    render(<TaskHeader title="t" state="COMPLETED" foregroundActivity="generating" />);
    expect(screen.getByText("Generating")).toBeTruthy();
    expect(screen.queryByText("COMPLETED")).toBeNull();
  });

  it("keeps the coarse state in the badge when no session is active", () => {
    render(<TaskHeader title="t" state="COMPLETED" foregroundActivity={null} />);
    expect(screen.getByText("COMPLETED")).toBeTruthy();
  });

  // §spec:waiting-for-input-parity: the header badge carries the sidebar's rich
  // "needs me" reading, distinct from done and from running.
  it("reads a plain WAITING_FOR_INPUT state as 'Waiting for input'", () => {
    render(<TaskHeader title="t" state="WAITING_FOR_INPUT" />);
    expect(screen.getByText("Waiting for input")).toBeTruthy();
    expect(screen.queryByText("WAITING_FOR_INPUT")).toBeNull();
  });

  it("reads a pending clarification as 'Waiting for input' even over a non-waiting coarse state", () => {
    render(<TaskHeader title="t" state="IN_PROGRESS" hasPendingClarification />);
    expect(screen.getByText("Waiting for input")).toBeTruthy();
  });

  it("reads a pending permission as 'Permission requested', taking precedence over clarification", () => {
    render(
      <TaskHeader
        title="t"
        state="WAITING_FOR_INPUT"
        hasPendingClarification
        hasPendingPermission
      />,
    );
    expect(screen.getByText("Permission requested")).toBeTruthy();
  });

  it("keeps live activity ahead of a waiting flag in the badge", () => {
    render(
      <TaskHeader
        title="t"
        state="WAITING_FOR_INPUT"
        foregroundActivity="background"
        hasPendingPermission
      />,
    );
    expect(screen.getByText("Background running")).toBeTruthy();
    expect(screen.queryByText("Permission requested")).toBeNull();
  });
});
