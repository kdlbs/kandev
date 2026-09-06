import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { WorkflowStep } from "@/lib/types/http";
import { AutomationTab } from "./automation-tab";

const step = {
  id: "step-1",
  workflow_id: "workflow-1",
  name: "Review",
  position: 0,
  color: "bg-slate-500",
  events: {
    on_enter: [{ type: "run_script", config: { command: "echo enter" } }],
    on_turn_complete: [{ type: "move_to_next" }],
    on_exit: [{ type: "run_script", config: { command: "echo exit" } }],
  },
  created_at: "",
  updated_at: "",
} as WorkflowStep;
const enterActionList = "workflow-action-list-on_enter";

describe("AutomationTab", () => {
  afterEach(cleanup);

  it("renders one ordered recipe group for each supported lifecycle trigger", () => {
    render(<AutomationTab step={step} steps={[step]} readOnly={false} onUpdate={vi.fn()} />);

    expect(screen.getByTestId(enterActionList)).toBeTruthy();
    expect(screen.getByTestId("workflow-action-list-on_turn_complete")).toBeTruthy();
    expect(screen.getByTestId("workflow-action-list-on_exit")).toBeTruthy();
    expect(screen.getByTestId("workflow-action-list-on_children_completed")).toBeTruthy();
    expect(screen.getByText("echo enter")).toBeTruthy();
    expect(screen.getByText("echo exit")).toBeTruthy();
  });

  it("keeps the add palette and action editor unavailable in read-only mode", () => {
    render(<AutomationTab step={step} steps={[step]} readOnly onUpdate={vi.fn()} />);

    expect(screen.queryAllByRole("button", { name: /add action/i })).toHaveLength(0);
    expect(screen.getByTestId(enterActionList).getAttribute("data-read-only")).toBe("true");
  });

  it("preserves other action config while editing a script field", () => {
    const onUpdate = vi.fn();
    render(
      <AutomationTab
        step={{
          ...step,
          events: {
            ...step.events,
            on_enter: [
              {
                type: "run_script",
                config: { command: "echo enter", timeout_seconds: 30, failure_policy: "continue" },
              },
            ],
          },
        }}
        steps={[step]}
        readOnly={false}
        onUpdate={onUpdate}
        focusedAction={{ trigger: "on_enter", index: 0 }}
      />,
    );

    fireEvent.change(screen.getByLabelText("Command"), { target: { value: "echo changed" } });

    expect(onUpdate).toHaveBeenCalledWith({
      events: expect.objectContaining({
        on_enter: [
          {
            type: "run_script",
            config: { command: "echo changed", timeout_seconds: 30, failure_policy: "continue" },
          },
        ],
      }),
    });
  });

  it("does not mark a valid script command as invalid", () => {
    render(
      <AutomationTab
        step={step}
        steps={[step]}
        readOnly={false}
        onUpdate={vi.fn()}
        focusedAction={{ trigger: "on_enter", index: 0 }}
      />,
    );

    expect(screen.getByLabelText("Command").getAttribute("aria-invalid")).toBeNull();
  });

  it("replaces the action list with a focused editor and provides a way back", () => {
    render(
      <AutomationTab
        step={step}
        steps={[step]}
        readOnly={false}
        onUpdate={vi.fn()}
        focusedAction={{ trigger: "on_enter", index: 0 }}
      />,
    );

    expect(screen.getByTestId("workflow-focused-action-editor")).toBeTruthy();
    expect(screen.queryByTestId(enterActionList)).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Back to automation" }));

    expect(screen.getByTestId(enterActionList)).toBeTruthy();
  });
});
