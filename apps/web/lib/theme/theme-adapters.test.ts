import { expect, it } from "vitest";
import { monacoThemeData } from "./theme-adapters";

it("maps generic editor and syntax roles without changing the base definitions", () => {
  const custom = monacoThemeData({
    reference: "theme:test/dark",
    base: "dark",
    digest: "x",
    status: "ready",
    definition: {
      schema_version: 1,
      base: "dark",
      editor: { background: "#272822", "diff-added-line": "#00ff0033" },
      syntax: { keyword: { foreground: "#f92672" } },
    },
  });
  expect(custom.colors["editor.background"]).toBe("#272822");
  expect(custom.colors["diffEditor.insertedLineBackground"]).toBe("#00ff0033");
  expect(custom.rules).toContainEqual(
    expect.objectContaining({ token: "keyword", foreground: "f92672" }),
  );
  const builtin = monacoThemeData({
    reference: "dark",
    base: "dark",
    digest: "dark",
    status: "ready",
  });
  expect(builtin.colors["editor.background"]).not.toBe("#272822");
});
