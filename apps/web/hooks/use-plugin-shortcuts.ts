"use client";

import { useEffect } from "react";
import { useAppStoreApi } from "@/components/state-provider";
import { usePlugins } from "@/hooks/domains/plugins/use-plugins";
import { pluginRegistry, usePluginRegistry } from "@/lib/plugins/registry";
import { isEditableKeydownTarget, isMac, matchesShortcut } from "@/lib/keyboard/utils";
import type { StoredShortcutOverrides } from "@/lib/keyboard/shortcut-overrides";
import {
  buildConfigurableShortcutEntries,
  coreShortcutEntries,
  resolveShortcutEntry,
} from "@/lib/keyboard/plugin-shortcuts";
import { comboKey } from "@/lib/keyboard/shortcut-conflicts";
import { SHORTCUTS } from "@/lib/keyboard/constants";

/**
 * Central shortcuts that are not in `CONFIGURABLE_SHORTCUTS` (so they have no
 * user override to resolve) but are still global, always-on core behavior
 * that must win over a plugin keybinding bound to the same combo:
 * - `FIND_IN_PANEL` (Cmd/Ctrl+F) — per-panel capture-phase listeners
 *   (`use-panel-search.ts`, terminal find) that don't check
 *   `event.defaultPrevented`.
 * - `SAVE` (Cmd/Ctrl+S) — reserved for editor save; must not be hijacked by a
 *   plugin even before a save listener exists for every surface.
 *
 * Other `SHORTCUTS` entries not in this list (`SUBMIT`, `SUBMIT_ENTER`,
 * `CANCEL`, `COMMAND_PANEL_SHIFT`) are contextual/per-component shortcuts —
 * typically registered via `useKeyboardShortcut`, which already yields to
 * plugin keybindings — so they're intentionally excluded here.
 */
const NON_CONFIGURABLE_CORE_SHORTCUT_IDS = [
  "FIND_IN_PANEL",
  "SAVE",
] as const satisfies ReadonlyArray<keyof typeof SHORTCUTS>;

/**
 * Global dispatcher for plugin-declared keybindings (`ui.keybindings`),
 * mirroring `useAppShortcuts`'s capture-phase, editable-skip pattern. Mount
 * once at the app root (alongside `useAppShortcuts`, see
 * `components/global-commands.tsx`).
 *
 * Also syncs each active plugin's declared keybinding ids into
 * `pluginRegistry` (`setDeclaredKeybindingIds`) so `registerKeybinding` can
 * warn on an id the manifest never declared — this is the one call site with
 * both the plugin records (`usePlugins`) and the registry.
 *
 * Dispatch order when two active plugins bind the same effective combo:
 * registration order (`pluginRegistry.getKeybindingHandlers()` iteration
 * order) — the first plugin to call `registerKeybinding` for that combo
 * wins; later handlers for the same event still run since this dispatches
 * to every match, not just the first.
 *
 * Core-vs-plugin precedence: effective CORE shortcuts always win over plugin
 * keybindings. There is no single core keydown dispatcher — panel-search,
 * task-switcher, save, editor keybinds, and other per-component listeners
 * are all independent, and most of them don't (and shouldn't have to) check
 * `event.defaultPrevented`. So instead of relying on every core listener
 * cooperating, this hook computes the set of effective CORE shortcut combos
 * (the central `SHORTCUTS`/`CONFIGURABLE_SHORTCUTS` registry, resolved
 * through any user overrides — see `buildCoreComboKeySet` below) and simply
 * never invokes a plugin handler whose effective combo is in that set. The
 * plugin handler is skipped entirely — no invocation, no `preventDefault()`
 * — so whichever core listener owns that combo runs normally, exactly once.
 * This makes panel-search/task-switcher/save/find-in-panel/etc. win over
 * plugins without editing any of those listeners, and it still works when a
 * user remaps a core shortcut's combo (the override is read fresh on every
 * keydown).
 *
 * Among non-core listeners, this capture-phase plugin dispatcher wins over
 * per-component core shortcuts registered via `useKeyboardShortcut` (bubble
 * phase) and `useEditorKeybinds`'s capture-phase handler: this hook calls
 * `event.preventDefault()` whenever it invokes a plugin handler for a
 * non-core combo, and those listeners bail out when `event.defaultPrevented`
 * is already true. So the full chain — effective core shortcuts (always win)
 * > plugin keybindings > per-component core shortcuts — resolves to exactly
 * one action per combo.
 */
