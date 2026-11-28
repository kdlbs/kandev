import { describe, expect, it } from "vitest";
import { WIPE_DRAG_MARGIN, wipeFromDrag, wipeFromPointer } from "./wipe";

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

describe("wipeFromDrag", () => {
  // @covers AC-TASKS-QUESTION-IMAGE-ITEMS-004.4
  it("moves the split by the horizontal travel only, wherever the grip was grabbed", () => {
    const wide = { left: 100, width: 600 };
    expect(wipeFromDrag(0.5, 0, wide)).toBe(0.5);
    expect(wipeFromDrag(0.5, 55.2, wide)).toBeCloseTo(0.6);
    expect(wipeFromDrag(0.9, 200, wide)).toBe(1);
    expect(wipeFromDrag(0.3, 0, { left: 0, width: 0 })).toBe(0.3);
  });
});
