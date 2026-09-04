import { describe, expect, it } from "vitest";
import { sortGraph2Tasks } from "./swimlane-graph2-content";
import type { Task } from "@/components/kanban-card";
import type { WorkflowStep } from "@/components/kanban-column";

function makeTask(overrides: Partial<Task> & { id: string; workflowStepId: string }): Task {
  return {
    title: overrides.id,
    position: 0,
    ...overrides,
  } as Task;
}

const steps: WorkflowStep[] = [
  { id: "todo", title: "Todo", color: "#64748b" },
  { id: "done", title: "Done", color: "#22c55e" },
];

describe("sortGraph2Tasks", () => {
  it("orders rows by step index first", () => {
    const tasks = [
      makeTask({ id: "b", workflowStepId: "done" }),
      makeTask({ id: "a", workflowStepId: "todo" }),
    ];
    expect(sortGraph2Tasks(tasks, steps).map((t) => t.id)).toEqual(["a", "b"]);
  });

  it("breaks a same-step position tie using the full AC.1 order rather than bare position", () => {
    // REQ-TASKS-KANBAN-TASK-REORDERING-001.2/.38: bare `position` is not a
    // total order, so two tasks that arrived together (same position) must
    // still resolve deterministically via priority, then queuedAt/createdAt,
    // then id — not fall back to whatever order they arrived in the array.
    const tasks = [
      makeTask({
        id: "low",
        workflowStepId: "todo",
        position: 1,
        priority: "low",
        createdAt: "2026-08-12T09:00:00Z",
      }),
      makeTask({
        id: "critical",
        workflowStepId: "todo",
        position: 1,
        priority: "critical",
        createdAt: "2026-08-12T09:00:00Z",
      }),
    ];
    expect(sortGraph2Tasks(tasks, steps).map((t) => t.id)).toEqual(["critical", "low"]);
  });

  it("sends a task on a step outside displaySteps to the front deterministically", () => {
    const tasks = [
      makeTask({ id: "known", workflowStepId: "todo" }),
      makeTask({ id: "unknown-step", workflowStepId: "deleted-step" }),
    ];
    expect(sortGraph2Tasks(tasks, steps).map((t) => t.id)).toEqual(["unknown-step", "known"]);
  });
});