export function usePluginShortcuts(): void {
  const appStore = useAppStoreApi();
  const { items } = usePlugins();
  usePluginRegistry();

  useEffect(() => {
    for (const plugin of items) {
      pluginRegistry.setDeclaredKeybindingIds(
        plugin.id,
        (plugin.ui?.keybindings ?? []).map((kb) => kb.id),
      );
    }
  }, [items]);

  useEffect(() => {
    const handler = (event: KeyboardEvent) => {
      // A core dispatcher already handled this combo (and called
      // preventDefault) — core wins, so bail out without invoking any
      // plugin handler for the same keypress.
      if (event.defaultPrevented) return;
      if (isEditableKeydownTarget(event)) return;

      const overrides = appStore.getState().userSettings
        .keyboardShortcuts as StoredShortcutOverrides;
      dispatchMatchingPluginShortcuts(event, items, overrides);
    };

    // Capture phase so plugin shortcuts win before focus-trapped surfaces
    // (e.g. xterm.js) can swallow the event — mirrors useAppShortcuts.
    window.addEventListener("keydown", handler, true);
    return () => window.removeEventListener("keydown", handler, true);
  }, [appStore, items]);
}

/**
 * Builds the set of effective CORE shortcut combo-keys (registry defaults,
 * resolved through any user override), so the dispatcher can recognize a
 * plugin combo that shadows a core one. Recomputed on every keydown from the
 * `overrides` snapshot read in the handler, so a user remapping a core
 * shortcut's combo takes effect immediately — no separate memoization
 * lifecycle to keep in sync with the override store.
 */
function buildCoreComboKeySet(
  overrides: StoredShortcutOverrides,
  isMacPlatform: boolean,
): Set<string> {
  const keys = new Set<string>();
  for (const entry of coreShortcutEntries()) {
    const shortcut = resolveShortcutEntry(entry, overrides);
    const key = comboKey(shortcut, isMacPlatform);
    if (key !== null) keys.add(key);
  }
  for (const id of NON_CONFIGURABLE_CORE_SHORTCUT_IDS) {
    const key = comboKey(SHORTCUTS[id], isMacPlatform);
    if (key !== null) keys.add(key);
  }
  return keys;
}

/**
 * Dispatches `event` to every registered plugin keybinding handler whose
 * effective combo matches. Iterates in `pluginRegistry.getKeybindingHandlers()`
 * order — registration order — so when two plugins bind the same combo, the
 * plugin that called `registerKeybinding` first runs first (both still run;
 * this only fixes the order, it does not stop at the first match).
 *
 * Before invoking a handler, checks whether that plugin combo's `comboKey`
 * matches an effective CORE shortcut (see `buildCoreComboKeySet`). If it
 * does, the plugin handler is skipped entirely — core always wins, and the
 * corresponding core listener (panel-search, task-switcher, save, etc.)
 * handles the event normally.
 *
 * Each handler invocation is isolated in its own try/catch: a throwing
 * handler is logged (with the offending plugin/keybinding id) and does not
 * abort the loop, so one broken plugin can't prevent other plugins bound to
 * the same combo from running.
 */
function dispatchMatchingPluginShortcuts(
  event: KeyboardEvent,
  plugins: Parameters<typeof buildConfigurableShortcutEntries>[0],
  overrides: StoredShortcutOverrides,
): void {
  const entryById = new Map(
    buildConfigurableShortcutEntries(plugins)
      .filter((entry) => entry.source === "plugin")
      .map((entry) => [entry.id, entry] as const),
  );

  const isMacPlatform = isMac();
  const coreComboKeys = buildCoreComboKeySet(overrides, isMacPlatform);

  for (const { pluginId, id, handler } of pluginRegistry.getKeybindingHandlers()) {
    const entry = entryById.get(`plugin:${pluginId}:${id}`);
    if (!entry) continue;

    const shortcut = resolveShortcutEntry(entry, overrides);
    if (!matchesShortcut(event, shortcut)) continue;

    const pluginComboKey = comboKey(shortcut, isMacPlatform);
    if (pluginComboKey !== null && coreComboKeys.has(pluginComboKey)) {
      if (process.env.NODE_ENV !== "production") {
        console.warn(
          `[plugin:${pluginId}] keybinding "${id}" is shadowed by a core shortcut bound to the same combo and will not fire.`,
        );
      }
      continue;
    }

    event.preventDefault();
    try {
      handler(event);
    } catch (err) {
      console.error(`[plugin:${pluginId}] keybinding "${id}" handler threw an error:`, err);
    }
  }
}
