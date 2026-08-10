import { describe, expect, it } from "vitest";
import {
  defaultGroupExpanded,
  effectiveGroupExpanded,
  shownStepCount,
  toggleGroupDisclosure,
} from "./steps-disclosure";

describe("defaultGroupExpanded — the disclosure-default function", () => {
  it("defaults to collapsed when nothing is hidden", () => {
    expect(defaultGroupExpanded([], new Set(["a", "b"]))).toBe(false);
  });

  it("defaults to expanded when a hidden id matches a live step", () => {
    expect(defaultGroupExpanded(["a"], new Set(["a", "b"]))).toBe(true);
  });

  it("defaults to collapsed when every hidden id is stale (matches no live step)", () => {
    expect(defaultGroupExpanded(["stale"], new Set(["a", "b"]))).toBe(false);
  });

  it("defaults to collapsed for a zero-step workflow even with hidden ids recorded", () => {
    expect(defaultGroupExpanded(["a"], new Set())).toBe(false);
  });
});

describe("effectiveGroupExpanded — the override-resolution function", () => {
  it("falls through to the recomputed default when no override is recorded", () => {
    expect(effectiveGroupExpanded("wf-a", {}, true)).toBe(true);
    expect(effectiveGroupExpanded("wf-a", {}, false)).toBe(false);
  });

  it("an explicit true override survives the hidden set going from empty to non-empty", () => {
    const overrides = { "wf-a": true };
    expect(effectiveGroupExpanded("wf-a", overrides, false)).toBe(true);
    expect(effectiveGroupExpanded("wf-a", overrides, true)).toBe(true);
  });

  it("an explicit false override survives the hidden set going from non-empty to empty", () => {
    const overrides = { "wf-a": false };
    expect(effectiveGroupExpanded("wf-a", overrides, true)).toBe(false);
    expect(effectiveGroupExpanded("wf-a", overrides, false)).toBe(false);
  });

  it("does not consult another workflow's override", () => {
    const overrides = { "wf-b": true };
    expect(effectiveGroupExpanded("wf-a", overrides, false)).toBe(false);
  });
});

describe("toggleGroupDisclosure — the toggle-write rule", () => {
  it("two toggles on a collapsed-default group leave false", () => {
    let overrides: Record<string, boolean> = {};
    overrides = toggleGroupDisclosure("wf-a", overrides, false);
    expect(overrides["wf-a"]).toBe(true);
    overrides = toggleGroupDisclosure("wf-a", overrides, false);
    expect(overrides["wf-a"]).toBe(false);
    expect("wf-a" in overrides).toBe(true);
  });

  it("two toggles on a hidden-bearing (expanded-default) group leave true, not false", () => {
    let overrides: Record<string, boolean> = {};
    overrides = toggleGroupDisclosure("wf-a", overrides, true);
    expect(overrides["wf-a"]).toBe(false);
    overrides = toggleGroupDisclosure("wf-a", overrides, true);
    expect(overrides["wf-a"]).toBe(true);
    expect("wf-a" in overrides).toBe(true);
  });

  it("does not mutate the other workflows' overrides", () => {
    const overrides = { "wf-b": true };
    const next = toggleGroupDisclosure("wf-a", overrides, false);
    expect(next).toEqual({ "wf-b": true, "wf-a": true });
  });
});

describe("shownStepCount — the shown-count derivation function", () => {
  it("reads N of N shown when nothing is hidden", () => {
    expect(shownStepCount(["a", "b", "c"], undefined)).toEqual({ shown: 3, total: 3 });
  });

  it("excludes a stale hidden id from both the hidden count and the total", () => {
    expect(shownStepCount(["a", "b", "c"], ["a", "stale"])).toEqual({ shown: 2, total: 3 });
  });

  it("reads 0 of 0 shown for a zero-step workflow", () => {
    expect(shownStepCount([], [])).toEqual({ shown: 0, total: 0 });
  });

  it("counts every live hidden id", () => {
    expect(shownStepCount(["a", "b"], ["a", "b"])).toEqual({ shown: 0, total: 2 });
  });
});
