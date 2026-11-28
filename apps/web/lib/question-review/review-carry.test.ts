import { describe, expect, it } from "vitest";
import { createReviewCarry } from "./review-carry";

const screen = { width: 320, height: 200 };
const other = { width: 300, height: 220 };
const zoomed = { scale: 2, x: 40, y: 10 };

describe("createReviewCarry", () => {
  // @covers AC-TASKS-QUESTION-IMAGE-ITEMS-003.9
  it("returns a stage to the zoom and pan it was left at", () => {
    const carry = createReviewCarry();
    carry.rememberView("q1:pair", screen, zoomed);
    carry.rememberView("q2:image", other, null);
    expect(carry.recallView("q1:pair", screen)).toEqual(zoomed);
  });

  // @covers AC-TASKS-QUESTION-IMAGE-ITEMS-003.9
  it("carries the last view to a new image of the same size only", () => {
    const carry = createReviewCarry();
    carry.rememberView("q1:pair", screen, zoomed);
    expect(carry.recallView("q1:other", screen)).toEqual(zoomed);
    expect(carry.recallView("q1:single", other)).toBeNull();
  });

  // @covers AC-TASKS-QUESTION-IMAGE-ITEMS-003.9
  it("forgets a view reset to Fit", () => {
    const carry = createReviewCarry();
    carry.rememberView("q1:pair", screen, zoomed);
    carry.rememberView("q1:pair", screen, null);
    expect(carry.recallView("q1:pair", screen)).toBeNull();
    expect(carry.recallView("q1:next", screen)).toBeNull();
  });

  // @covers AC-TASKS-QUESTION-IMAGE-ITEMS-003.9
  it("keeps a stage left at Fit at Fit after another same-size image is zoomed", () => {
    const carry = createReviewCarry();
    carry.rememberView("q1:a", screen, null);
    carry.rememberView("q1:b", screen, zoomed);
    expect(carry.recallView("q1:a", screen)).toBeNull();
    expect(carry.recallView("q1:c", screen)).toEqual(zoomed);
  });

  it("keeps the comparison settings for the next stage", () => {
    const carry = createReviewCarry();
    expect(carry.read()).toBeUndefined();
    carry.write({ mode: "wipe", wipe: 0.3, opacity: 0.5, tolerance: 16 });
    expect(carry.read()?.mode).toBe("wipe");
  });
});
