import { describe, it, expect } from "vitest";
import { findShortcutConflicts } from "./shortcut-conflicts";
import type { ShortcutEntry } from "./plugin-shortcuts";
import type { KeyboardShortcut } from "./constants";

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

describe("findShortcutConflicts", () => {
  // @covers AC-PLUGINS-PLUGIN-SHORTCUT-SETTINGS-001.5
  it.each([
    ["Mod and Ctrl", { ctrlOrCmd: true }, { ctrl: true }, true],
    ["Mod and Meta", { ctrlOrCmd: true }, { cmd: true }, true],
    ["Mod and both", { ctrlOrCmd: true }, { ctrl: true, cmd: true }, true],
    ["Ctrl and Meta", { ctrl: true }, { cmd: true }, false],
    ["Ctrl and both", { ctrl: true }, { ctrl: true, cmd: true }, false],
    ["Meta and both", { cmd: true }, { ctrl: true, cmd: true }, false],
    ["Mod and no modifier", { ctrlOrCmd: true }, {}, false],
    ["mixed Mod/explicit Ctrl and Meta", { ctrlOrCmd: true, ctrl: true }, { cmd: true }, true],
    ["mixed Mod/explicit Meta and Ctrl", { ctrlOrCmd: true, cmd: true }, { ctrl: true }, true],
    [
      "mixed Mod/explicit both and Ctrl",
      { ctrlOrCmd: true, ctrl: true, cmd: true },
      { ctrl: true },
      true,
    ],
    ["mixed false Mod and Meta", { ctrlOrCmd: false, ctrl: true }, { cmd: true }, false],
    ["different Alt", { ctrlOrCmd: true, alt: true }, { ctrl: true }, false],
    ["matching Alt", { ctrlOrCmd: true, alt: true }, { cmd: true, alt: true }, true],
    ["different Shift", { ctrlOrCmd: true, shift: true }, { cmd: true }, false],
    ["matching Shift", { ctrlOrCmd: true, shift: true }, { ctrl: true, shift: true }, true],
    ["bare keys", {}, {}, true],
  ] satisfies [string, KeyboardShortcut["modifiers"], KeyboardShortcut["modifiers"], boolean][])(
    "accepted overlap: %s",
    (_name, first, second, overlaps) => {
      const left = coreEntry("SEARCH", { key: "q", modifiers: first });
      const right = pluginEntry("plugin", "open", { key: "q", modifiers: second });
      const groups = findShortcutConflicts([
        { entry: left, shortcut: left.default },
        { entry: right, shortcut: right.default },
      ]);
      expect(groups.map((group) => group.entries)).toEqual(overlaps ? [[left, right]] : []);
    },
  );

  // @covers AC-PLUGINS-PLUGIN-SHORTCUT-SETTINGS-001.5
  it("accepted overlap keeps Mod bridges pairwise and compares key case", () => {
    const mod = coreEntry("SEARCH", { key: "Q" as never, modifiers: { ctrlOrCmd: true } });
    const ctrl = pluginEntry("ctrl", "open", { key: "q", modifiers: { ctrl: true } });
    const meta = pluginEntry("meta", "open", { key: "q", modifiers: { cmd: true } });
    const groups = findShortcutConflicts(
      [mod, ctrl, meta].map((entry) => ({
        entry,
        shortcut: entry.default,
      })),
    );
    expect(groups.map((group) => group.entries)).toEqual([
      [mod, ctrl],
      [mod, meta],
    ]);
  });

  it("accepted overlap excludes an unbound entry beside a bound entry", () => {
    const unbound = coreEntry("SEARCH", { key: "" as never });
    const bound = pluginEntry("plugin", "open", { key: "q" });
    expect(
      findShortcutConflicts(
        [unbound, bound].map((entry) => ({
          entry,
          shortcut: entry.default,
        })),
      ),
    ).toEqual([]);
  });
});

describe("findShortcutConflicts existing bindings", () => {
  it("detects a plugin-vs-core conflict", () => {
    const search = coreEntry("SEARCH", { key: "k", modifiers: { ctrlOrCmd: true } });
    const plugin = pluginEntry("session-cost", "open", {
      key: "k",
      modifiers: { ctrlOrCmd: true },
    });

    const conflicts = findShortcutConflicts([
      { entry: search, shortcut: search.default },
      { entry: plugin, shortcut: plugin.default },
    ]);

    expect(conflicts).toHaveLength(1);
    expect(conflicts[0].entries).toEqual([search, plugin]);
  });

  it("detects a plugin-vs-plugin conflict", () => {
    const pluginA = pluginEntry("plugin-a", "open", { key: "j", modifiers: { shift: true } });
    const pluginB = pluginEntry("plugin-b", "open", { key: "j", modifiers: { shift: true } });

    const conflicts = findShortcutConflicts([
      { entry: pluginA, shortcut: pluginA.default },
      { entry: pluginB, shortcut: pluginB.default },
    ]);

    expect(conflicts).toHaveLength(1);
    expect(conflicts[0].entries).toEqual([pluginA, pluginB]);
  });

  it("reports no conflicts when combos differ", () => {
    const pluginA = pluginEntry("plugin-a", "open", { key: "j" });
    const pluginB = pluginEntry("plugin-b", "open", { key: "k" });

    const conflicts = findShortcutConflicts([
      { entry: pluginA, shortcut: pluginA.default },
      { entry: pluginB, shortcut: pluginB.default },
    ]);

    expect(conflicts).toEqual([]);
  });

  it("does not flag two unbound shortcuts as conflicting", () => {
    const pluginA = pluginEntry("plugin-a", "open", { key: "" as never });
    const pluginB = pluginEntry("plugin-b", "open", { key: "" as never });

    const conflicts = findShortcutConflicts([
      { entry: pluginA, shortcut: pluginA.default },
      { entry: pluginB, shortcut: pluginB.default },
    ]);

    expect(conflicts).toEqual([]);
  });
});
