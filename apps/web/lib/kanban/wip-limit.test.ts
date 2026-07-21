import { describe, expect, it } from "vitest";
import { formatWipCount, isOverWipLimit } from "./wip-limit";

describe("WIP limit display helpers", () => {
  it("formats unlimited steps as a plain task count", () => {
    expect(formatWipCount(3, 0)).toBe("3");
    expect(formatWipCount(3, undefined)).toBe("3");
  });

  it("formats limited steps as occupied over limit", () => {
    expect(formatWipCount(3, 5)).toBe("3/5");
  });

  it("only warns when a positive limit is exceeded", () => {
    expect(isOverWipLimit(3, 2)).toBe(true);
    expect(isOverWipLimit(2, 2)).toBe(false);
    expect(isOverWipLimit(99, 0)).toBe(false);
  });
});
