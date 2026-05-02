"use client";

import { useEffect, useRef } from "react";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { useAppStoreApi } from "@/components/state-provider";
import { createUserShell } from "@/lib/api/domains/user-shell-api";
import { matchesShortcut } from "@/lib/keyboard/utils";
import { getShortcut, type StoredShortcutOverrides } from "@/lib/keyboard/shortcut-overrides";
import type { DockviewApi } from "dockview-react";

function handleTabNavigation(e: KeyboardEvent, api: DockviewApi) {
  const activePanel = api.activePanel;
  if (!activePanel) return;

  const panels = activePanel.group.panels;
  if (panels.length <= 1) return;

  const currentIndex = panels.findIndex((p) => p.id === activePanel.id);
  if (currentIndex === -1) return;

  e.preventDefault();
  e.stopPropagation();

  const direction = e.code === "BracketLeft" ? -1 : 1;
  const nextIndex = (currentIndex + direction + panels.length) % panels.length;
  panels[nextIndex].api.setActive();
}

function handleTerminalToggle(
  e: KeyboardEvent,
  api: DockviewApi,
  previousPanelIdRef: React.MutableRefObject<string | null>,
  getEnvironmentId: () => string | null,
) {
  e.preventDefault();
  e.stopPropagation();

  const activePanel = api.activePanel;
  const isTerminalFocused = activePanel?.id.startsWith("terminal-") ?? false;

  if (isTerminalFocused) {
    const prevId = previousPanelIdRef.current;
    const target = prevId ? api.getPanel(prevId) : api.getPanel("chat");
    if (target) target.api.setActive();
    previousPanelIdRef.current = null;
    return;
  }

  if (activePanel) {
    previousPanelIdRef.current = activePanel.id;
  }

  const terminalPanel = api.panels.find((p) => p.id.startsWith("terminal-"));
  if (terminalPanel) {
    terminalPanel.api.setActive();
    return;
  }

  const environmentId = getEnvironmentId();
  if (!environmentId) return;

  createUserShell(environmentId)
    .then((result) => {
      useDockviewStore.getState().addTerminalPanel(result.terminalId, undefined, environmentId);
    })
    .catch((err) => {
      console.warn("Failed to create terminal shell:", err);
    });
}

/** Returns true if the active element is a text input or contenteditable. */
function isEditableTarget(e: KeyboardEvent): boolean {
  const tag = (e.target as HTMLElement)?.tagName;
  return (
    tag === "INPUT" || tag === "TEXTAREA" || (e.target as HTMLElement)?.isContentEditable === true
  );
}

function handleLayoutToggle(
  e: KeyboardEvent,
  overrides: StoredShortcutOverrides | undefined,
): boolean {
  if (isEditableTarget(e)) return false;

  if (matchesShortcut(e, getShortcut("TOGGLE_SIDEBAR", overrides))) {
    e.preventDefault();
    e.stopPropagation();
    useDockviewStore.getState().toggleSidebar();
    return true;
  }

  return false;
}

function handleBottomTerminal(
  e: KeyboardEvent,
  appStore: ReturnType<typeof useAppStoreApi>,
  previousFocusRef: React.MutableRefObject<Element | null>,
  overrides: StoredShortcutOverrides | undefined,
): boolean {
  // Toggle bottom terminal panel (default: Cmd/Ctrl+J).
  // Note: no isEditableTarget guard here. The default binding is not a standard text
  // editing shortcut, and we must preventDefault even when an xterm textarea
  // is focused — otherwise the un-prevented event causes escape-sequence
  // artifacts (e.g. trailing "R" from cursor-position reports during resize).
  if (matchesShortcut(e, getShortcut("BOTTOM_TERMINAL", overrides))) {
    e.preventDefault();
    e.stopPropagation();

    const isOpen = appStore.getState().bottomTerminal.isOpen;

    if (!isOpen) {
      // Opening: save the currently focused element to restore later
      previousFocusRef.current = document.activeElement;
    }

    appStore.getState().toggleBottomTerminal();

    if (isOpen) {
      // Closing: restore focus to the previously focused element
      const prev = previousFocusRef.current;
      if (prev instanceof HTMLElement && prev.isConnected) {
        prev.focus({ preventScroll: true });
      } else {
        // Fallback: focus the chat panel
        const api = useDockviewStore.getState().api;
        const chatPanel = api?.getPanel("chat");
        if (chatPanel) chatPanel.api.setActive();
      }
      previousFocusRef.current = null;
    }

    return true;
  }

  return false;
}

/**
 * Global editor keybinds for dockview:
 * - Cmd/Ctrl+Shift+[ / ] — navigate prev/next tab in active group
 * - Ctrl+` — toggle terminal focus
 * - Cmd/Ctrl+B — toggle sidebar
 * - Cmd/Ctrl+J — toggle bottom terminal panel
 */
export function useEditorKeybinds() {
  const previousPanelIdRef = useRef<string | null>(null);
  const previousFocusRef = useRef<Element | null>(null);
  const appStore = useAppStoreApi();

  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      const api = useDockviewStore.getState().api;
      if (!api) return;

      const overrides = appStore.getState().userSettings.keyboardShortcuts;

      const isTabNav =
        (e.metaKey || e.ctrlKey) &&
        e.shiftKey &&
        (e.code === "BracketLeft" || e.code === "BracketRight");

      if (isTabNav) {
        handleTabNavigation(e, api);
        return;
      }

      const isTerminalToggle = e.ctrlKey && !e.metaKey && !e.shiftKey && e.code === "Backquote";

      if (isTerminalToggle) {
        handleTerminalToggle(e, api, previousPanelIdRef, () => {
          const state = appStore.getState();
          const sid = state.tasks.activeSessionId;
          return sid ? (state.environmentIdBySessionId[sid] ?? null) : null;
        });
        return;
      }

      if (handleLayoutToggle(e, overrides)) return;
      handleBottomTerminal(e, appStore, previousFocusRef, overrides);
    };

    // Use capture phase so we receive events before xterm.js
    window.addEventListener("keydown", handler, true);
    return () => window.removeEventListener("keydown", handler, true);
  }, [appStore]);
}
