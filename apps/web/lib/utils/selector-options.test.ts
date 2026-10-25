import { describe, expect, it } from "vitest";

import {
  configClassName,
  prioritizeSelectedOption,
  selectorOptionClassName,
} from "@/lib/utils/selector-options";

describe("prioritizeSelectedOption", () => {
  it("moves the selected option first without changing the source", () => {
    const source = [{ id: "first" }, { id: "current" }, { id: "last" }];

    expect(prioritizeSelectedOption(source, "current", (option) => option.id)).toEqual([
      { id: "current" },
      { id: "first" },
      { id: "last" },
    ]);
    expect(source).toEqual([{ id: "first" }, { id: "current" }, { id: "last" }]);
  });

  it("preserves source order when there is no selected option", () => {
    const source = [{ id: "first" }, { id: "last" }];

    expect(prioritizeSelectedOption(source, "missing", (option) => option.id)).toEqual(source);
    expect(prioritizeSelectedOption(source, "", (option) => option.id)).toEqual(source);
  });
});

describe("selectorOptionClassName", () => {
  it("returns only base classes when not selected", () => {
    const className = selectorOptionClassName(false);

    expect(className).toContain("border-transparent");
    expect(className).not.toContain("border-primary");
    expect(className).not.toContain("opacity-40");
  });

  it("adds a persistent surface and active highlight when selected", () => {
    const className = selectorOptionClassName(true);

    expect(className).toContain("border-primary/50");
    expect(className).toContain("bg-card");
    expect(className).toContain("data-[selected=true]:ring-2");
  });

  it("adds disabled classes to a selected option", () => {
    const className = selectorOptionClassName(true, true);

    expect(className).toContain("opacity-40");
    expect(className).toContain("cursor-not-allowed");
  });
});

describe("configClassName", () => {
  it("adds the selected surface only when selected", () => {
    expect(configClassName(true)).toContain("border-primary/50");
    expect(configClassName(false)).not.toContain("border-primary");
  });
});
