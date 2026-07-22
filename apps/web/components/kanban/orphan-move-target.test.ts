import { describe, expect, it } from "vitest";
import { ORPHAN_STEP_ID, isOrphanMoveTarget } from "./swimlane-kanban-content";
import { getStepAdjacency } from "./graph2-task-pipeline";
import type { WorkflowStep } from "@/components/kanban-column";

describe("isOrphanMoveTarget", () => {
  it("identifies the synthetic Needs Reassignment sentinel", () => {
    expect(isOrphanMoveTarget(ORPHAN_STEP_ID)).toBe(true);
  });

  it("treats real step ids as valid move targets", () => {
    expect(isOrphanMoveTarget("todo")).toBe(false);
    expect(isOrphanMoveTarget("")).toBe(false);
  });
});

describe("getStepAdjacency", () => {
  const realSteps: WorkflowStep[] = [
    { id: "todo", title: "Todo", color: "#64748b" },
    { id: "in-progress", title: "In Progress", color: "#3b82f6" },
    { id: "done", title: "Done", color: "#22c55e" },
  ];
  const stepsWithOrphan: WorkflowStep[] = [
    ...realSteps,
    { id: ORPHAN_STEP_ID, title: "Needs Reassignment", color: "#f59e0b" },
  ];

  it("offers both neighbors for a middle real step with no orphan present", () => {
    const adjacency = getStepAdjacency(realSteps, 1);
    expect(adjacency).toEqual({
      hasPrev: true,
      prevStepId: "todo",
      hasNext: true,
      nextStepId: "done",
    });
  });

  it("does not offer the orphan sentinel as the next move target", () => {
    // "done" (index 2) is immediately followed by the orphan node (index 3).
    const adjacency = getStepAdjacency(stepsWithOrphan, 2);
    expect(adjacency.hasNext).toBe(false);
    expect(adjacency.nextStepId).toBeUndefined();
    // The real, backward neighbor is still a valid target.
    expect(adjacency.hasPrev).toBe(true);
    expect(adjacency.prevStepId).toBe("in-progress");
  });

  it("still allows an orphaned task to move back onto the last real step", () => {
    // The orphan node itself (index 3) — its prev neighbor is a real step.
    const adjacency = getStepAdjacency(stepsWithOrphan, 3);
    expect(adjacency.hasPrev).toBe(true);
    expect(adjacency.prevStepId).toBe("done");
    expect(adjacency.hasNext).toBe(false);
  });
});
