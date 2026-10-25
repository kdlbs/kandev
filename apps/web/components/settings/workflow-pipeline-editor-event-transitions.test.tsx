import { cleanup, render, renderHook, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { WorkflowStep } from "@/lib/types/http";
import { workflowId as toWorkflowId } from "@/lib/types/ids";
import {
  getEventTransitionTargetStepId,
  getEventTransitionType,
} from "./workflow-pipeline-editor-helpers";
import { EventTransitionSelect, useStepActions } from "./workflow-pipeline-editor-step-actions";

afterEach(cleanup);

const STEP_ID = "accept-step";
const DONE_STEP_ID = "done-step";
const MOVE_TO_STEP = "move_to_step";

function step(overrides: Partial<WorkflowStep> = {}): WorkflowStep {
  return {
    id: STEP_ID,
    workflow_id: toWorkflowId("workflow-1"),
    name: "Acceptance",
    position: 1,
    color: "bg-yellow-500",
    events: {},
    created_at: "",
    updated_at: "",
    ...overrides,
  };
}

const doneStep = step({ id: DONE_STEP_ID, name: "Done", position: 2, color: "bg-green-500" });

describe("event transition helpers", () => {
  it("reads the PR merged transition and its target step", () => {
    const current = step({
      events: { on_pr_merged: [{ type: MOVE_TO_STEP, config: { step_id: DONE_STEP_ID } }] },
    });

    expect(getEventTransitionType(current, "on_pr_merged")).toBe(MOVE_TO_STEP);
    expect(getEventTransitionTargetStepId(current, "on_pr_merged")).toBe(DONE_STEP_ID);
    expect(getEventTransitionType(current, "on_pr_closed")).toBe("none");
    expect(getEventTransitionTargetStepId(current, "on_pr_closed")).toBe("");
  });
});

describe("useStepActions.setEventTransition", () => {
  it("writes a move_to_step action under the selected PR event only", () => {
    const onUpdate = vi.fn();
    const current = step({
      events: {
        on_children_completed: [{ type: "move_to_next" }],
        on_pr_merged: [{ type: "auto_start_agent" }],
      },
    });
    const { result } = renderHook(() => useStepActions({ step: current, onUpdate }));

    result.current.setEventTransition("on_pr_merged", MOVE_TO_STEP, DONE_STEP_ID);

    expect(onUpdate).toHaveBeenCalledWith({
      events: {
        on_children_completed: [{ type: "move_to_next" }],
        on_pr_merged: [
          { type: "auto_start_agent" },
          { type: MOVE_TO_STEP, config: { step_id: DONE_STEP_ID } },
        ],
      },
    });
  });

  it("clears only the transition when set to none", () => {
    const onUpdate = vi.fn();
    const current = step({
      events: { on_pr_closed: [{ type: "move_to_previous" }, { type: "auto_start_agent" }] },
    });
    const { result } = renderHook(() => useStepActions({ step: current, onUpdate }));

    result.current.setEventTransition("on_pr_closed", "none");

    expect(onUpdate).toHaveBeenCalledWith({
      events: { on_pr_closed: [{ type: "auto_start_agent" }] },
    });
  });

  it("ignores move_to_step without a target step", () => {
    const onUpdate = vi.fn();
    const { result } = renderHook(() => useStepActions({ step: step(), onUpdate }));

    result.current.setEventTransition("on_pr_merged", MOVE_TO_STEP);

    expect(onUpdate).not.toHaveBeenCalled();
  });
});

describe("EventTransitionSelect", () => {
  it("renders the PR merged transition with its target step select", () => {
    const current = step({
      events: { on_pr_merged: [{ type: MOVE_TO_STEP, config: { step_id: DONE_STEP_ID } }] },
    });
    render(
      <EventTransitionSelect
        step={current}
        savedStep={current}
        otherSteps={[doneStep]}
        onUpdate={vi.fn()}
        eventKey="on_pr_merged"
        testIdSegment="pr-merged"
        label="When PR Merged"
        help="help"
        helpAria="How PR merge transitions work"
        setEventTransition={vi.fn()}
        readOnly={false}
      />,
    );

    expect(screen.getByTestId(`${STEP_ID}-pr-merged-transition-select`)).toBeTruthy();
    expect(screen.getByTestId(`${STEP_ID}-pr-merged-step-select`).textContent).toContain("Done");
    expect(screen.getByTestId(`${STEP_ID}-pr-merged-help`).getAttribute("aria-label")).toBe(
      "How PR merge transitions work",
    );
  });
});
