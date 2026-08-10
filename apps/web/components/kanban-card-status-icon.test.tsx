import { describe, expect, it } from "vitest";
import { isValidElement, type ReactNode } from "react";
import { render } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import {
  IconCheck,
  IconLoader,
  IconLoader2,
  IconMessageQuestion,
  IconShieldQuestion,
} from "@tabler/icons-react";
import { renderToStaticMarkup } from "react-dom/server";
import { renderSubagentCountChip, renderTaskStatusIcon } from "./kanban-card-content";
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

describe("renderTaskStatusIcon — parked on background work", () => {
  // AC-58: a parked task at REVIEW with no pending input, no foreground_activity
  // and not interrupted must show the affordance against BOTH early returns —
  // the `return null` short-circuit (no activity/spinner/needsMe/interrupted)
  // and the bare-IconLoader2 launch-spinner short-circuit.
  it("renders the background-running affordance instead of null (first early return)", () => {
    const node = renderTaskStatusIcon(
      task({ state: "REVIEW", parkedOnBackgroundWork: true }),
      false,
      false,
      false,
    );
    expect(node).not.toBeNull();
    const { container } = render(<TooltipProvider>{node}</TooltipProvider>);
    expect(container.querySelector('[data-testid="task-state-background-running"]')).not.toBeNull();
  });

  it("renders the background-running affordance instead of the bare launch spinner (second early return)", () => {
    const node = renderTaskStatusIcon(
      task({ state: "IN_PROGRESS", primarySessionState: "RUNNING", parkedOnBackgroundWork: true }),
      true,
      false,
      false,
    );
    const { container } = render(<TooltipProvider>{node}</TooltipProvider>);
    expect(container.querySelector('[data-testid="task-state-background-running"]')).not.toBeNull();
  });

  it("AC-58b: a pending clarification wins over parked — the question is never hidden", () => {
    const node = renderTaskStatusIcon(
      task({ state: "WAITING_FOR_INPUT", parkedOnBackgroundWork: true }),
      false,
      true,
      false,
    );
    expect(iconType(node)).toBe(IconMessageQuestion);
  });

  it("AC-58b: a pending permission wins over parked — the question is never hidden", () => {
    const node = renderTaskStatusIcon(
      task({ state: "WAITING_FOR_INPUT", parkedOnBackgroundWork: true }),
      false,
      false,
      true,
    );
    expect(iconType(node)).toBe(IconShieldQuestion);
  });

  it("§7.4: a generating session outranks a merely parked one", () => {
    const node = renderTaskStatusIcon(
      task({ state: "REVIEW", foregroundActivity: "generating", parkedOnBackgroundWork: true }),
      false,
      false,
      false,
    );
    expect(iconType(node)).toBe(IconLoader2);
  });

  it("§7.4: background foreground activity outranks a merely parked one", () => {
    const node = renderTaskStatusIcon(
      task({ state: "REVIEW", foregroundActivity: "background", parkedOnBackgroundWork: true }),
      false,
      false,
      false,
    );
    expect(iconType(node)).toBe(IconLoader);
  });

  it("AC-59: parked=false renders exactly as today (null for a resting task)", () => {
    expect(
      renderTaskStatusIcon(
        task({ state: "REVIEW", parkedOnBackgroundWork: false }),
        false,
        false,
        false,
      ),
    ).toBeNull();
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
