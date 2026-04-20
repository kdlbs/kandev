import { describe, it, expect } from "vitest";
import { buildOptionGroups, hasGroupedOptions } from "./filter-option-groups";

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
