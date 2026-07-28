import { describe, it, expect, vi, afterEach } from "vitest";
import type { PluginRecord } from "@/lib/types/plugins";
import {
  buildConfigurableShortcutEntries,
  pluginShortcutId,
  resolveShortcutEntry,
  type ShortcutEntry,
} from "./plugin-shortcuts";

const PLUGIN_ID = "session-cost";
const KEYBINDING_ID = "toggle-panel";
const NAMESPACED_ID = pluginShortcutId(PLUGIN_ID, KEYBINDING_ID);

function makePlugin(overrides: Partial<PluginRecord> = {}): PluginRecord {
  return {
    id: PLUGIN_ID,
    api_version: 1,
    version: "1.0.0",
    display_name: "Session Cost",
    description: "",
    author: "",
    categories: [],
    capabilities: {},
    status: "active",
    install_path: "",
    signed: true,
    installed_at: "",
    restart_count: 0,
    ...overrides,
  };
}

function makeTogglePanelEntry(): ShortcutEntry {
  return {
    source: "plugin",
    id: NAMESPACED_ID,
    label: "Session Cost: Toggle panel",
    default: { key: "k", modifiers: { ctrlOrCmd: true } },
    pluginId: PLUGIN_ID,
    keybindingId: KEYBINDING_ID,
  };
}

describe("pluginShortcutId", () => {
  it("namespaces the id with the plugin id", () => {
    expect(pluginShortcutId(PLUGIN_ID, KEYBINDING_ID)).toBe(NAMESPACED_ID);
  });
});

describe("buildConfigurableShortcutEntries", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("includes core entries followed by plugin entries", () => {
    const plugin = makePlugin({
      ui: {
        keybindings: [{ id: KEYBINDING_ID, default: "mod+shift+k", description: "Toggle panel" }],
      },
    });

    const entries = buildConfigurableShortcutEntries([plugin]);
    const coreEntry = entries.find((e) => e.source === "core" && e.id === "SEARCH");
    const pluginEntry = entries.find((e) => e.source === "plugin" && e.id === NAMESPACED_ID);

    expect(coreEntry).toBeDefined();
    expect(pluginEntry).toMatchObject({
      source: "plugin",
      id: NAMESPACED_ID,
      label: "Session Cost: Toggle panel",
      default: { key: "k", modifiers: { ctrlOrCmd: true, shift: true } },
      pluginId: PLUGIN_ID,
      keybindingId: KEYBINDING_ID,
    });
  });

  it("skips a plugin with no declared keybindings", () => {
    const plugin = makePlugin({ ui: { bundle: "ui/bundle.js" } });
    const entries = buildConfigurableShortcutEntries([plugin]);
    expect(entries.some((e) => e.source === "plugin")).toBe(false);
  });

  it("warns and skips a keybinding with an unparseable default combo", () => {
    const warnSpy = vi.spyOn(console, "warn").mockImplementation(() => {});
    const plugin = makePlugin({
      ui: { keybindings: [{ id: "bad", default: "banana", description: "Bad" }] },
    });

    const entries = buildConfigurableShortcutEntries([plugin]);

    expect(entries.some((e) => e.source === "plugin")).toBe(false);
    expect(warnSpy).toHaveBeenCalledWith(expect.stringContaining("bad"));
  });
});

describe("resolveShortcutEntry", () => {
  it("returns the default when no override is set", () => {
    const entry = makeTogglePanelEntry();

    expect(resolveShortcutEntry(entry)).toEqual(entry.default);
  });

  it("returns the user override when set", () => {
    const entry = makeTogglePanelEntry();
    const override = { key: "j", modifiers: { shift: true } };

    expect(resolveShortcutEntry(entry, { [entry.id]: override })).toEqual(override);
  });
});
