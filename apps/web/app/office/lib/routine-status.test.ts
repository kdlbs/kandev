import { describe, expect, it } from "vitest";

import { isRoutineFiring } from "./routine-status";

describe("isRoutineFiring", () => {
  it("fires for active and the empty string", () => {
    expect(isRoutineFiring("active")).toBe(true);
    expect(isRoutineFiring("")).toBe(true);
  });

  it("does not fire for paused or archived", () => {
    expect(isRoutineFiring("paused")).toBe(false);
    expect(isRoutineFiring("archived")).toBe(false);
  });

  it("does not fire for an unrecognized value", () => {
    expect(isRoutineFiring("on_hold")).toBe(false);
  });

  it("is byte-exact: no case folding, no trimming", () => {
    expect(isRoutineFiring("Active")).toBe(false);
    expect(isRoutineFiring(" active")).toBe(false);
    expect(isRoutineFiring("active ")).toBe(false);
  });
});
