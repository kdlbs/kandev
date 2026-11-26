import { describe, expect, it } from "vitest";
import { parseThemeDefinition } from "./theme-schema";

describe("portable theme boundary", () => {
  it("accepts canonical palette data and rejects CSS or unknown roles", () => {
    const value = { schema_version: 1, base: "dark", ui: { background: "#272822" } };
    expect(parseThemeDefinition(value)).toEqual(value);
    for (const ui of [
      { background: "url(evil)" },
      { font: "#ffffff" },
      { background: "#ffffff00" },
    ]) {
      expect(parseThemeDefinition({ ...value, ui })).toBeNull();
    }
    expect(parseThemeDefinition({ ...value, runtime: {} })).toBeNull();
  });
});

import night from "../../../backend/internal/plugins/themes/testdata/sample/themes/night.json";
import paper from "../../../backend/internal/plugins/themes/testdata/sample/themes/paper.json";
it("accepts the same two variant fixtures as the backend validator", () => {
  expect(parseThemeDefinition(night)?.base).toBe("dark");
  expect(parseThemeDefinition(paper)?.base).toBe("light");
});

it("accepts opaque uppercase RGBA colors", () => {
  expect(
    parseThemeDefinition({ schema_version: 1, base: "dark", ui: { background: "#FFFFFFFF" } }),
  ).not.toBeNull();
});
