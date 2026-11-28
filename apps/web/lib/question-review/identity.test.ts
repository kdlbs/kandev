import { describe, expect, it } from "vitest";
import { compareReducer, initialCompareState, type CompareMode } from "./compare";
import { visibleIdentity, wipeLabelSides, wipeNameLetters, wipeNameMaxLetters } from "./identity";

const start = (mode: CompareMode = "toggle") =>
  initialCompareState(["before", "after"], { mode, wipe: 0.5, opacity: 0.4, tolerance: 16 });

describe("visibleIdentity", () => {
  // @covers AC-TASKS-QUESTION-IMAGE-ITEMS-004.1
  it("names the shown image in Toggle and while holding", () => {
    expect(visibleIdentity(start(), false)).toEqual({ kind: "one", slot: "a" });
    const flipped = compareReducer(start(), { type: "flip" });
    expect(visibleIdentity(flipped, false)).toEqual({ kind: "one", slot: "b" });
    expect(visibleIdentity(flipped, true)).toEqual({ kind: "one", slot: "a" });
    expect(visibleIdentity(start("wipe"), true)).toEqual({ kind: "one", slot: "a" });
  });

  // @covers AC-TASKS-QUESTION-IMAGE-ITEMS-004.1
  it("describes each side or layer in the other modes", () => {
    expect(visibleIdentity(start("wipe"), false)).toEqual({ kind: "sides", wipe: 0.5 });
    expect(visibleIdentity(start("side"), false)).toEqual({ kind: "panes" });
    expect(visibleIdentity(start("onion"), false)).toEqual({ kind: "over", opacity: 40 });
    expect(visibleIdentity(start("diff"), false)).toEqual({ kind: "versus" });
  });

  // @covers AC-TASKS-QUESTION-IMAGE-ITEMS-004.2
  it("follows the slots, so a swap renames what each slot shows", () => {
    const swapped = compareReducer(start(), { type: "swap" });
    expect(swapped.aId).toBe("after");
    expect(visibleIdentity(swapped, false)).toEqual({ kind: "one", slot: "a" });
  });
});

describe("wipeLabelSides", () => {
  // @covers AC-TASKS-QUESTION-IMAGE-ITEMS-004.1
  it("hides the name on a side of the divider too narrow for it", () => {
    expect(wipeLabelSides(0.5, 400)).toEqual({ a: true, b: true });
    expect(wipeLabelSides(0.02, 400)).toEqual({ a: false, b: true });
    expect(wipeLabelSides(0.98, 400)).toEqual({ a: true, b: false });
  });

  it("shows both names before the stage is measured", () => {
    expect(wipeLabelSides(0, 0)).toEqual({ a: true, b: true });
  });
});

describe("wipeNameLetters", () => {
  // @covers AC-TASKS-QUESTION-IMAGE-ITEMS-004.1
  it("stacks a name letter by letter and cuts a long one with a vertical ellipsis", () => {
    expect(wipeNameLetters(" before ")).toEqual(["b", "e", "f", "o", "r", "e"]);
    expect(wipeNameLetters("settings screen", 5)).toEqual(["s", "e", "t", "t", "\u22ee"]);
    expect(wipeNameLetters("ab", 1)).toEqual(["a", "\u22ee"]);
  });

  it("keeps characters outside the basic plane whole", () => {
    expect(wipeNameLetters("a\u{1F600}b")).toEqual(["a", "\u{1F600}", "b"]);
  });
});

describe("wipeNameMaxLetters", () => {
  // @covers AC-TASKS-QUESTION-IMAGE-ITEMS-004.1
  it("fits the names to the stage height within fixed bounds", () => {
    expect(wipeNameMaxLetters(0)).toBe(12);
    expect(wipeNameMaxLetters(1000)).toBe(12);
    expect(wipeNameMaxLetters(224)).toBe(9);
    expect(wipeNameMaxLetters(120)).toBe(3);
  });
});
