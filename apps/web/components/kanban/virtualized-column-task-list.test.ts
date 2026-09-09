import { describe, expect, it } from "vitest";
import {
  computeInsertionEdge,
  computeKeyboardInsertionEdge,
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

describe("computeKeyboardInsertionEdge", () => {
  function draft(overrides: Partial<KeyboardReorderDraft> = {}): KeyboardReorderDraft {
    return {
      taskId: "a",
      stepId: "step-1",
      band: "admitted",
      order: ["a", "b", "c", "d"],
      ...overrides,
    };
  }

  it("returns null when there is no draft", () => {
    expect(computeKeyboardInsertionEdge(null, "step-1", "b")).toBeNull();
  });

  it("returns null when the draft belongs to a different step", () => {
    expect(computeKeyboardInsertionEdge(draft(), "step-2", "b")).toBeNull();
  });

  it("returns null for a row not present in the draft's band order", () => {
    expect(computeKeyboardInsertionEdge(draft(), "step-1", "queued-task")).toBeNull();
  });

  it("returns null for the dragged card's own row", () => {
    expect(computeKeyboardInsertionEdge(draft(), "step-1", "a")).toBeNull();
  });

  it("returns 'bottom' on the row immediately before the draft's position", () => {
    expect(
      computeKeyboardInsertionEdge(draft({ order: ["b", "a", "c", "d"] }), "step-1", "b"),
    ).toBe("bottom");
  });

  it("returns 'top' on the row immediately after the draft's position", () => {
    expect(
      computeKeyboardInsertionEdge(draft({ order: ["b", "a", "c", "d"] }), "step-1", "c"),
    ).toBe("top");
  });

  it("tracks the draft's neighbors by identity after multiple moves, not by original DOM index", () => {
    // "a" starts at index 0 ([a,b,c,d]) and is moved down twice, landing
    // between "c" and "d" ([b,c,a,d]). The DOM never re-renders (only the
    // draft evolves), so this proves the indicator follows "a"'s actual
    // neighbors rather than whatever row sits at a numerically-adjacent DOM
    // index (a regression that would flag "b" - "a"'s original DOM
    // neighbor, not its current one).
    const moved = draft({ order: ["b", "c", "a", "d"] });
    expect(computeKeyboardInsertionEdge(moved, "step-1", "b")).toBeNull();
    expect(computeKeyboardInsertionEdge(moved, "step-1", "c")).toBe("bottom");
    expect(computeKeyboardInsertionEdge(moved, "step-1", "d")).toBe("top");
  });
});
