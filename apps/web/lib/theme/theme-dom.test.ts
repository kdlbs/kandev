import { expect, it } from "vitest";
import { applyThemeColors } from "./theme-dom";

it("clears prior roles and converts Git RGB values when changing palettes", () => {
  const root = document.createElement("div");
  applyThemeColors(root, {
    reference: "theme:test/dark",
    base: "dark",
    digest: "x",
    status: "ready",
    definition: {
      schema_version: 1,
      base: "dark",
      ui: { background: "#272822", "git-addition": "#a6e22e" },
    },
  });
  expect(root.style.getPropertyValue("--background")).toBe("#272822");
  expect(root.style.getPropertyValue("--git-addition")).toBe("166 226 46");
  applyThemeColors(root, { reference: "light", base: "light", digest: "light", status: "ready" });
  expect(root.style.getPropertyValue("--background")).toBe("");
});
