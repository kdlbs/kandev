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
 *
 * Core-vs-plugin precedence: a combo that matches both a core shortcut and a
 * plugin keybinding must fire exactly one action, with core winning. Core
 * dispatchers (e.g. `useAppShortcuts`) call `event.preventDefault()` on a
 * match, and this hook bails out immediately when `event.defaultPrevented`
 * is already true — so plugins never shadow a built-in. This only works
 * because `useAppShortcuts()` is mounted (and therefore has its capture-phase
 * listener registered) before `usePluginShortcuts()` — see
 * `components/global-commands.tsx`. Keep that ordering when adding new core
 * dispatchers.
 *
 * Plugin bindings in turn win over per-component core shortcuts registered
 * via `useKeyboardShortcut`: this hook's dispatcher runs in the capture phase
 * (before `useKeyboardShortcut`'s bubble-phase listener) and calls
 * `event.preventDefault()` whenever it invokes a plugin handler (see
 * `dispatchMatchingPluginShortcuts` below), and `useKeyboardShortcut` bails
 * out when `event.defaultPrevented` is already true. So the full chain —
 * central core shortcuts > plugin keybindings > per-component core shortcuts
 * — always resolves to exactly one action per combo.
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
 * Dispatches `event` to every registered plugin keybinding handler whose
 * effective combo matches. Iterates in `pluginRegistry.getKeybindingHandlers()`
 * order — registration order — so when two plugins bind the same combo, the
 * plugin that called `registerKeybinding` first runs first (both still run;
 * this only fixes the order, it does not stop at the first match).
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

  for (const { pluginId, id, handler } of pluginRegistry.getKeybindingHandlers()) {
    const entry = entryById.get(`plugin:${pluginId}:${id}`);
    if (!entry) continue;

    const shortcut = resolveShortcutEntry(entry, overrides);
    if (!matchesShortcut(event, shortcut)) continue;

    event.preventDefault();
    try {
      handler(event);
    } catch (err) {
      console.error(`[plugin:${pluginId}] keybinding "${id}" handler threw an error:`, err);
    }
  }
}
