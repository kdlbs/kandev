import { describe, expect, it } from "vitest";
import {
  areAllEmptyStepsAutoHidden,
  deriveAutoHiddenStepIds,
  getAutoHiddenMoveTargets,
  sortWorkflowStepsByPosition,
} from "./auto-hide-empty-columns";

const steps = [{ id: "backlog" }, { id: "doing" }, { id: "done" }];

describe("deriveAutoHiddenStepIds", () => {
  it("hides only unoccupied live steps when enabled", () => {
    const result = deriveAutoHiddenStepIds(steps, [{ workflowStepId: "doing" }], true, []);
    expect([...result]).toEqual(["backlog", "done"]);
  });

  it("returns no automatic hidden steps when disabled", () => {
    expect([...deriveAutoHiddenStepIds(steps, [], false, [])]).toEqual([]);
  });

  it("leaves manually hidden steps to the manual visibility contract", () => {
    const result = deriveAutoHiddenStepIds(steps, [], true, ["backlog"]);
    expect([...result]).toEqual(["doing", "done"]);
  });
});

describe("getAutoHiddenMoveTargets", () => {
  it("returns only non-rendered destinations without changing either input", () => {
    const visible = [{ id: "backlog" }, { id: "review" }];
    const moveTargets = [...visible, { id: "done" }];

    expect(getAutoHiddenMoveTargets(visible, moveTargets)).toEqual([{ id: "done" }]);
    expect(visible.map((step) => step.id)).toEqual(["backlog", "review"]);
    expect(moveTargets.map((step) => step.id)).toEqual(["backlog", "review", "done"]);
  });
});

describe("areAllEmptyStepsAutoHidden", () => {
  it("distinguishes auto-hidden steps from a workflow with no configured steps", () => {
    expect(areAllEmptyStepsAutoHidden([], [{ id: "todo" }])).toBe(true);
    expect(areAllEmptyStepsAutoHidden([], [])).toBe(false);
    expect(areAllEmptyStepsAutoHidden([{ id: "todo" }], [{ id: "todo" }])).toBe(false);
  });
});

describe("sortWorkflowStepsByPosition", () => {
  it("orders workflow steps deterministically without mutating the snapshot", () => {
    const unordered = [
      { id: "step-b", position: 1 },
      { id: "step-c", position: 0 },
      { id: "step-a", position: 1 },
    ];

    expect(sortWorkflowStepsByPosition(unordered).map((step) => step.id)).toEqual([
      "step-c",
      "step-a",
      "step-b",
    ]);
    expect(unordered.map((step) => step.id)).toEqual(["step-b", "step-c", "step-a"]);
  });
});
