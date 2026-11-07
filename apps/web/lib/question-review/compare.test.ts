import { describe, expect, it } from "vitest";
import { compareReducer, initialCompareState, type CompareState } from "./compare";

const ids = ["before", "after", "other"];
const start = initialCompareState(ids);

describe("compareReducer", () => {
  // @covers AC-TASKS-QUESTION-IMAGE-REVIEW-005.1
  it("starts in Toggle with the first image as A and the second as B", () => {
    expect(start).toMatchObject({ mode: "toggle", aId: "before", bId: "after", showB: false });
  });

  // @covers AC-TASKS-QUESTION-IMAGE-REVIEW-005.2
  it("flips and swaps", () => {
    const flipped = compareReducer(start, { type: "flip" });
    expect(flipped.showB).toBe(true);
    const swapped = compareReducer(start, { type: "swap" });
    expect([swapped.aId, swapped.bId]).toEqual(["after", "before"]);
  });

  it("swaps instead of comparing an image with itself", () => {
    const next = compareReducer(start, { type: "pick", slot: "a", id: "after" });
    expect([next.aId, next.bId]).toEqual(["after", "before"]);
    const other = compareReducer(start, { type: "pick", slot: "b", id: "other" });
    expect([other.aId, other.bId]).toEqual(["before", "other"]);
  });

  it("clamps wipe, opacity, and tolerance", () => {
    let state: CompareState = compareReducer(start, { type: "wipe", value: 1.4 });
    expect(state.wipe).toBe(1);
    state = compareReducer(state, { type: "opacity", value: -1 });
    expect(state.opacity).toBe(0);
    state = compareReducer(state, { type: "tolerance", value: 99 });
    expect(state.tolerance).toBe(64);
  });

  // @covers AC-TASKS-QUESTION-IMAGE-REVIEW-005.7
  it("changes mode without touching the image choice or the shown image", () => {
    const flipped = compareReducer(start, { type: "flip" });
    const wipe = compareReducer(flipped, { type: "mode", mode: "wipe" });
    expect(wipe).toMatchObject({ mode: "wipe", aId: "before", bId: "after", showB: true });
  });

  it("nudges the active slider by key in Wipe and Onion skin only", () => {
    const wipe = compareReducer(start, { type: "mode", mode: "wipe" });
    expect(compareReducer(wipe, { type: "nudge", direction: 1 }).wipe).toBeCloseTo(0.55);
    const onion = compareReducer(start, { type: "mode", mode: "onion" });
    expect(compareReducer(onion, { type: "nudge", direction: -1 }).opacity).toBeCloseTo(0.45);
    expect(compareReducer(start, { type: "nudge", direction: 1 })).toBe(start);
  });
});

describe("showing an image for a selected region", () => {
  // @covers AC-TASKS-QUESTION-IMAGE-REVIEW-007.1
  it("switches to Toggle and shows the image as A or B, picking it as A otherwise", () => {
    const wipe = compareReducer(start, { type: "mode", mode: "wipe" });
    expect(compareReducer(wipe, { type: "show", id: "after" })).toMatchObject({
      mode: "toggle",
      showB: true,
    });
    expect(compareReducer(wipe, { type: "show", id: "before" })).toMatchObject({ showB: false });
    expect(compareReducer(start, { type: "show", id: "other" })).toMatchObject({
      aId: "other",
      bId: "after",
      showB: false,
    });
  });
});
