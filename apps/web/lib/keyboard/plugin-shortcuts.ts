/**
 * Dynamic layer over `CONFIGURABLE_SHORTCUTS`: builds the same
 * label/default shape for plugin-declared keybindings (`ui.keybindings` on a
 * `PluginRecord`), so Settings > Keyboard Shortcuts can render and
 * user-override plugin shortcuts alongside the static core list without
 * widening `ConfigurableShortcutId` to an open string type.
 *
 * Namespacing: a plugin keybinding's effective override id is
 * `plugin:{pluginId}:{keybindingId}` — this is what gets stored under
 * `userSettings.keyboardShortcuts` (the same `StoredShortcutOverrides` map
 * core shortcuts already use).
 */
import type { PluginRecord } from "@/lib/types/plugins";
import type { KeyboardShortcut } from "./constants";
import { parseCombo } from "./parse-combo";
import {
  CONFIGURABLE_SHORTCUTS,
  type ConfigurableShortcutId,
  type StoredShortcutOverrides,
} from "./shortcut-overrides";

/** Prefix marking a namespaced id as belonging to a plugin, not core. */
export const PLUGIN_SHORTCUT_PREFIX = "plugin:";

export function pluginShortcutId(pluginId: string, keybindingId: string): string {
  return `${PLUGIN_SHORTCUT_PREFIX}${pluginId}:${keybindingId}`;
}

/** One configurable shortcut entry, from either the core static list or a plugin manifest. */
export type ShortcutEntry =
  | { source: "core"; id: ConfigurableShortcutId; label: string; default: KeyboardShortcut }
  | {
      source: "plugin";
      id: string;
      label: string;
      default: KeyboardShortcut;
      pluginId: string;
      keybindingId: string;
    };

/**
 * The static core shortcuts, in `ShortcutEntry` shape — exported so callers
 * that already hold a `ShortcutEntry[]` for plugins (e.g.
 * `KeyboardShortcutsCard`, which receives plugin entries as a prop rather
 * than raw `PluginRecord[]`) can merge core + plugin entries without
 * re-deriving the core list themselves.
 */
export function coreShortcutEntries(): ShortcutEntry[] {
  return (Object.keys(CONFIGURABLE_SHORTCUTS) as ConfigurableShortcutId[]).map((id) => ({
    source: "core",
    id,
    label: CONFIGURABLE_SHORTCUTS[id].label,
    default: CONFIGURABLE_SHORTCUTS[id].default,
  }));
}

/**
 * Builds the plugin-sourced configurable shortcut entries for every active
 * plugin's declared `ui.keybindings`. A combo that fails to parse (should
 * never happen — the backend validates manifests at registration) is
 * skipped with a console warning rather than throwing.
 */
function buildPluginEntries(plugins: PluginRecord[]): ShortcutEntry[] {
  const entries: ShortcutEntry[] = [];
  for (const plugin of plugins) {
    for (const keybinding of plugin.ui?.keybindings ?? []) {
      const shortcut = parseCombo(keybinding.default);
      if (!shortcut) {
        console.warn(
          `[plugins] "${plugin.id}" keybinding "${keybinding.id}" has an unparseable default combo "${keybinding.default}"`,
        );
        continue;
      }
      entries.push({
        source: "plugin",
        id: pluginShortcutId(plugin.id, keybinding.id),
        label: `${plugin.display_name}: ${keybinding.description}`,
        default: shortcut,
        pluginId: plugin.id,
        keybindingId: keybinding.id,
      });
    }
  }
  return entries;
}

/**
 * The full list of configurable shortcuts — core (static) entries followed
 * by plugin (dynamic) entries — for rendering in Settings > Keyboard
 * Shortcuts. Core behavior/order is unchanged from `CONFIGURABLE_SHORTCUTS`.
 */
export function buildConfigurableShortcutEntries(plugins: PluginRecord[]): ShortcutEntry[] {
  return [...coreShortcutEntries(), ...buildPluginEntries(plugins)];
}

/** Builds only the plugin-sourced entries — used by callers that already have core entries. */
export function buildPluginShortcutEntries(plugins: PluginRecord[]): ShortcutEntry[] {
  return buildPluginEntries(plugins);
}

/** Resolves an entry's effective shortcut: the user override if set, else its default. */
export function resolveShortcutEntry(
  entry: ShortcutEntry,
  overrides?: StoredShortcutOverrides,
): KeyboardShortcut {
  const override = overrides?.[entry.id];
  if (override) return override as KeyboardShortcut;
  return entry.default;
}
