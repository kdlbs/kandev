import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { createElement } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { ORPHAN_STEP_ID } from "./swimlane-kanban-content";
import {
  getGraph2DisplayState,
  sortGraph2Tasks,
  SwimlaneGraph2Content,
} from "./swimlane-graph2-content";
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
        ToastProvider,
        null,
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

describe("sortGraph2Tasks (AC-UI-PIPELINE-ROW-005.1)", () => {
  it("sorts by displayed-step-index ascending", () => {
    const sorted = sortGraph2Tasks(
      [makeTask("on-done", "done"), makeTask("on-todo", "todo")],
      steps,
    );
    expect(sorted.map((t) => t.id)).toEqual(["on-todo", "on-done"]);
  });

  it("breaks a step-index tie by position ascending, treating an absent position as 0", () => {
    const sorted = sortGraph2Tasks(
      [
        makeTask("second", "todo", 2),
        { ...makeTask("no-position", "todo"), position: undefined } as Task,
        makeTask("first", "todo", 1),
      ],
      steps,
    );
    expect(sorted.map((t) => t.id)).toEqual(["no-position", "first", "second"]);
  });

  it("breaks a step-index and position tie by task id ascending", () => {
    const sorted = sortGraph2Tasks([makeTask("b", "todo", 0), makeTask("a", "todo", 0)], steps);
    expect(sorted.map((t) => t.id)).toEqual(["a", "b"]);
  });

  it("sorts the no-resolvable-current-step group last, after every task with a resolvable step", () => {
    const sorted = sortGraph2Tasks(
      [makeTask("unassigned", ""), makeTask("on-done", "done"), makeTask("on-todo", "todo")],
      steps,
    );
    expect(sorted.map((t) => t.id)).toEqual(["on-todo", "on-done", "unassigned"]);
  });

  it("breaks a tie between two or more no-resolvable-current-step tasks by position then id, identically regardless of arrival order", () => {
    const zTask = makeTask("z-task", "", 5);
    const aTask = makeTask("a-task", "", 1);

    const order1 = sortGraph2Tasks([zTask, aTask], steps);
    const order2 = sortGraph2Tasks([aTask, zTask], steps);

    expect(order1.map((t) => t.id)).toEqual(["a-task", "z-task"]);
    expect(order2.map((t) => t.id)).toEqual(["a-task", "z-task"]);
  });

  it("does not mutate the input array", () => {
    const input = [makeTask("on-done", "done"), makeTask("on-todo", "todo")];
    const originalOrder = input.map((t) => t.id);
    sortGraph2Tasks(input, steps);
    expect(input.map((t) => t.id)).toEqual(originalOrder);
  });
});

describe("SwimlaneGraph2Content — row order follows the deterministic sort", () => {
  function renderPipeline(tasks: Task[]) {
    return render(
      createElement(
        ToastProvider,
        null,
        createElement(
          StateProvider,
          null,
          createElement(TooltipProvider, {
            delayDuration: 0,
            children: createElement(SwimlaneGraph2Content, {
              workflowId: "wf-1",
              steps,
              moveTargetSteps: steps,
              tasks,
              onPreviewTask: () => undefined,
              onOpenTask: () => undefined,
              onEditTask: () => undefined,
              onDeleteTask: () => undefined,
            }),
          }),
        ),
      ),
    );
  }

  it("renders rows in displayed-step-index order with the unassigned group last", () => {
    const { container } = renderPipeline([
      makeTask("unassigned", ""),
      makeTask("on-done", "done"),
      makeTask("on-todo", "todo"),
    ]);

    const rowIds = Array.from(container.querySelectorAll('[data-testid^="pipeline-task-"]')).map(
      (el) => el.getAttribute("data-testid"),
    );
    expect(rowIds).toEqual([
      "pipeline-task-on-todo",
      "pipeline-task-on-done",
      "pipeline-task-unassigned",
    ]);
  });
});
