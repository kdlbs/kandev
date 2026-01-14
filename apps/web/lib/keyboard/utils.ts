/**
 * Keyboard shortcut utility functions
 */

import type { KeyboardShortcut, Platform } from './constants';

/**
 * Detect the current platform
 */
export function detectPlatform(): Platform {
  if (typeof navigator === 'undefined') {
    return 'unknown';
  }

  const platform = navigator.platform.toLowerCase();
  const userAgent = navigator.userAgent.toLowerCase();

  if (platform.includes('mac') || userAgent.includes('mac')) {
    return 'mac';
  }

  if (platform.includes('win') || userAgent.includes('win')) {
    return 'windows';
  }

  if (platform.includes('linux') || userAgent.includes('linux')) {
    return 'linux';
  }

  return 'unknown';
}

/**
 * Check if the current platform is Mac
 */
export function isMac(): boolean {
  return detectPlatform() === 'mac';
}

/**
 * Format a keyboard shortcut for display based on platform
 * @param shortcut - The keyboard shortcut definition
 * @param platform - Optional platform override (defaults to current platform)
 * @returns Formatted string like "Cmd+Enter" or "Ctrl+Enter"
 */
export function formatShortcut(shortcut: KeyboardShortcut, platform?: Platform): string {
  const currentPlatform = platform ?? detectPlatform();
  const parts: string[] = [];

  if (shortcut.modifiers) {
    // Handle ctrlOrCmd - use Cmd on Mac, Ctrl elsewhere
    if (shortcut.modifiers.ctrlOrCmd) {
      parts.push(currentPlatform === 'mac' ? 'Cmd' : 'Ctrl');
    } else {
      // Handle individual modifiers
      if (shortcut.modifiers.ctrl) {
        parts.push('Ctrl');
      }
      if (shortcut.modifiers.cmd && currentPlatform === 'mac') {
        parts.push('Cmd');
      }
      if (shortcut.modifiers.alt) {
        parts.push(currentPlatform === 'mac' ? 'Option' : 'Alt');
      }
      if (shortcut.modifiers.shift) {
        parts.push('Shift');
      }
    }
  }

  // Format the key
  const key = formatKey(shortcut.key);
  parts.push(key);

  return parts.join('+');
}

/**
 * Format a key for display
 */
function formatKey(key: string): string {
  // Special keys
  const specialKeys: Record<string, string> = {
    Enter: 'Enter',
    Escape: 'Esc',
    ' ': 'Space',
    Tab: 'Tab',
    Backspace: 'Backspace',
    Delete: 'Del',
    ArrowUp: '↑',
    ArrowDown: '↓',
    ArrowLeft: '←',
    ArrowRight: '→',
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

/**
 * Check if a keyboard event matches a shortcut definition
 * @param event - The keyboard event
 * @param shortcut - The shortcut definition to match against
 * @returns true if the event matches the shortcut
 */
export function matchesShortcut(
  event: KeyboardEvent | React.KeyboardEvent,
  shortcut: KeyboardShortcut
): boolean {
  // Check if the key matches
  if (event.key !== shortcut.key) {
    return false;
  }

  // If no modifiers specified, ensure no modifiers are pressed
  if (!shortcut.modifiers) {
    return !event.ctrlKey && !event.metaKey && !event.altKey && !event.shiftKey;
  }

  const { ctrl, cmd, alt, shift, ctrlOrCmd } = shortcut.modifiers;

  // Handle ctrlOrCmd
  if (ctrlOrCmd) {
    const hasCorrectModifier = event.metaKey || event.ctrlKey;
    if (!hasCorrectModifier) return false;

    // Ensure other modifiers match
    if (alt && !event.altKey) return false;
    if (!alt && event.altKey) return false;
    if (shift && !event.shiftKey) return false;
    if (!shift && event.shiftKey) return false;

    return true;
  }

  // Check individual modifiers
  if (ctrl && !event.ctrlKey) return false;
  if (!ctrl && event.ctrlKey) return false;
  if (cmd && !event.metaKey) return false;
  if (!cmd && event.metaKey) return false;
  if (alt && !event.altKey) return false;
  if (!alt && event.altKey) return false;
  if (shift && !event.shiftKey) return false;
  if (!shift && event.shiftKey) return false;

  return true;
}

