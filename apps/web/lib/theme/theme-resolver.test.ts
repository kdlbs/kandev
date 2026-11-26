import { expect, it } from "vitest";
import { resolveEffectiveTheme, isThemeSelection } from "./theme-resolver";
import type { ThemeDefinition, ThemeInventory } from "./theme-types";

it("resolves an installed digest and retains a removed reference without its colors", () => {
  const inventory: ThemeInventory = {
    boot_id: "b",
    revision: 1,
    themes: [
      {
        package_id: "sample",
        package_name: "Sample",
        key: "night",
        label: "Night",
        base: "dark",
        version: "1.0.0",
        digest: "abc",
      },
    ],
  };
  const definition: ThemeDefinition = {
    schema_version: 1,
    base: "dark",
    ui: { background: "#272822" },
  };
  const selected = resolveEffectiveTheme("theme:sample/night", "light", inventory, {
    abc: definition,
  });
  expect(selected).toMatchObject({ base: "dark", digest: "abc", definition, status: "ready" });
  const removed = resolveEffectiveTheme(
    "theme:sample/night",
    "light",
    { ...inventory, revision: 2, themes: [] },
    { abc: definition },
  );
  expect(removed).toMatchObject({ reference: "theme:sample/night", status: "unavailable" });
  expect(removed.definition).toBeUndefined();
  expect(isThemeSelection("theme:../x")).toBe(false);
});
