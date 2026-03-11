"use client";

import { useEffect, useRef } from "react";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { useAppStoreApi } from "@/components/state-provider";
import { createUserShell } from "@/lib/api/domains/user-shell-api";
import { SHORTCUTS } from "@/lib/keyboard/constants";
import { matchesShortcut } from "@/lib/keyboard/utils";
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
  getSessionId: () => string | null,
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

  const sessionId = getSessionId();
  if (!sessionId) return;

  createUserShell(sessionId)
    .then((result) => {
      useDockviewStore.getState().addTerminalPanel(result.terminalId);
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

/** Returns true if the event matches Cmd/Ctrl + key (no shift). */
function isCmdKey(e: KeyboardEvent, code: string): boolean {
  return (e.metaKey || e.ctrlKey) && !e.shiftKey && e.code === code;
}

function handleLayoutToggle(e: KeyboardEvent): boolean {
  if (isEditableTarget(e)) return false;

  if (isCmdKey(e, "KeyB")) {
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
): boolean {
  // Cmd/Ctrl+J — toggle bottom terminal panel
  // Note: no isEditableTarget guard here. Ctrl+J is not a standard text
  // editing shortcut, and we must preventDefault even when an xterm textarea
  // is focused — otherwise the un-prevented event causes escape-sequence
  // artifacts (e.g. trailing "R" from cursor-position reports during resize).
  if (matchesShortcut(e, SHORTCUTS.BOTTOM_TERMINAL)) {
    e.preventDefault();
    e.stopPropagation();
    appStore.getState().toggleBottomTerminal();
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
  const appStore = useAppStoreApi();

  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      const api = useDockviewStore.getState().api;
      if (!api) return;

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
        handleTerminalToggle(
          e,
          api,
          previousPanelIdRef,
          () => appStore.getState().tasks.activeSessionId,
        );
        return;
      }

      if (handleLayoutToggle(e)) return;
      handleBottomTerminal(e, appStore);
    };

    // Use capture phase so we receive events before xterm.js
    window.addEventListener("keydown", handler, true);
    return () => window.removeEventListener("keydown", handler, true);
  }, [appStore]);
}
