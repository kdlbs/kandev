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

    const result = classifyDrop({ draggedTaskId: "a", overId: "c", stepTasks });

    expect(result).toEqual({
      kind: "reorder",
      stepId: STEP_A,
      band: "admitted",
      visibleOrderAfterMove: ["b", "c", "a"],
    });
  });

  it("classifies a queued-band drop the same way as admitted", () => {
    const stepTasks = [admitted("a", 0), queued("b", 1), queued("c", 2)];

    const result = classifyDrop({ draggedTaskId: "b", overId: "c", stepTasks });

    expect(result).toEqual({
      kind: "reorder",
      stepId: STEP_A,
      band: "queued",
      visibleOrderAfterMove: ["c", "b"],
    });
  });

  it("rejects a drop onto the other band of the same step, without a request (AC.11)", () => {
    const stepTasks = [admitted("a", 0), queued("b", 1)];

    const result = classifyDrop({ draggedTaskId: "a", overId: "b", stepTasks });

    expect(result).toEqual({ kind: "reject" });
  });

  it("is a no-op when the drop reproduces the order already displayed (AC.10)", () => {
    const stepTasks = [admitted("a", 0), admitted("b", 1)];

    // Dropping "a" back onto itself.
    const result = classifyDrop({ draggedTaskId: "a", overId: "a", stepTasks });

    expect(result).toEqual({ kind: "no-op" });
  });

  it("is a no-op when there is no drop target (dropped outside any band, AC.9)", () => {
    const stepTasks = [admitted("a", 0), admitted("b", 1)];

    const result = classifyDrop({ draggedTaskId: "a", overId: null, stepTasks });

    expect(result).toEqual({ kind: "no-op" });
  });

  it("is a no-op when dropped on the column itself with no specific card underneath", () => {
    const stepTasks = [admitted("a", 0), admitted("b", 1)];

    const result = classifyDrop({ draggedTaskId: "a", overId: STEP_A, stepTasks });

    expect(result).toEqual({ kind: "no-op" });
  });

  it("classifies a drop onto a different step's column as cross-step (AC.13)", () => {
    const stepTasks = [admitted("a", 0), admitted("b", 1)];

    const result = classifyDrop({ draggedTaskId: "a", overId: STEP_B, stepTasks });

    expect(result).toEqual({ kind: "cross-step", targetStepId: STEP_B });
  });

  it("classifies a drop onto a card that belongs to a different step as cross-step", () => {
    const stepTasks = [admitted("a", 0), { ...admitted("x", 0, STEP_B) }];

    const result = classifyDrop({ draggedTaskId: "a", overId: "x", stepTasks });

    expect(result).toEqual({ kind: "cross-step", targetStepId: STEP_B });
  });

  it("is a no-op when the dragged task cannot be found", () => {
    const stepTasks = [admitted("a", 0), admitted("b", 1)];

    const result = classifyDrop({ draggedTaskId: "missing", overId: "b", stepTasks });

    expect(result).toEqual({ kind: "no-op" });
  });
});
