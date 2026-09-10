import { describe, expect, it } from "vitest";
import { sortWorkflowStepsByPosition } from "./workflow-step-order";

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
