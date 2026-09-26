import { describe, expect, it } from "vitest";
import type { ResolvedSettingsDiscoveryItem } from "./types";
import { searchSettingsDiscovery } from "./search";

function item(
  id: string,
  label: string,
  options: Partial<ResolvedSettingsDiscoveryItem> = {},
): ResolvedSettingsDiscoveryItem {
  return {
    id,
    kind: "control",
    label,
    aliases: [],
    breadcrumb: ["General", "Terminal"],
    groupId: "general",
    groupLabel: "General",
    href: `/settings/general/terminal#setting-${id}`,
    targetId: `setting-${id}`,
    order: 0,
    ...options,
  };
}

describe("searchSettingsDiscovery", () => {
  const entries = [
    item("general", "General", {
      kind: "page",
      breadcrumb: [],
      href: "/settings/general",
      targetId: undefined,
      order: 0,
    }),
    item("terminal-font-size", "Terminal font size", {
      aliases: ["text size"],
      order: 1,
    }),
    item("terminal-font-family", "Terminal font family", {
      aliases: ["typeface"],
      order: 2,
    }),
    item("appearance-language", "Language", {
      aliases: ["locale"],
      breadcrumb: ["General", "Appearance"],
      order: 3,
    }),
  ];

  it("requires a direct label or alias match instead of flooding ancestor results", () => {
    expect(searchSettingsDiscovery(entries, "General").map((entry) => entry.id)).toEqual([
      "general",
    ]);
    expect(searchSettingsDiscovery(entries, "General font").map((entry) => entry.id)).toEqual([
      "terminal-font-size",
      "terminal-font-family",
    ]);
  });

  it("ranks exact labels above exact aliases and prefixes", () => {
    const aliasMatch = item("alias", "Text scale", {
      aliases: ["font size"],
      order: 0,
    });
    const prefixMatch = item("prefix", "Font size behavior", { order: 1 });
    const exactMatch = item("exact", "Font size", { order: 2 });

    expect(
      searchSettingsDiscovery([aliasMatch, prefixMatch, exactMatch], "font size").map(
        (entry) => entry.id,
      ),
    ).toEqual(["exact", "alias", "prefix"]);
  });

  it("normalizes case, whitespace, and diacritics with Unicode terms", () => {
    const localized = item("localized", "Tamaño de fuente", {
      aliases: ["tipografía"],
    });

    expect(
      searchSettingsDiscovery([localized], "  TAMANO   fuente ").map((entry) => entry.id),
    ).toEqual(["localized"]);
    expect(searchSettingsDiscovery([localized], "tipografia").map((entry) => entry.id)).toEqual([
      "localized",
    ]);
  });
});
