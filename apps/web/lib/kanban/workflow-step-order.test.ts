import { describe, expect, it } from "vitest";
import { mergeStepOrderRevisions, sortWorkflowStepsByPosition } from "./workflow-step-order";

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

describe("mergeStepOrderRevisions", () => {
  it("seeds a revision for a step with no prior recorded value", () => {
    const result = mergeStepOrderRevisions({}, [{ id: "step-1", order_revision: 3 }]);
    expect(result).toEqual({ "step-1": 3 });
  });

  it("advances an existing revision when the fetched value is greater", () => {
    const result = mergeStepOrderRevisions({ "step-1": 2 }, [{ id: "step-1", order_revision: 5 }]);
    expect(result).toEqual({ "step-1": 5 });
  });

  it("never rolls back a fresher recorded revision to a stale fetched one", () => {
    const current = { "step-1": 7 };
    const result = mergeStepOrderRevisions(current, [{ id: "step-1", order_revision: 4 }]);
    expect(result).toEqual({ "step-1": 7 });
  });

  it("returns the same reference when nothing changes, to avoid extra re-renders", () => {
    const current = { "step-1": 7 };
    const result = mergeStepOrderRevisions(current, [{ id: "step-1", order_revision: 4 }]);
    expect(result).toBe(current);
  });

  it("ignores a step whose order_revision is absent", () => {
    const result = mergeStepOrderRevisions({}, [{ id: "step-1" }]);
    expect(result).toEqual({});
  });

  it("passes through an empty or undefined steps list unchanged", () => {
    const current = { "step-1": 1 };
    expect(mergeStepOrderRevisions(current, [])).toBe(current);
    expect(mergeStepOrderRevisions(current, undefined)).toBe(current);
  });
});
