import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import type { ComponentProps } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Workflow, WorkflowStep } from "@/lib/types/http";
import { WorkflowCompactRow } from "./workflow-compact-row";

afterEach(cleanup);

const workflow = {
  id: "wf-1",
  workspace_id: "ws-1",
  name: "Development",
  description: "Default development workflow",
  created_at: "",
  updated_at: "",
} as Workflow;

function step(id: string, name: string, position: number, color = "bg-blue-500"): WorkflowStep {
  return {
    id,
    workflow_id: "wf-1",
    name,
    position,
    color,
    created_at: "",
    updated_at: "",
  } as WorkflowStep;
}

const ROW_TEST_ID = "workflow-compact-row-wf-1";

function renderRow(overrides: Partial<ComponentProps<typeof WorkflowCompactRow>> = {}) {
  const props: ComponentProps<typeof WorkflowCompactRow> = {
    workflow,
    steps: [step("s2", "Review", 2), step("s0", "Backlog", 0), step("s1", "In Progress", 1)],
    stepsLoading: false,
    agentProfileLabel: undefined,
    readOnlyReason: null,
    isDirty: false,
    expanded: false,
    onToggle: vi.fn(),
    bodyId: "workflow-body-wf-1",
    ...overrides,
  };
  render(<WorkflowCompactRow {...props} />);
  return props;
}

describe("WorkflowCompactRow", () => {
  it("shows the name, description, ordered steps, and step count", () => {
    renderRow();
    const row = screen.getByTestId(ROW_TEST_ID);
    expect(within(row).getByText("Development")).toBeTruthy();
    expect(within(row).getByText("Default development workflow")).toBeTruthy();
    const names = within(row)
      .getAllByTestId("workflow-compact-step")
      .map((el) => el.textContent);
    expect(names).toEqual(["Backlog", "In Progress", "Review"]);
    expect(within(row).getByText("3 steps")).toBeTruthy();
  });

  it("uses the singular step count", () => {
    renderRow({ steps: [step("s0", "Only", 0)] });
    expect(screen.getByText("1 step")).toBeTruthy();
  });

  it("shows the agent profile only when one is set", () => {
    renderRow({ agentProfileLabel: "Claude" });
    expect(screen.getByText("Claude")).toBeTruthy();
    cleanup();
    renderRow();
    expect(screen.queryByTestId("workflow-compact-profile")).toBeNull();
  });

  it("shows a loading label while steps load", () => {
    renderRow({ steps: [], stepsLoading: true });
    expect(screen.getByText("Loading workflow steps...")).toBeTruthy();
  });

  it("shows synced, read-only, and unsaved markers", () => {
    renderRow({ readOnlyReason: "synced", isDirty: true });
    expect(screen.getByText("Synced")).toBeTruthy();
    expect(screen.getByText("Unsaved")).toBeTruthy();
    cleanup();
    renderRow({ readOnlyReason: "improve" });
    expect(screen.getByText("Read-only")).toBeTruthy();
    expect(screen.queryByText("Unsaved")).toBeNull();
  });

  it("is a button exposing its expanded state and a localized name", () => {
    renderRow({ expanded: true });
    const button = screen.getByRole("button", { name: "Show or hide details for Development" });
    expect(button.getAttribute("aria-expanded")).toBe("true");
    expect(button.getAttribute("aria-controls")).toBe("workflow-body-wf-1");
  });

  it("describes the ordered steps, count, profile, and status", () => {
    renderRow({ agentProfileLabel: "Claude", readOnlyReason: "synced", isDirty: true });

    expect(
      screen.getByRole("button", {
        name: "Show or hide details for Development",
        description: /Backlog.*In Progress.*Review.*3 steps.*Claude.*Synced.*Unsaved/,
      }),
    ).toBeTruthy();
  });

  it("calls onToggle when activated", () => {
    const props = renderRow();
    fireEvent.click(screen.getByTestId(ROW_TEST_ID));
    expect(props.onToggle).toHaveBeenCalledTimes(1);
  });
});
