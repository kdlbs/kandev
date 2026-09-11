import { describe, expect, it } from "vitest";
import { classifyDrop } from "./drop-classification";

const STEP_A = "step-a";
const STEP_B = "step-b";

type FakeTask = {
  id: string;
  workflowStepId: string;
  position: number;
  wipAdmitted?: boolean;
  queuedForStepId?: string;
};

function admitted(id: string, position: number, stepId = STEP_A): FakeTask {
  return { id, workflowStepId: stepId, position, wipAdmitted: true };
}

function queued(id: string, position: number, stepId = STEP_A): FakeTask {
  return { id, workflowStepId: stepId, position, wipAdmitted: false, queuedForStepId: stepId };
}

describe("classifyDrop", () => {
  it("classifies a same-band drop as a reorder with the moved visible order (AC.5)", () => {
    const stepTasks = [admitted("a", 0), admitted("b", 1), admitted("c", 2)];

    const result = classifyDrop({
      draggedTaskId: "a",
      overId: "c",
      stepTasks,
      allTasks: stepTasks,
    });

    expect(result).toEqual({
      kind: "reorder",
      stepId: STEP_A,
      band: "admitted",
      visibleOrderAfterMove: ["b", "c", "a"],
    });
  });

  it("classifies a queued-band drop the same way as admitted", () => {
    const stepTasks = [admitted("a", 0), queued("b", 1), queued("c", 2)];

    const result = classifyDrop({
      draggedTaskId: "b",
      overId: "c",
      stepTasks,
      allTasks: stepTasks,
    });

    expect(result).toEqual({
      kind: "reorder",
      stepId: STEP_A,
      band: "queued",
      visibleOrderAfterMove: ["c", "b"],
    });
  });

  it("rejects a drop onto the other band of the same step, without a request (AC.11)", () => {
    const stepTasks = [admitted("a", 0), queued("b", 1)];

    const result = classifyDrop({
      draggedTaskId: "a",
      overId: "b",
      stepTasks,
      allTasks: stepTasks,
    });

    expect(result).toEqual({ kind: "reject" });
  });

  it("is a no-op when the drop reproduces the order already displayed (AC.10)", () => {
    const stepTasks = [admitted("a", 0), admitted("b", 1)];

    // Dropping "a" back onto itself.
    const result = classifyDrop({
      draggedTaskId: "a",
      overId: "a",
      stepTasks,
      allTasks: stepTasks,
    });

    expect(result).toEqual({ kind: "no-op" });
  });

  it("is a no-op when there is no drop target (dropped outside any band, AC.9)", () => {
    const stepTasks = [admitted("a", 0), admitted("b", 1)];

    const result = classifyDrop({
      draggedTaskId: "a",
      overId: null,
      stepTasks,
      allTasks: stepTasks,
    });

    expect(result).toEqual({ kind: "no-op" });
  });

  it("is a no-op when dropped on the column itself with no specific card underneath", () => {
    const stepTasks = [admitted("a", 0), admitted("b", 1)];

    const result = classifyDrop({
      draggedTaskId: "a",
      overId: STEP_A,
      stepTasks,
      allTasks: stepTasks,
    });

    expect(result).toEqual({ kind: "no-op" });
  });

  it("classifies a drop onto a different step's column as cross-step (AC.13)", () => {
    const stepTasks = [admitted("a", 0), admitted("b", 1)];

    const result = classifyDrop({
      draggedTaskId: "a",
      overId: STEP_B,
      stepTasks,
      allTasks: stepTasks,
    });

    expect(result).toEqual({ kind: "cross-step", targetStepId: STEP_B });
  });

  it("is a no-op when the dragged task cannot be found", () => {
    const stepTasks = [admitted("a", 0), admitted("b", 1)];

    const result = classifyDrop({
      draggedTaskId: "missing",
      overId: "b",
      stepTasks,
      allTasks: stepTasks,
    });

    expect(result).toEqual({ kind: "no-op" });
  });
});

describe("classifyDrop — band computed from position (AC.15)", () => {
  it("classifies a same-band drop by position, not by stepTasks' array order", () => {
    // stepTasks arrives in creation order (as it would after a prior reorder
    // changed positions but arrival order stayed the same) — true step order
    // by position is c(0), a(1), b(2), not the array's own a, b, c order.
    const stepTasks = [admitted("a", 1), admitted("b", 2), admitted("c", 0)];

    const result = classifyDrop({
      draggedTaskId: "a",
      overId: "b",
      stepTasks,
      allTasks: stepTasks,
    });

    expect(result).toEqual({
      kind: "reorder",
      stepId: STEP_A,
      band: "admitted",
      visibleOrderAfterMove: ["c", "b", "a"],
    });
  });
});

describe("classifyDrop — cross-step target resolution (AC.13)", () => {
  it("classifies a drop onto a card that belongs to a different step as cross-step, resolving the target from the whole board rather than the dragged task's own step", () => {
    // `stepTasks` is scoped to the dragged task's own step, exactly as the
    // real caller (`useSwimlaneKanbanDnd`) produces it — it never contains a
    // foreign-step card. Only `allTasks` (the whole board) has "x".
    const stepTasks = [admitted("a", 0)];
    const allTasks = [...stepTasks, admitted("x", 0, STEP_B)];

    const result = classifyDrop({ draggedTaskId: "a", overId: "x", stepTasks, allTasks });

    expect(result).toEqual({ kind: "cross-step", targetStepId: STEP_B });
  });

  it("treats an overId that matches no task anywhere on the board as a literal step id (dropped on an empty column)", () => {
    const stepTasks = [admitted("a", 0)];

    const result = classifyDrop({
      draggedTaskId: "a",
      overId: STEP_B,
      stepTasks,
      allTasks: stepTasks,
    });

    expect(result).toEqual({ kind: "cross-step", targetStepId: STEP_B });
  });
});
