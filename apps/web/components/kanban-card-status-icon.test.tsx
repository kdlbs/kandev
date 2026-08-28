import { describe, expect, it } from "vitest";
import { isValidElement, type ReactNode } from "react";
import {
  IconCheck,
  IconLoader,
  IconLoader2,
  IconMessageQuestion,
  IconShieldQuestion,
} from "@tabler/icons-react";
import { renderToStaticMarkup } from "react-dom/server";
import { CompositorSpin } from "@kandev/ui/compositor-spin";
import { renderSubagentCountChip, renderTaskStatusIcon } from "./kanban-card-content";
import { AutoStartFailedTaskIcon } from "@/lib/ui/state-icons";
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
  if (node.type === CompositorSpin) {
    return iconType((node.props as { children: ReactNode }).children);
  }
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
      false,
    );
    expect(iconType(node)).toBe(IconLoader2);
  });

  it("renders nothing for a resting done task with no activity", () => {
    expect(renderTaskStatusIcon(task({ state: "COMPLETED" }), false, false, false)).toBeNull();
  });

  it("keeps the running spinner for an active primary session with no aggregate yet", () => {
    const node = renderTaskStatusIcon(
      task({ state: "IN_PROGRESS", primarySessionState: "RUNNING" }),
      true,
      false,
      false,
    );
    expect(iconType(node)).toBe(IconLoader2);
  });
});

describe("renderTaskStatusIcon — waiting-for-input variants", () => {
  it("shows the message-question for a pending clarification, distinct from done and running", () => {
    const node = renderTaskStatusIcon(task({ state: "REVIEW" }), false, true, false);
    expect(iconType(node)).toBe(IconMessageQuestion);
    expect(iconType(node)).not.toBe(IconCheck);
    expect(iconType(node)).not.toBe(IconLoader2);
  });

  it("shows the shield-question for a pending permission, distinct from done and running", () => {
    const node = renderTaskStatusIcon(task({ state: "WAITING_FOR_INPUT" }), false, false, true);
    expect(iconType(node)).toBe(IconShieldQuestion);
    expect(iconType(node)).not.toBe(IconCheck);
    expect(iconType(node)).not.toBe(IconLoader2);
  });

  it("keeps the needs-me icon when a mid-turn prompt coincides with the running spinner", () => {
    // showRunningSpinner is true (coarse RUNNING) but a pending permission must
    // not be masked by the launch-spinner short-circuit.
    const node = renderTaskStatusIcon(
      task({ state: "IN_PROGRESS", primarySessionState: "RUNNING" }),
      true,
      false,
      true,
    );
    expect(iconType(node)).toBe(IconShieldQuestion);
  });
});

describe("renderTaskStatusIcon — auto-start failed", () => {
  it("shows the auto-start-failed triangle for a task whose on_enter launch failed", () => {
    const node = renderTaskStatusIcon(
      task({ state: "IN_PROGRESS", autoStartFailed: true }),
      false,
      false,
      false,
    );
    expect(iconType(node)).toBe(AutoStartFailedTaskIcon);
  });

  // The real shape a failed kanban auto-start leaves behind: startTask sets the
  // task to SCHEDULING before the launch, so a failure before session creation
  // produces a session-less SCHEDULING/IN_PROGRESS task, which is exactly what
  // shouldShowTaskRunningSpinner reads as "still launching" (showRunningSpinner
  // true). The triangle must not be masked by the launch-spinner short-circuit
  // the way needsMe already isn't.
  it("shows the triangle over the launch spinner for a session-less SCHEDULING task", () => {
    const node = renderTaskStatusIcon(
      task({ state: "SCHEDULING", autoStartFailed: true }),
      true,
      false,
      false,
    );
    expect(iconType(node)).toBe(AutoStartFailedTaskIcon);
    expect(iconType(node)).not.toBe(IconLoader2);
  });

  it("shows the triangle over the launch spinner for a session-less IN_PROGRESS task", () => {
    const node = renderTaskStatusIcon(
      task({ state: "IN_PROGRESS", autoStartFailed: true }),
      true,
      false,
      false,
    );
    expect(iconType(node)).toBe(AutoStartFailedTaskIcon);
    expect(iconType(node)).not.toBe(IconLoader2);
  });

  it("keeps the terminal done check over a lingering auto-start-failed marker", () => {
    const node = renderTaskStatusIcon(
      task({ state: "COMPLETED", autoStartFailed: true }),
      false,
      false,
      false,
    );
    expect(iconType(node)).toBe(IconCheck);
  });

  it("renders nothing for a resting task with the marker absent", () => {
    expect(renderTaskStatusIcon(task({ state: "IN_PROGRESS" }), false, false, false)).toBeNull();
  });
});

// active_subagent_count has been published end-to-end since the background-work
// liveness work, and reached the store with no component reading it — rendering
// it was an explicit non-goal of that spec. This is the follow-up.
describe("renderSubagentCountChip", () => {
  it("renders a chip carrying the count while subagents are live", () => {
    const node = renderSubagentCountChip(task({ activeSubagentCount: 3 }), "3 subagents running");
    expect(isValidElement(node)).toBe(true);
    expect(renderToStaticMarkup(node)).toContain("3");
  });

  it("renders nothing at zero", () => {
    expect(
      renderSubagentCountChip(task({ activeSubagentCount: 0 }), "0 subagents running"),
    ).toBeNull();
  });

  it("renders nothing when the field is absent", () => {
    expect(renderSubagentCountChip(task({}), "0 subagents running")).toBeNull();
  });

  it("labels the chip with a pluralized count for assistive tech", () => {
    expect(
      renderToStaticMarkup(
        renderSubagentCountChip(task({ activeSubagentCount: 1 }), "1 subagent running"),
      ),
    ).toContain('aria-label="1 subagent running"');
    expect(
      renderToStaticMarkup(
        renderSubagentCountChip(task({ activeSubagentCount: 2 }), "2 subagents running"),
      ),
    ).toContain('aria-label="2 subagents running"');
  });

  it("uses the locale-subscribed label supplied by its component", () => {
    expect(
      renderToStaticMarkup(
        renderSubagentCountChip(task({ activeSubagentCount: 1 }), "1 pšëúđø šûɓåĝëñŧ"),
      ),
    ).toContain('aria-label="1 pšëúđø šûɓåĝëñŧ"');
  });
});
