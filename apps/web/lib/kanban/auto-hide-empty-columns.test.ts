import { describe, expect, it } from "vitest";
import { deriveAutoHiddenStepIds } from "./auto-hide-empty-columns";

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
