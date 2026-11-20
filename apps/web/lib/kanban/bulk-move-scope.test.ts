import { describe, expect, it } from "vitest";
import { isBulkMoveTargetInWorkflow, resolveBulkSelectionWorkflow } from "./bulk-move-scope";

const board = { workflowId: null, tasks: [], steps: [] };
const snapshots = {
  a: { workflowId: "a", tasks: [{ id: "a-task", workflowId: "a" }], steps: [{ id: "a-step" }] },
  b: { workflowId: "b", tasks: [{ id: "b-task", workflowId: "b" }], steps: [{ id: "b-step" }] },
};

describe("bulk move scope evidence", () => {
  it("accepts a consistent selection and target", () => {
    expect(resolveBulkSelectionWorkflow(["b-task"], snapshots, board)).toBe("b");
    expect(isBulkMoveTargetInWorkflow("b-step", "b", snapshots, board, {})).toBe(true);
  });

  it("rejects duplicate ownership and contradictory task identity", () => {
    const duplicate = {
      ...snapshots,
      a: { ...snapshots.a, tasks: [{ id: "b-task", workflowId: "a" }] },
    };
    const contradictory = {
      ...snapshots,
      b: { ...snapshots.b, tasks: [{ id: "b-task", workflowId: "a" }] },
    };
    expect(resolveBulkSelectionWorkflow(["b-task"], duplicate, board)).toBeNull();
    expect(resolveBulkSelectionWorkflow(["b-task"], contradictory, board)).toBeNull();
  });

  it("rejects a destination occurring in two workflows", () => {
    const duplicate = { ...snapshots, a: { ...snapshots.a, steps: [{ id: "b-step" }] } };
    expect(isBulkMoveTargetInWorkflow("b-step", "b", duplicate, board, {})).toBe(false);
  });
});
