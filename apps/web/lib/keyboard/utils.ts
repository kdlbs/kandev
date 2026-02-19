/**
 * Keyboard shortcut utility functions
 */

import type { KeyboardShortcut, Platform } from "./constants";

/**
 * Detect the current platform
 */
export function detectPlatform(): Platform {
  if (typeof navigator === "undefined") {
    return "unknown";
  }

  const platform = navigator.platform.toLowerCase();
  const userAgent = navigator.userAgent.toLowerCase();

  if (platform.includes("mac") || userAgent.includes("mac")) {
    return "mac";
  }

  if (platform.includes("win") || userAgent.includes("win")) {
    return "windows";
  }

  if (platform.includes("linux") || userAgent.includes("linux")) {
    return "linux";
  }

  return "unknown";
}

/**
 * Check if the current platform is Mac
 */
export function isMac(): boolean {
  return detectPlatform() === "mac";
}

/** Collect modifier display names based on platform. */
function collectModifierNames(
  modifiers: KeyboardShortcut["modifiers"],
  currentPlatform: Platform,
): string[] {
  if (!modifiers) return [];
  const parts: string[] = [];

  if (modifiers.ctrlOrCmd) {
    parts.push(currentPlatform === "mac" ? "Cmd" : "Ctrl");
    return parts;
  }

  if (modifiers.ctrl) parts.push("Ctrl");
  if (modifiers.cmd && currentPlatform === "mac") parts.push("Cmd");
  if (modifiers.alt) parts.push(currentPlatform === "mac" ? "Option" : "Alt");
  if (modifiers.shift) parts.push("Shift");
  return parts;
}

/**
 * Format a keyboard shortcut for display based on platform
 * @param shortcut - The keyboard shortcut definition
 * @param platform - Optional platform override (defaults to current platform)
 * @returns Formatted string like "Cmd+Enter" or "Ctrl+Enter"
 */
export function formatShortcut(shortcut: KeyboardShortcut, platform?: Platform): string {
  const currentPlatform = platform ?? detectPlatform();
  const parts = collectModifierNames(shortcut.modifiers, currentPlatform);
  parts.push(formatKey(shortcut.key));
  return parts.join("+");
}

/**
 * Format a key for display
 */
function formatKey(key: string): string {
  // Special keys
  const specialKeys: Record<string, string> = {
    Enter: "Enter",
    Escape: "Esc",
    " ": "Space",
    Tab: "Tab",
    Backspace: "Backspace",
    Delete: "Del",
    ArrowUp: "↑",
    ArrowDown: "↓",
    ArrowLeft: "←",
    ArrowRight: "→",
  };

  if (key in specialKeys) {
    return specialKeys[key];
  }

  // Uppercase single letters
  if (key.length === 1) {
    return key.toUpperCase();
  }

  return key;
}

/** Check if a modifier flag matches the event state. */
function modifierMatches(expected: boolean | undefined, actual: boolean): boolean {
  return !!expected === actual;
}

/**
 * Check if a keyboard event matches a shortcut definition
 * @param event - The keyboard event
 * @param shortcut - The shortcut definition to match against
 * @returns true if the event matches the shortcut
 */
export function matchesShortcut(
  event: KeyboardEvent | React.KeyboardEvent,
  shortcut: KeyboardShortcut,
): boolean {
  if (event.key !== shortcut.key) return false;

  if (!shortcut.modifiers) {
    return !event.ctrlKey && !event.metaKey && !event.altKey && !event.shiftKey;
  }

  const { ctrl, cmd, alt, shift, ctrlOrCmd } = shortcut.modifiers;

  if (ctrlOrCmd) {
    if (!event.metaKey && !event.ctrlKey) return false;
    return modifierMatches(alt, event.altKey) && modifierMatches(shift, event.shiftKey);
  }

  return (
    modifierMatches(ctrl, event.ctrlKey) &&
    modifierMatches(cmd, event.metaKey) &&
    modifierMatches(alt, event.altKey) &&
    modifierMatches(shift, event.shiftKey)
  );
}

/**
 * Convert a keyboard shortcut definition to a CodeMirror keybinding string.
 */
export function shortcutToCodeMirrorKeybinding(shortcut: KeyboardShortcut): string {
  const parts: string[] = [];

  if (shortcut.modifiers) {
    if (shortcut.modifiers.ctrlOrCmd) {
      parts.push("Mod");
    } else {
      if (shortcut.modifiers.ctrl) parts.push("Ctrl");
      if (shortcut.modifiers.cmd) parts.push("Mod");
      if (shortcut.modifiers.alt) parts.push("Alt");
      if (shortcut.modifiers.shift) parts.push("Shift");
    }
  }

  parts.push(shortcut.key);
  return parts.join("-");
}
