import { describe, expect, it } from "vitest";
import { WIPE_DRAG_MARGIN, wipeFromPointer } from "./wipe";

const stage = { left: 12, width: 366 };

describe("wipeFromPointer", () => {
  // @covers AC-TASKS-QUESTION-IMAGE-REVIEW-005.3
  it("reaches 0% and 100% with the pointer a margin inside the stage edges", () => {
    expect(wipeFromPointer(stage.left + WIPE_DRAG_MARGIN, stage)).toBe(0);
    expect(wipeFromPointer(stage.left + stage.width - WIPE_DRAG_MARGIN, stage)).toBe(1);
    expect(wipeFromPointer(stage.left + stage.width / 2, stage)).toBeCloseTo(0.5);
    expect(wipeFromPointer(0, stage)).toBe(0);
    expect(wipeFromPointer(1000, stage)).toBe(1);
  });

  it("falls back to the full width when the stage is too small for margins", () => {
    expect(wipeFromPointer(30, { left: 0, width: 40 })).toBeCloseTo(0.75);
  });
});
