import { describe, expect, it } from "vitest";

import type { Workflow, WorkflowStep } from "@/lib/types/http";
import {
  buildWorkflowEditorViewModel,
  repairWorkflowEditorSelection,
} from "./workflow-editor-view-model";

const workflow = {
  id: "wf-1",
  workspace_id: "ws-1",
  name: "Delivery",
  agent_profile_id: "profile-default",
  created_at: "",
  updated_at: "",
} as Workflow;

function step(id: string, name: string, position: number, overrides: Partial<WorkflowStep> = {}) {
  return {
    id,
    workflow_id: workflow.id,
    name,
    position,
    color: "bg-slate-500",
    allow_manual_move: true,
    created_at: "",
    updated_at: "",
    ...overrides,
  } as WorkflowStep;
}

describe("workflow editor view model", () => {
  it("derives ordered summaries, profile inheritance, destination, dirty state, and issues", () => {
    const saved = [
      step("one", "Todo", 0),
      step("two", "Review", 1, {
        events: { on_turn_complete: [{ type: "move_to_step", config: { step_id: "missing" } }] },
      }),
    ];
    const draft = [
      saved[0],
      step("two", "Review", 1, {
        agent_profile_id: "profile-review",
        events: {
          on_enter: [
            { type: "run_script", config: { command: "" } },
            { type: "run_script", config: { command: "echo ready" } },
          ],
          on_turn_complete: [{ type: "move_to_next" }],
          on_children_completed: [{ type: "move_to_step", config: { step_id: "one" } }],
        },
      }),
    ];

    const model = buildWorkflowEditorViewModel(workflow, draft, workflow, saved);
    expect(model.stepSummaries).toHaveLength(2);
    expect(model.stepSummaries[1]).toMatchObject({
      stepId: "two",
      effectiveProfileId: "profile-review",
      actionCount: 4,
      primaryDestinationId: undefined,
      isDirty: true,
    });
    expect(model.stepSummaries[1].issues).toEqual([
      expect.objectContaining({
        stepId: "two",
        trigger: "on_enter",
        actionIndex: 0,
        target: "action",
      }),
    ]);
    expect(model.workflowDirty).toBe(false);
    expect(model.edges).toEqual([]);
  });

  it("projects next and explicit transitions as ordered pipeline edges", () => {
    const steps = [
      step("one", "Todo", 0, { events: { on_turn_complete: [{ type: "move_to_next" }] } }),
      step("two", "Review", 1, {
        events: { on_turn_complete: [{ type: "move_to_step", config: { step_id: "one" } }] },
      }),
    ];
    expect(buildWorkflowEditorViewModel(workflow, steps).edges).toEqual([
      { fromStepId: "one", toStepId: "two", trigger: "on_turn_complete" },
      { fromStepId: "two", toStepId: "one", trigger: "on_turn_complete" },
    ]);
  });

  it("repairs a missing selection to the nearest remaining step", () => {
    expect(repairWorkflowEditorSelection("removed", [step("one", "Todo", 0)])).toBe("one");
    expect(repairWorkflowEditorSelection("two", [])).toBeNull();
    expect(repairWorkflowEditorSelection("one", [step("one", "Todo", 0)])).toBe("one");
  });
});
