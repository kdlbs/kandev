import { describe, it, expect } from "vitest";
import {
  buildOptionGroups,
  hasGroupedOptions,
  partitionSelectedOptions,
} from "./filter-option-groups";

describe("buildOptionGroups", () => {
  it("collects options into buckets keyed by group, preserving first-seen order", () => {
    const result = buildOptionGroups([
      { value: "a1", group: "Alpha" },
      { value: "b1", group: "Beta" },
      { value: "a2", group: "Alpha" },
    ]);
    expect(result).toEqual([
      {
        heading: "Alpha",
        items: [
          { value: "a1", group: "Alpha" },
          { value: "a2", group: "Alpha" },
        ],
      },
      { heading: "Beta", items: [{ value: "b1", group: "Beta" }] },
    ]);
  });

  it("does not require options with the same group to be consecutive", () => {
    const result = buildOptionGroups([
      { value: "a1", group: "Alpha" },
      { value: "b1", group: "Beta" },
      { value: "a2", group: "Alpha" },
      { value: "b2", group: "Beta" },
    ]);
    expect(result.map((g) => g.heading)).toEqual(["Alpha", "Beta"]);
    expect(result[0]?.items.map((i) => i.value)).toEqual(["a1", "a2"]);
    expect(result[1]?.items.map((i) => i.value)).toEqual(["b1", "b2"]);
  });

  it("treats missing group as an empty-string heading", () => {
    const input: Array<{ value: string; group?: string }> = [{ value: "x" }, { value: "y" }];
    const result = buildOptionGroups(input);
    expect(result).toEqual([{ heading: "", items: input }]);
  });
});

describe("hasGroupedOptions", () => {
  it("returns true when any option has a group", () => {
    expect(hasGroupedOptions([{ group: "A" }, {}])).toBe(true);
  });

  it("returns false when no option has a group", () => {
    expect(hasGroupedOptions([{}, { group: "" }])).toBe(false);
  });
});

// @covers AC-UI-FILTER-SELECTED-FIRST-001.1
// @covers AC-UI-FILTER-SELECTED-FIRST-001.3
describe("partitionSelectedOptions", () => {
  const options = Object.freeze([
    { value: "a", group: "Alpha" },
    { value: "b", group: "Alpha" },
    { value: "c", group: "Beta" },
    { value: "d", group: "Beta" },
  ]);

  it("retains source order and object identity without mutating inputs", () => {
    const selected = new Set(["d", "missing", "b"]);
    const result = partitionSelectedOptions(options, selected);
    expect(result).toEqual({
      selected: [options[1], options[3]],
      unselected: [options[0], options[2]],
    });
    expect(result.selected[0]).toBe(options[1]);
    expect([...selected]).toEqual(["d", "missing", "b"]);
    expect(options.map((option) => option.value)).toEqual(["a", "b", "c", "d"]);
  });

  it.each([
    { selected: [], expectedSelected: [], expectedUnselected: ["a", "b", "c", "d"] },
    {
      selected: ["a", "b", "c", "d"],
      expectedSelected: ["a", "b", "c", "d"],
      expectedUnselected: [],
    },
    { selected: ["missing"], expectedSelected: [], expectedUnselected: ["a", "b", "c", "d"] },
  ])("handles selection $selected", ({ selected, expectedSelected, expectedUnselected }) => {
    const result = partitionSelectedOptions(options, new Set(selected));
    expect(result.selected.map((option) => option.value)).toEqual(expectedSelected);
    expect(result.unselected.map((option) => option.value)).toEqual(expectedUnselected);
  });

  it("does not invent rows when no options are available", () => {
    expect(partitionSelectedOptions([], new Set(["missing"]))).toEqual({
      selected: [],
      unselected: [],
    });
  });
});
