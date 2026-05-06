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
});
