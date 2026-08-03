import { describe, expect, it } from "vitest";
import { reorderAdmittedInStep, resolveKanbanDropStepId } from "./reorder-admitted";

const base = [
  {
    id: "a",
    workflowStepId: "step-1",
    position: 0,
    createdAt: "2026-05-02T10:00:00Z",
  },
  {
    id: "b",
    workflowStepId: "step-1",
    position: 0,
    createdAt: "2026-05-01T10:00:00Z",
  },
  {
    id: "q",
    workflowStepId: "step-1",
    position: 0,
    queuedForStepId: "step-1",
    wipAdmitted: false,
  },
  {
    id: "other",
    workflowStepId: "step-2",
    position: 0,
  },
];

describe("reorderAdmittedInStep", () => {
  it("densifies admitted cards after moving newer card below older", () => {
    // Board order with position 0: a (newer) above b (older).
    const result = reorderAdmittedInStep(base, "step-1", "a", "b");
    expect(result).not.toBeNull();
    // b already had position 0; only a needs a persist patch.
    expect(result!.patches).toEqual([{ taskId: "a", workflowStepId: "step-1", position: 1 }]);
    const byId = Object.fromEntries(result!.tasks.map((task) => [task.id, task.position]));
    expect(byId).toMatchObject({ a: 1, b: 0, q: 0 });
  });

  it("does not reorder when active card is queued overflow", () => {
    expect(reorderAdmittedInStep(base, "step-1", "q", "a")).toBeNull();
  });

  it("treats a queued overflow over-target as end of the admitted list", () => {
    const result = reorderAdmittedInStep(base, "step-1", "a", "q");
    expect(result).not.toBeNull();
    const admitted = result!.tasks
      .filter((task) => task.workflowStepId === "step-1" && !task.queuedForStepId)
      .sort((left, right) => (left.position ?? 0) - (right.position ?? 0));
    expect(admitted.map((task) => task.id)).toEqual(["b", "a"]);
  });

  it("no-ops when dropping on self", () => {
    expect(reorderAdmittedInStep(base, "step-1", "a", "a")).toBeNull();
  });

  it("moves to end when over is the column id", () => {
    const result = reorderAdmittedInStep(base, "step-1", "a", "step-1");
    expect(result).not.toBeNull();
    expect(result!.patches.map((patch) => patch.taskId)).toContain("a");
    const admitted = result!.tasks
      .filter((task) => task.workflowStepId === "step-1" && !task.queuedForStepId)
      .sort((left, right) => (left.position ?? 0) - (right.position ?? 0));
    expect(admitted.map((task) => task.id)).toEqual(["b", "a"]);
  });
});

describe("resolveKanbanDropStepId", () => {
  it("resolves step ids and task ids", () => {
    const stepIds = new Set(["step-1", "step-2"]);
    expect(resolveKanbanDropStepId("step-2", base, stepIds)).toBe("step-2");
    expect(resolveKanbanDropStepId("a", base, stepIds)).toBe("step-1");
    expect(resolveKanbanDropStepId("missing", base, stepIds)).toBeNull();
  });
});
