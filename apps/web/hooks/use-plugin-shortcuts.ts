"use client";

import { useEffect } from "react";
import { useAppStoreApi } from "@/components/state-provider";
import { usePlugins } from "@/hooks/domains/plugins/use-plugins";
import { pluginRegistry, usePluginRegistry } from "@/lib/plugins/registry";
import { isEditableKeydownTarget, matchesShortcut } from "@/lib/keyboard/utils";
import type { StoredShortcutOverrides } from "@/lib/keyboard/shortcut-overrides";
import {
  buildConfigurableShortcutEntries,
  resolveShortcutEntry,
} from "@/lib/keyboard/plugin-shortcuts";

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
 * Dispatches `event` to every registered plugin keybinding handler whose
 * effective combo matches. Iterates in `pluginRegistry.getKeybindingHandlers()`
 * order — registration order — so when two plugins bind the same combo, the
 * plugin that called `registerKeybinding` first runs first (both still run;
 * this only fixes the order, it does not stop at the first match).
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

  for (const { pluginId, id, handler } of pluginRegistry.getKeybindingHandlers()) {
    const entry = entryById.get(`plugin:${pluginId}:${id}`);
    if (!entry) continue;

    const shortcut = resolveShortcutEntry(entry, overrides);
    if (!matchesShortcut(event, shortcut)) continue;

    event.preventDefault();
    handler(event);
  }
}
