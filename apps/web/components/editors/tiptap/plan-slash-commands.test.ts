import { describe, expect, it } from "vitest";
import type { TFunction } from "i18next";
import { buildPlanSlashCommands, filterPlanSlashCommands } from "./plan-slash-commands";

/**
 * Stands in for i18next: returns the key itself under `en`, and a marked form
 * under `xx`, so a locale switch is observable without loading real catalogs.
 */
function fakeT(locale: "en" | "xx"): TFunction {
  const labels: Record<string, string> = {
    "editors:slashHeading1": "Heading 1",
    "editors:slashBulletList": "Bullet List",
    "editors:slashCodeBlock": "Code Block",
    "editors:slashTable": "Table",
  };
  return ((key: string) => {
    const en = labels[key] ?? key;
    return locale === "en" ? en : `[${en}]`;
  }) as unknown as TFunction;
}

describe("buildPlanSlashCommands", () => {
  it("resolves labels through the passed t", () => {
    const commands = buildPlanSlashCommands(fakeT("en"));
    expect(commands.find((c) => c.id === "heading1")?.label).toBe("Heading 1");
    expect(commands.find((c) => c.id === "table")?.label).toBe("Table");
  });

  it("re-resolves copy when called again with a different locale", () => {
    const before = buildPlanSlashCommands(fakeT("en"));
    const after = buildPlanSlashCommands(fakeT("xx"));

    expect(before.find((c) => c.id === "heading1")?.label).toBe("Heading 1");
    expect(after.find((c) => c.id === "heading1")?.label).toBe("[Heading 1]");
  });

  it("keeps ids and category grouping keys stable across locales", () => {
    const before = buildPlanSlashCommands(fakeT("en"));
    const after = buildPlanSlashCommands(fakeT("xx"));

    expect(after.map((c) => c.id)).toEqual(before.map((c) => c.id));
    expect(after.map((c) => c.category)).toEqual(before.map((c) => c.category));
    expect(new Set(after.map((c) => c.category))).toEqual(new Set(["text", "lists", "blocks"]));
  });
});

describe("filterPlanSlashCommands", () => {
  it("returns every command for an empty query", () => {
    const commands = buildPlanSlashCommands(fakeT("en"));
    expect(filterPlanSlashCommands(commands, "")).toHaveLength(commands.length);
  });

  it("matches on the translated label", () => {
    const commands = buildPlanSlashCommands(fakeT("en"));
    const matched = filterPlanSlashCommands(commands, "bullet");
    expect(matched.map((c) => c.id)).toEqual(["bulletList"]);
  });

  it("matches on the stable id even when the label is translated away", () => {
    const commands = buildPlanSlashCommands(fakeT("xx"));
    // The label is now "[Code Block]", so this only matches via `id`.
    const matched = filterPlanSlashCommands(commands, "codeblock");
    expect(matched.map((c) => c.id)).toEqual(["codeBlock"]);
  });

  it("is case-insensitive", () => {
    const commands = buildPlanSlashCommands(fakeT("en"));
    expect(filterPlanSlashCommands(commands, "TABLE").map((c) => c.id)).toEqual(["table"]);
  });

  it("returns nothing when no command matches", () => {
    const commands = buildPlanSlashCommands(fakeT("en"));
    expect(filterPlanSlashCommands(commands, "nonexistent")).toEqual([]);
  });
});
