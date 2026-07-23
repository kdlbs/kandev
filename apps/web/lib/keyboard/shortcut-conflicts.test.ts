import { describe, it, expect } from "vitest";
import { comboKey, findShortcutConflicts } from "./shortcut-conflicts";
import type { ShortcutEntry } from "./plugin-shortcuts";

function coreEntry(id: string, shortcut: ShortcutEntry["default"]): ShortcutEntry {
  return { source: "core", id: id as never, label: id, default: shortcut };
}

function pluginEntry(
  pluginId: string,
  keybindingId: string,
  shortcut: ShortcutEntry["default"],
): ShortcutEntry {
  return {
    source: "plugin",
    id: `plugin:${pluginId}:${keybindingId}`,
    label: `${pluginId}: ${keybindingId}`,
    default: shortcut,
    pluginId,
    keybindingId,
  };
}

describe("comboKey", () => {
  it("resolves ctrlOrCmd to cmd on mac and ctrl elsewhere", () => {
    const shortcut = { key: "k" as const, modifiers: { ctrlOrCmd: true } };
    expect(comboKey(shortcut, true)).toBe(comboKey({ key: "k", modifiers: { cmd: true } }, true));
    expect(comboKey(shortcut, false)).toBe(
      comboKey({ key: "k", modifiers: { ctrl: true } }, false),
    );
  });

  it("returns null for an unbound shortcut", () => {
    expect(comboKey({ key: "" as never }, true)).toBeNull();
  });
});

describe("findShortcutConflicts", () => {
  it("detects a plugin-vs-core conflict", () => {
    const search = coreEntry("SEARCH", { key: "k", modifiers: { ctrlOrCmd: true } });
    const plugin = pluginEntry("session-cost", "open", {
      key: "k",
      modifiers: { ctrlOrCmd: true },
    });

    const conflicts = findShortcutConflicts(
      [
        { entry: search, shortcut: search.default },
        { entry: plugin, shortcut: plugin.default },
      ],
      true,
    );

    expect(conflicts).toHaveLength(1);
    expect(conflicts[0].entries).toEqual([search, plugin]);
  });

  it("detects a plugin-vs-plugin conflict", () => {
    const pluginA = pluginEntry("plugin-a", "open", { key: "j", modifiers: { shift: true } });
    const pluginB = pluginEntry("plugin-b", "open", { key: "j", modifiers: { shift: true } });

    const conflicts = findShortcutConflicts(
      [
        { entry: pluginA, shortcut: pluginA.default },
        { entry: pluginB, shortcut: pluginB.default },
      ],
      true,
    );

    expect(conflicts).toHaveLength(1);
    expect(conflicts[0].entries).toEqual([pluginA, pluginB]);
  });

  it("reports no conflicts when combos differ", () => {
    const pluginA = pluginEntry("plugin-a", "open", { key: "j" });
    const pluginB = pluginEntry("plugin-b", "open", { key: "k" });

    const conflicts = findShortcutConflicts(
      [
        { entry: pluginA, shortcut: pluginA.default },
        { entry: pluginB, shortcut: pluginB.default },
      ],
      true,
    );

    expect(conflicts).toEqual([]);
  });

  it("does not flag two unbound shortcuts as conflicting", () => {
    const pluginA = pluginEntry("plugin-a", "open", { key: "" as never });
    const pluginB = pluginEntry("plugin-b", "open", { key: "" as never });

    const conflicts = findShortcutConflicts(
      [
        { entry: pluginA, shortcut: pluginA.default },
        { entry: pluginB, shortcut: pluginB.default },
      ],
      true,
    );

    expect(conflicts).toEqual([]);
  });
});
