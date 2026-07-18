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
});
