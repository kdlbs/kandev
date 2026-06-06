"use client";

import { useEffect } from "react";
import { useAppStoreApi } from "@/components/state-provider";
import { matchesShortcut } from "@/lib/keyboard/utils";
import { getShortcut } from "@/lib/keyboard/shortcut-overrides";

/** Returns true if the active element is a text input or contenteditable. */
function isEditableTarget(e: KeyboardEvent): boolean {
  const tag = (e.target as HTMLElement)?.tagName;
  return (
    tag === "INPUT" || tag === "TEXTAREA" || (e.target as HTMLElement)?.isContentEditable === true
  );
}

/**
 * App-root keyboard shortcuts that must fire on every route — not just inside
 * the dockview session editor.
 *
 * Currently handles `TOGGLE_SIDEBAR` (collapse/expand the global AppSidebar).
 * Previously this lived in {@link useEditorKeybinds}, which is mounted only by
 * the dockview desktop layout, so the shortcut was dead on the Kanban board,
 * task list, Office, Settings, etc. Mount this once near the app root (it has
 * no dockview dependency) so the binding works everywhere.
 *
 * The AppSidebar is desktop-only (`hidden md:flex`); on mobile the toggle still
 * flips store state but has no visible effect, which is fine.
 */
export function useAppShortcuts() {
  const appStore = useAppStoreApi();

  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (isEditableTarget(e)) return;

      const overrides = appStore.getState().userSettings.keyboardShortcuts;
      if (matchesShortcut(e, getShortcut("TOGGLE_SIDEBAR", overrides))) {
        e.preventDefault();
        e.stopPropagation();
        appStore.getState().toggleAppSidebar();
      }
    };

    // Capture phase so we win the event before focus-trapped surfaces (e.g.
    // xterm.js) can swallow it — mirrors useEditorKeybinds.
    window.addEventListener("keydown", handler, true);
    return () => window.removeEventListener("keydown", handler, true);
  }, [appStore]);
}
