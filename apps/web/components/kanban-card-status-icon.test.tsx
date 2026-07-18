import { describe, expect, it } from "vitest";
import { isValidElement, type ReactNode } from "react";
import { IconCheck, IconLoader, IconLoader2 } from "@tabler/icons-react";
import { renderTaskStatusIcon } from "./kanban-card-content";
import type { Task } from "./kanban-card";

function task(overrides: Partial<Task>): Task {
  return {
    id: "task-1",
    title: "T",
    workflowStepId: "step-1",
    ...overrides,
  };
}

function iconType(node: ReactNode) {
  if (!isValidElement(node)) throw new Error("Expected React element");
  return node.type;
}

describe("renderTaskStatusIcon — task-level activity aggregate", () => {
  it("shows the background affordance when the primary session finished but a secondary runs background", () => {
    // Two-session case: most-active-wins reads as working, not done. showRunningSpinner
    // is false (primary is COMPLETED) yet the aggregate must still surface.
    const node = renderTaskStatusIcon(
      task({ state: "REVIEW", primarySessionState: "COMPLETED", foregroundActivity: "background" }),
      false,
      false,
    );
    expect(iconType(node)).toBe(IconLoader);
    expect(iconType(node)).not.toBe(IconCheck);
  });

  it("shows the generating spinner when a session generates even if the coarse state is done", () => {
    const node = renderTaskStatusIcon(
      task({ state: "COMPLETED", foregroundActivity: "generating" }),
      false,
      false,
    );
    expect(iconType(node)).toBe(IconLoader2);
  });

  it("renders nothing for a resting done task with no activity", () => {
    expect(renderTaskStatusIcon(task({ state: "COMPLETED" }), false, false)).toBeNull();
  });

  it("keeps the running spinner for an active primary session with no aggregate yet", () => {
    const node = renderTaskStatusIcon(
      task({ state: "IN_PROGRESS", primarySessionState: "RUNNING" }),
      true,
      false,
    );
    expect(iconType(node)).toBe(IconLoader2);
  });
});
