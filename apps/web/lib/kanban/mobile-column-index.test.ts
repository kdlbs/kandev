import { describe, expect, it } from "vitest";
import { getInitialColumnIndex, resolveMobileColumnIndex } from "./mobile-column-index";

const steps = [{ id: "todo" }, { id: "plan" }, { id: "done" }];

describe("getInitialColumnIndex", () => {
  it("returns 0 when there are no steps", () => {
    expect(getInitialColumnIndex([], [{ workflowStepId: "plan" }])).toBe(0);
  });

  it("returns the first step that has tasks", () => {
    expect(
      getInitialColumnIndex(steps, [{ workflowStepId: "plan" }, { workflowStepId: "done" }]),
    ).toBe(1);
  });

  it("returns 0 when no step has tasks", () => {
    expect(getInitialColumnIndex(steps, [])).toBe(0);
  });
});

describe("resolveMobileColumnIndex", () => {
  it("restores the stored step when it is still present", () => {
    expect(resolveMobileColumnIndex(steps, [{ workflowStepId: "todo" }], "plan")).toBe(1);
  });

  it("falls back when the stored step id is missing", () => {
    expect(resolveMobileColumnIndex(steps, [{ workflowStepId: "done" }], undefined)).toBe(2);
  });

  it("falls back when the stored step id is no longer in the board", () => {
    expect(resolveMobileColumnIndex(steps, [{ workflowStepId: "todo" }], "deleted")).toBe(0);
  });

  it("returns 0 for an empty step list", () => {
    expect(resolveMobileColumnIndex([], [{ workflowStepId: "plan" }], "plan")).toBe(0);
  });
});
