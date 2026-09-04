import { describe, expect, it } from "vitest";
import { computeInsertionEdge } from "./virtualized-column-task-list";
import type { Task } from "../kanban-card";

function fakeTask(id: string): Task {
  return { id, workflowStepId: "step-1", title: id, position: 0 } as Task;
}

describe("computeInsertionEdge", () => {
  const orderedTasks = [fakeTask("a"), fakeTask("b"), fakeTask("c"), fakeTask("d")];
  const queuedStartIndex = 2; // "a","b" admitted; "c","d" queued

  it("returns null when nothing is being dragged", () => {
    expect(computeInsertionEdge(orderedTasks, queuedStartIndex, null, 1)).toBeNull();
  });

  it("returns null for the dragged card's own row", () => {
    expect(computeInsertionEdge(orderedTasks, queuedStartIndex, "b", 1)).toBeNull();
  });

  it("returns 'bottom' when the hovered row is later than the dragged row in the same band", () => {
    expect(computeInsertionEdge(orderedTasks, queuedStartIndex, "a", 1)).toBe("bottom");
  });

  it("returns 'top' when the hovered row is earlier than the dragged row in the same band", () => {
    expect(computeInsertionEdge(orderedTasks, queuedStartIndex, "b", 0)).toBe("top");
  });

  it("returns null across a band boundary (AC.11 cross-band reject shows no indicator)", () => {
    expect(computeInsertionEdge(orderedTasks, queuedStartIndex, "a", 3)).toBeNull();
  });

  it("returns null when the dragged task is not in this step's ordered tasks", () => {
    expect(computeInsertionEdge(orderedTasks, queuedStartIndex, "missing", 1)).toBeNull();
  });
});
