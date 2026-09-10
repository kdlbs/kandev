import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { WorkflowStep } from "@/lib/types/http";
import { workflowId as toWorkflowId } from "@/lib/types/ids";
import {
  CompleteTaskOnEnterToggle,
  TurnCompleteSelect,
} from "./workflow-pipeline-editor-step-actions";

afterEach(cleanup);

const STEP_ID = "step-1";
const CANCEL_COMPLETION_CHECKBOX_TEST_ID = `${STEP_ID}-cancel-completion-checkbox`;
const CANCEL_COMPLETION_HELP_TEST_ID = `${STEP_ID}-cancel-completion-help`;
const COMPLETE_TASK_CHECKBOX_TEST_ID = `${STEP_ID}-complete-task-on-enter-checkbox`;
const COMPLETE_TASK_HELP_TEST_ID = `${STEP_ID}-complete-task-on-enter-help`;
const CHECKED_ARIA_VALUE = "true";
const UNCHECKED_ARIA_VALUE = "false";
const ARIA_CHECKED_ATTRIBUTE = "aria-checked";

function step(overrides: Partial<WorkflowStep> = {}): WorkflowStep {
  return {
    id: STEP_ID,
    workflow_id: toWorkflowId("workflow-1"),
    name: "In Progress",
    position: 0,
    color: "bg-blue-500",
    events: { on_turn_complete: [{ type: "move_to_next" }] },
    created_at: "",
    updated_at: "",
    ...overrides,
  };
}

function renderTurnComplete(current: WorkflowStep, readOnly = false) {
  const onUpdate = vi.fn();
  render(
    <TurnCompleteSelect
      step={current}
      savedStep={current}
      otherSteps={[]}
      onUpdate={onUpdate}
      setTransition={vi.fn()}
      toggleDisablePlanMode={vi.fn()}
      planModeEnabled={false}
      readOnly={readOnly}
    />,
  );
  return { onUpdate };
}

describe("TurnCompleteSelect cancel completion policy", () => {
  it("renders the policy disabled by default and updates it when checked", () => {
    const { onUpdate } = renderTurnComplete(step());
    const checkbox = screen.getByTestId(CANCEL_COMPLETION_CHECKBOX_TEST_ID);

    expect(checkbox.getAttribute(ARIA_CHECKED_ATTRIBUTE)).toBe(UNCHECKED_ARIA_VALUE);
    expect(screen.getByTestId(CANCEL_COMPLETION_HELP_TEST_ID).getAttribute("aria-label")).toBe(
      "More information",
    );
    expect(screen.queryByText(/Applies only when a user explicitly cancels a turn\./)).toBeNull();
    fireEvent.click(checkbox);
    expect(onUpdate).toHaveBeenCalledWith({ cancel_triggers_turn_complete: true });
  });

  it("preserves the persisted value and disables it in read-only mode", () => {
    renderTurnComplete(step({ cancel_triggers_turn_complete: true }), true);
    const checkbox = screen.getByTestId(CANCEL_COMPLETION_CHECKBOX_TEST_ID);

    expect(checkbox.getAttribute(ARIA_CHECKED_ATTRIBUTE)).toBe(CHECKED_ARIA_VALUE);
    expect((checkbox as HTMLButtonElement).disabled).toBe(true);
  });

  it("does not render the subordinate policy when no completion transition is configured", () => {
    renderTurnComplete(
      step({ events: { on_turn_complete: [] }, cancel_triggers_turn_complete: true }),
    );

    expect(screen.queryByTestId(CANCEL_COMPLETION_CHECKBOX_TEST_ID)).toBeNull();
  });
});

describe("CompleteTaskOnEnterToggle", () => {
  it("renders only for the final step and keeps help separate from the checkbox", () => {
    const onUpdate = vi.fn();
    render(
      <CompleteTaskOnEnterToggle
        step={step({ complete_task_on_enter: true })}
        savedStep={step({ complete_task_on_enter: true })}
        onUpdate={onUpdate}
        readOnly={false}
        isFinalStep
      />,
    );

    const checkbox = screen.getByTestId(COMPLETE_TASK_CHECKBOX_TEST_ID);
    const help = screen.getByTestId(COMPLETE_TASK_HELP_TEST_ID);
    expect(checkbox.getAttribute(ARIA_CHECKED_ATTRIBUTE)).toBe(CHECKED_ARIA_VALUE);
    expect(help).not.toBe(checkbox);
    expect(help.className).toContain("h-11");

    fireEvent.click(help);
    expect(checkbox.getAttribute(ARIA_CHECKED_ATTRIBUTE)).toBe(CHECKED_ARIA_VALUE);
    expect(onUpdate).not.toHaveBeenCalled();

    fireEvent.click(checkbox);
    expect(onUpdate).toHaveBeenCalledWith({ complete_task_on_enter: false });
  });

  it("does not expose the control for a non-final step", () => {
    render(
      <CompleteTaskOnEnterToggle
        step={step({ complete_task_on_enter: true })}
        savedStep={step({ complete_task_on_enter: true })}
        onUpdate={vi.fn()}
        readOnly={false}
        isFinalStep={false}
      />,
    );

    expect(screen.queryByTestId(COMPLETE_TASK_CHECKBOX_TEST_ID)).toBeNull();
    expect(screen.queryByTestId(COMPLETE_TASK_HELP_TEST_ID)).toBeNull();
  });
});
