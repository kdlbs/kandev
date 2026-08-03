import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { WorkflowStep } from "@/lib/types/http";
import { workflowId as toWorkflowId } from "@/lib/types/ids";
import { TurnCompleteSelect } from "./workflow-pipeline-editor-step-actions";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

afterEach(cleanup);

function step(overrides: Partial<WorkflowStep> = {}): WorkflowStep {
  return {
    id: "step-1",
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
    const checkbox = screen.getByTestId("step-1-cancel-completion-checkbox");

    expect(checkbox.getAttribute("aria-checked")).toBe("false");
    // This file stubs react-i18next with an identity `t`, so keys render as
    // keys. `HelpTip`'s default aria-label is now a catalog key rather than a
    // hardcoded string, so the expectation follows the same convention as the
    // queryByText below.
    expect(screen.getByTestId("step-1-cancel-completion-help").getAttribute("aria-label")).toBe(
      "workflows:moreInformation",
    );
    expect(screen.queryByText("workflows:runCompletionActionsWhenTurnCancelledHelp")).toBeNull();
    fireEvent.click(checkbox);
    expect(onUpdate).toHaveBeenCalledWith({ cancel_triggers_turn_complete: true });
  });

  it("preserves the persisted value and disables it in read-only mode", () => {
    renderTurnComplete(step({ cancel_triggers_turn_complete: true }), true);
    const checkbox = screen.getByTestId("step-1-cancel-completion-checkbox");

    expect(checkbox.getAttribute("aria-checked")).toBe("true");
    expect((checkbox as HTMLButtonElement).disabled).toBe(true);
  });

  it("does not render the subordinate policy when no completion transition is configured", () => {
    renderTurnComplete(
      step({ events: { on_turn_complete: [] }, cancel_triggers_turn_complete: true }),
    );

    expect(screen.queryByTestId("step-1-cancel-completion-checkbox")).toBeNull();
  });
});
