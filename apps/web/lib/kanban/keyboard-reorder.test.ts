import { describe, expect, it } from "vitest";
import { moveOneStep } from "./keyboard-reorder";

describe("moveOneStep", () => {
  const order = ["a", "b", "c"];

  it("moves the task up one place", () => {
    expect(moveOneStep(order, "b", "up")).toEqual(["b", "a", "c"]);
  });

  it("moves the task down one place", () => {
    expect(moveOneStep(order, "b", "down")).toEqual(["a", "c", "b"]);
  });

  it("ignores an up move at the top of the band", () => {
    expect(moveOneStep(order, "a", "up")).toBeNull();
  });

  it("ignores a down move at the bottom of the band", () => {
    expect(moveOneStep(order, "c", "down")).toBeNull();
  });

  it("returns null when the task is not in the order", () => {
    expect(moveOneStep(order, "missing", "up")).toBeNull();
  });
});
