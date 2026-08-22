import { describe, expect, it } from "vitest";

import { prioritizeSelectedOption } from "@/lib/utils/selector-options";

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
