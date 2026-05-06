/**
 * Tests for DockviewLayout — covers F7.4 dormant-state semantics.
 *
 * The active path is dynamically loaded from a sibling module and
 * pulls in the full dockview-react bundle, so it isn't exercised in
 * the unit suite. We assert the dormant branch (executionId === null)
 * and the pure isDormant() helper.
 */

import { describe, it, expect, afterEach } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";
import { DockviewLayout, isDormant } from "./DockviewLayout";

afterEach(cleanup);

describe("isDormant", () => {
  it("treats null as dormant", () => {
    expect(isDormant(null)).toBe(true);
  });

  it("treats undefined as dormant", () => {
    expect(isDormant(undefined)).toBe(true);
  });

  it("treats any string as active", () => {
    expect(isDormant("exec_123")).toBe(false);
    expect(isDormant("")).toBe(false);
  });
});

describe("DockviewLayout dormant state", () => {
  it("renders the dormant placeholder when executionId is null", () => {
    render(<DockviewLayout executionId={null} sessionId="s1" taskId="t1" kind="office" />);
    expect(screen.getByRole("status")).toBeTruthy();
    expect(screen.getByText(/dormant/i)).toBeTruthy();
    // Banner mentions disabled affordances.
    expect(screen.getByText(/Terminal/)).toBeTruthy();
  });

  it("kind=kanban changes the dormant verb", () => {
    render(<DockviewLayout executionId={null} sessionId={null} taskId="t1" kind="kanban" />);
    expect(screen.getByText(/Agent is dormant/i)).toBeTruthy();
  });

  it("kind=office uses the routine wording", () => {
    render(<DockviewLayout executionId={null} sessionId={null} taskId="t1" kind="office" />);
    expect(screen.getByText(/Routine is dormant/i)).toBeTruthy();
  });
});
