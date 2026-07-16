import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import type { WorkflowStep } from "@/lib/types/http";
import type { WorkflowReplayCycleDiagnostic } from "@/lib/workflows/replay-cycle-analysis";
import { WorkflowPipelineEditor } from "./workflow-pipeline-editor";
import { WorkflowCycleDiagnostic, WorkflowCycleGuardDialog } from "./workflow-cycle-diagnostic";

afterEach(cleanup);

const AUTO_START_STEP_NAME = "In Progress";

const diagnostic: WorkflowReplayCycleDiagnostic = {
  identity: "cycle-work-review",
  severity: "warning",
  autoStartStepId: "work",
  autoStartStepName: AUTO_START_STEP_NAME,
  affectedStepIds: ["work", "review", "done"],
  trace: [
    {
      sourceStepId: "work",
      sourceStepName: AUTO_START_STEP_NAME,
      trigger: "on_turn_complete",
      actionKind: "move_to_next",
      destinationStepId: "review",
      destinationStepName: "Review",
      requiresUserInvolvement: false,
    },
    {
      sourceStepId: "review",
      sourceStepName: "Review",
      trigger: "on_turn_start",
      actionKind: "move_to_step",
      destinationStepId: "done",
      destinationStepName: "Done",
      requiresUserInvolvement: true,
    },
    {
      sourceStepId: "done",
      sourceStepName: "Done",
      trigger: "on_turn_complete",
      actionKind: "move_to_previous",
      destinationStepId: "work",
      destinationStepName: AUTO_START_STEP_NAME,
      requiresUserInvolvement: true,
    },
  ],
  promptSource: "task_description",
};

function workflowStep(id: string, name: string, position: number): WorkflowStep {
  return {
    id,
    workflow_id: "workflow-1",
    name,
    position,
    color: "bg-slate-500",
    created_at: "2026-07-15T00:00:00.000Z",
    updated_at: "2026-07-15T00:00:00.000Z",
  } as WorkflowStep;
}

describe("WorkflowCycleDiagnostic", () => {
  it("renders severity, the exact ordered trace, user-required hops, and prompt source", () => {
    render(<WorkflowCycleDiagnostic diagnostic={diagnostic} />);

    expect(screen.getByText("Potential repeated agent run")).toBeTruthy();
    const hops = screen.getAllByRole("listitem");
    expect(hops).toHaveLength(3);
    expect(hops[0].textContent).toContain("In Progresson_turn_completemove_to_nextReview");
    expect(hops[1].textContent).toContain("Reviewon_turn_startmove_to_stepDone");
    expect(hops[2].textContent).toContain("Doneon_turn_completemove_to_previousIn Progress");
    expect(within(hops[0]).queryByText("User action required")).toBeNull();
    expect(within(hops[1]).getByText("User action required")).toBeTruthy();
    expect(within(hops[2]).getByText("User action required")).toBeTruthy();
    expect(
      screen.getByText(/has no step prompt, so re-entering it sends the task description/i),
    ).toBeTruthy();
  });

  it.each([
    [
      "step_prompt_with_task_description" as const,
      /rendered step prompt including the task description/i,
    ],
    ["step_prompt" as const, /step prompt instead of the task description/i],
  ])("explains the %s prompt source", (promptSource, expected) => {
    render(<WorkflowCycleDiagnostic diagnostic={{ ...diagnostic, promptSource }} />);
    expect(screen.getByText(expected)).toBeTruthy();
  });

  it("renders blocking severity separately from a warning", () => {
    render(<WorkflowCycleDiagnostic diagnostic={{ ...diagnostic, severity: "blocking" }} />);
    expect(screen.getByText("Automatic workflow cycle")).toBeTruthy();
    expect(screen.getByText(/without another user action/i)).toBeTruthy();
  });
});

describe("WorkflowCycleGuardDialog", () => {
  it("offers no override for a blocking proposal", () => {
    const onCancel = vi.fn();
    render(
      <WorkflowCycleGuardDialog
        proposal={{
          diagnostics: [{ ...diagnostic, severity: "blocking" }],
          intent: "apply",
          severity: "blocking",
        }}
        onCancel={onCancel}
        onConfirm={vi.fn()}
      />,
    );

    expect(screen.getByRole("heading", { name: "Workflow cycle blocked" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: /anyway/i })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Return to workflow" }));
    expect(onCancel).toHaveBeenCalledOnce();
  });

  it.each([
    ["apply" as const, "Apply anyway"],
    ["create" as const, "Create anyway"],
  ])("confirms a warning proposal with %s intent", (intent, actionLabel) => {
    const onConfirm = vi.fn();
    const onCancel = vi.fn();
    render(
      <WorkflowCycleGuardDialog
        proposal={{ diagnostics: [diagnostic], intent, severity: "warning" }}
        onCancel={onCancel}
        onConfirm={onConfirm}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: actionLabel }));
    expect(onConfirm).toHaveBeenCalledOnce();
    expect(onCancel).not.toHaveBeenCalled();
    expect(
      screen.getByRole("button", { name: actionLabel }).hasAttribute("data-dialog-default-action"),
    ).toBe(true);
    expect(screen.getByRole("button", { name: "Cancel" })).toBeTruthy();
  });

  it("uses the semantic dialog action for Enter confirmation", () => {
    const onConfirm = vi.fn();
    render(
      <WorkflowCycleGuardDialog
        proposal={{ diagnostics: [diagnostic], intent: "apply", severity: "warning" }}
        onCancel={vi.fn()}
        onConfirm={onConfirm}
      />,
    );

    fireEvent.keyDown(screen.getByTestId("workflow-cycle-guard-dialog"), { key: "Enter" });
    expect(onConfirm).toHaveBeenCalledOnce();
  });
});

describe("WorkflowPipelineEditor replay-cycle marker", () => {
  it("marks affected nodes with visible and accessible text", () => {
    render(
      <WorkflowPipelineEditor
        steps={[workflowStep("work", AUTO_START_STEP_NAME, 0), workflowStep("review", "Review", 1)]}
        diagnostics={[diagnostic]}
        onUpdateStep={() => {}}
        onAddStep={() => {}}
        onRemoveStep={() => {}}
        onReorderSteps={() => {}}
      />,
    );

    expect(screen.getByLabelText("In Progress is part of a replay cycle")).toBeTruthy();
    expect(screen.getByLabelText("Review is part of a replay cycle")).toBeTruthy();
    expect(screen.getAllByText("Cycle")).toHaveLength(2);
  });
});
