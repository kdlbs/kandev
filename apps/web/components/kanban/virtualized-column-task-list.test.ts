import { describe, expect, it } from "vitest";
import {
  computeInsertionEdge,
  computeKeyboardActiveIndex,
  findTaskIndex,
  type KeyboardReorderDraft,
} from "./virtualized-column-task-list";
import type { Task } from "../kanban-card";

function fakeTask(id: string): Task {
  return { id, workflowStepId: "step-1", title: id, position: 0 } as Task;
}

describe("computeInsertionEdge", () => {
  const queuedStartIndex = 2; // "a","b" admitted; "c","d" queued

  it("returns null when nothing is being dragged", () => {
    expect(computeInsertionEdge(queuedStartIndex, null, 1)).toBeNull();
  });

  it("returns null for the dragged card's own row", () => {
    expect(computeInsertionEdge(queuedStartIndex, 1, 1)).toBeNull();
  });

  it("returns 'bottom' when the hovered row is later than the dragged row in the same band", () => {
    expect(computeInsertionEdge(queuedStartIndex, 0, 1)).toBe("bottom");
  });

  it("returns 'top' when the hovered row is earlier than the dragged row in the same band", () => {
    expect(computeInsertionEdge(queuedStartIndex, 1, 0)).toBe("top");
  });

  it("returns null across a band boundary (AC.11 cross-band reject shows no indicator)", () => {
    expect(computeInsertionEdge(queuedStartIndex, 0, 3)).toBeNull();
  });
});

describe("findTaskIndex", () => {
  const orderedTasks = [fakeTask("a"), fakeTask("b"), fakeTask("c")];

  it("returns the task's index", () => {
    expect(findTaskIndex(orderedTasks, "b")).toBe(1);
  });

  it("returns null for a missing or absent id", () => {
    expect(findTaskIndex(orderedTasks, "missing")).toBeNull();
    expect(findTaskIndex(orderedTasks, null)).toBeNull();
    expect(findTaskIndex(orderedTasks, undefined)).toBeNull();
  });
});

describe("computeKeyboardActiveIndex", () => {
  const queuedStartIndex = 2;

  function draft(overrides: Partial<KeyboardReorderDraft> = {}): KeyboardReorderDraft {
    return {
      taskId: "a",
      stepId: "step-1",
      band: "admitted",
      order: ["a", "b"],
      ...overrides,
    };
  }

  it("returns null when there is no draft", () => {
    expect(computeKeyboardActiveIndex("step-1", queuedStartIndex, null)).toBeNull();
  });

  it("returns null when the draft belongs to a different step", () => {
    expect(computeKeyboardActiveIndex("step-2", queuedStartIndex, draft())).toBeNull();
  });

  it("resolves an admitted-band draft to its raw index", () => {
    expect(
      computeKeyboardActiveIndex("step-1", queuedStartIndex, draft({ order: ["b", "a"] })),
    ).toBe(1);
  });

  it("offsets a queued-band draft by queuedStartIndex", () => {
    const queuedDraft = draft({ taskId: "c", band: "queued", order: ["d", "c"] });
    expect(computeKeyboardActiveIndex("step-1", queuedStartIndex, queuedDraft)).toBe(3);
  });
});
