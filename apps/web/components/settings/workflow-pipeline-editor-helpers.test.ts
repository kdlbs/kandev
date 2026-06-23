import { describe, expect, it } from "vitest";
import type { WorkflowStep } from "@/lib/types/http";
import { getChildrenCompletedTransitionType } from "./workflow-pipeline-editor-helpers";

describe("workflow pipeline editor helpers", () => {
  it("reads the all child tasks complete transition from workflow step events", () => {
    const step = {
      events: {
        on_children_completed: [{ type: "move_to_step", config: { step_id: "done-step" } }],
      },
    } as WorkflowStep;

    expect(getChildrenCompletedTransitionType(step)).toBe("move_to_step");
  });

  it("defaults all child tasks complete to none when no transition is configured", () => {
    expect(getChildrenCompletedTransitionType({ events: {} } as WorkflowStep)).toBe("none");
  });
});
