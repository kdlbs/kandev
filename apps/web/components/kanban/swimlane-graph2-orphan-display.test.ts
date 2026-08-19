import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { createElement } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { ORPHAN_STEP_ID } from "./swimlane-kanban-content";
import { getGraph2DisplayState, SwimlaneGraph2Content } from "./swimlane-graph2-content";
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

afterEach(cleanup);

describe("getGraph2DisplayState", () => {
  it("adds a Needs Reassignment pipeline step for tasks on deleted workflow steps", () => {
    const { displayTasks, displaySteps } = getGraph2DisplayState(
      [makeTask("valid", "todo"), makeTask("orphan", "deleted-step")],
      steps,
      "Needs Reassignment",
    );

    expect(displaySteps.map((step) => step.title)).toEqual(["Todo", "Done", "Needs Reassignment"]);
    expect(displayTasks.find((task) => task.id === "valid")?.workflowStepId).toBe("todo");
    expect(displayTasks.find((task) => task.id === "orphan")?.workflowStepId).toBe(ORPHAN_STEP_ID);
  });

  it("keeps the pipeline unchanged when every task references a rendered step", () => {
    const { displayTasks, displaySteps } = getGraph2DisplayState(
      [makeTask("valid", "todo"), makeTask("finished", "done")],
      steps,
      "Needs Reassignment",
    );

    expect(displaySteps).toBe(steps);
    expect(displayTasks.map((task) => task.workflowStepId)).toEqual(["todo", "done"]);
  });
});

describe("SwimlaneGraph2Content auto-hidden steps", () => {
  function renderPipeline(
    visibleSteps: WorkflowStep[],
    moveTargetSteps: WorkflowStep[],
    tasks: Task[],
  ) {
    return render(
      createElement(
        StateProvider,
        null,
        createElement(TooltipProvider, {
          delayDuration: 0,
          children: createElement(SwimlaneGraph2Content, {
            workflowId: "wf-1",
            steps: visibleSteps,
            moveTargetSteps,
            tasks,
            onPreviewTask: () => undefined,
            onOpenTask: () => undefined,
            onEditTask: () => undefined,
            onDeleteTask: () => undefined,
          }),
        }),
      ),
    );
  }

  it("keeps auto-hidden stages out of the rendered pipeline", () => {
    const doing = { id: "doing", title: "Doing", color: "#3b82f6" };
    renderPipeline(
      [doing],
      [
        { id: "todo", title: "Todo", color: "#64748b" },
        doing,
        { id: "done", title: "Done", color: "#22c55e" },
      ],
      [makeTask("task-1", "doing")],
    );

    expect(screen.getByText("Doing")).toBeTruthy();
    expect(screen.queryByText("Todo")).toBeNull();
    expect(screen.queryByText("Done")).toBeNull();

    fireEvent.mouseEnter(screen.getByRole("button", { name: "Doing" }).parentElement!);
    expect(screen.getByRole("button", { name: "Move to Todo" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Move to Done" })).toBeTruthy();
  });

  it("explains when every empty pipeline stage is auto-hidden", () => {
    renderPipeline([], steps, []);

    expect(screen.getByTestId("pipeline-auto-hidden-empty-state")).toBeTruthy();
    expect(screen.queryByText("No tasks")).toBeNull();
  });
});
