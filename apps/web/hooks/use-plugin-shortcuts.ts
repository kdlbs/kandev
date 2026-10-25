"use client";

import { useEffect } from "react";
import { useAppStoreApi } from "@/components/state-provider";
import { usePlugins } from "@/hooks/domains/plugins/use-plugins";
import { pluginRegistry, usePluginRegistry } from "@/lib/plugins/registry";
import { isEditableKeydownTarget, matchesShortcut } from "@/lib/keyboard/utils";
import { isUnboundShortcut, type StoredShortcutOverrides } from "@/lib/keyboard/shortcut-overrides";
import {
  buildConfigurableShortcutEntries,
  coreShortcutEntries,
  resolveShortcutEntry,
} from "@/lib/keyboard/plugin-shortcuts";
import { SHORTCUTS, type KeyboardShortcut } from "@/lib/keyboard/constants";
import { NON_CONFIGURABLE_CORE_SHORTCUT_IDS } from "@/lib/keyboard/core-shortcuts";

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
 * cooperating, this hook reserves each event accepted by an effective core
 * shortcut. Configurable defaults are resolved through the current override
 * snapshot; nonconfigurable core combinations retain their fixed defaults.
 * A reserved event never invokes or prevents a plugin handler, leaving the
 * owning core listener able to handle it normally.
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

      const overrides = appStore.getState().userSettings
        .keyboardShortcuts as StoredShortcutOverrides;
      dispatchMatchingPluginShortcuts(event, items, overrides, isEditableKeydownTarget(event));
    };

    // Capture phase so plugin shortcuts win before focus-trapped surfaces
    // (e.g. xterm.js) can swallow the event — mirrors useAppShortcuts.
    window.addEventListener("keydown", handler, true);
    return () => window.removeEventListener("keydown", handler, true);
  }, [appStore, items]);
}

/** Checks the current event against configurable and fixed core reservations. */
function isCoreShortcutEvent(event: KeyboardEvent, overrides: StoredShortcutOverrides): boolean {
  return (
    coreShortcutEntries().some((entry) => {
      const shortcut = resolveShortcutEntry(entry, overrides);
      return !isUnboundShortcut(shortcut) && matchesShortcut(event, shortcut);
    }) || NON_CONFIGURABLE_CORE_SHORTCUT_IDS.some((id) => matchesShortcut(event, SHORTCUTS[id]))
  );
}

/**
 * A combo the user could not produce by ordinary typing. `shift` does not
 * count: shift+a is just a capital A.
 */
function hasNonShiftModifier(shortcut: KeyboardShortcut): boolean {
  const m = shortcut.modifiers;
  return Boolean(m && (m.ctrlOrCmd || m.ctrl || m.cmd || m.alt));
}

/**
 * Collects the namespaced ids of every declared keybinding that set
 * `allow_in_editor`, so the dispatcher can let just those through while an
 * input, textarea or contenteditable holds focus.
 */
function buildAllowInEditorSet(
  plugins: Parameters<typeof buildConfigurableShortcutEntries>[0],
): Set<string> {
  const ids = new Set<string>();
  for (const plugin of plugins) {
    for (const keybinding of plugin.ui?.keybindings ?? []) {
      if (keybinding.allow_in_editor) ids.add(`plugin:${plugin.id}:${keybinding.id}`);
    }
  }
  return ids;
}

/**
 * Dispatches `event` to every registered plugin keybinding handler whose
 * effective combo matches. Iterates in `pluginRegistry.getKeybindingHandlers()`
 * order — registration order — so when two plugins bind the same combo, the
 * plugin that called `registerKeybinding` first runs first (both still run;
 * this only fixes the order, it does not stop at the first match).
 *
 * Events accepted by an effective core shortcut are reserved without invoking
 * or preventing a plugin handler. Events outside that core reservation remain
 * eligible even when the plugin binding overlaps it on another event.
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
  targetIsEditable: boolean,
): void {
  const entryById = new Map(
    buildConfigurableShortcutEntries(plugins)
      .filter((entry) => entry.source === "plugin")
      .map((entry) => [entry.id, entry] as const),
  );
  const allowedInEditor = buildAllowInEditorSet(plugins);

  const reservedByCore = isCoreShortcutEvent(event, overrides);

  for (const { pluginId, id, handler } of pluginRegistry.getKeybindingHandlers()) {
    const entry = entryById.get(`plugin:${pluginId}:${id}`);
    if (!entry) continue;
    // Skipping while the user is typing is the default so a plugin can never
    // shadow an ordinary keystroke. A binding that exists to act on the
    // focused composer opts in through the manifest.
    if (targetIsEditable && !allowedInEditor.has(`plugin:${pluginId}:${id}`)) continue;

    const shortcut = resolveShortcutEntry(entry, overrides);
    // The manifest validator requires a ctrl/cmd/mod/alt modifier for an
    // editor-enabled binding, but the effective combo may be a user override
    // and the shortcut recorder accepts a bare key. Re-check the resolved
    // combo, or a rebind to plain "m" would eat every m the user types.
    if (targetIsEditable && !hasNonShiftModifier(shortcut)) continue;
    if (!matchesShortcut(event, shortcut)) continue;

    if (reservedByCore) {
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
