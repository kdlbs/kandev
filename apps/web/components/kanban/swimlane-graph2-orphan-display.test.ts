import { describe, expect, it } from "vitest";
import { ORPHAN_STEP_ID } from "./swimlane-kanban-content";
import { getGraph2DisplayState } from "./swimlane-graph2-content";
import type { Task } from "@/components/kanban-card";
import type { WorkflowStep } from "@/components/kanban-column";

function makeTask(id: string, stepId: string, position = 0): Task {
  return {
    id,
    title: id,
    workflowStepId: stepId,
    position,
  } as Task;
}

const steps: WorkflowStep[] = [
  { id: "todo", title: "Todo", color: "#64748b" },
  { id: "done", title: "Done", color: "#22c55e" },
];

describe("getGraph2DisplayState", () => {
  it("adds a Needs Reassignment pipeline step for tasks on deleted workflow steps", () => {
    const { displayTasks, displaySteps } = getGraph2DisplayState(
      [makeTask("valid", "todo"), makeTask("orphan", "deleted-step")],
      steps,
    );

    expect(displaySteps.map((step) => step.title)).toEqual(["Todo", "Done", "Needs Reassignment"]);
    expect(displayTasks.find((task) => task.id === "valid")?.workflowStepId).toBe("todo");
    expect(displayTasks.find((task) => task.id === "orphan")?.workflowStepId).toBe(ORPHAN_STEP_ID);
  });

  it("keeps the pipeline unchanged when every task references a rendered step", () => {
    const { displayTasks, displaySteps } = getGraph2DisplayState(
      [makeTask("valid", "todo"), makeTask("finished", "done")],
      steps,
    );

    expect(displaySteps).toBe(steps);
    expect(displayTasks.map((task) => task.workflowStepId)).toEqual(["todo", "done"]);
  });
});
