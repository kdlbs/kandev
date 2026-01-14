/**
 * React hook for handling keyboard shortcuts
 */

import { useEffect, useCallback, useRef } from 'react';
import type { KeyboardShortcut } from '@/lib/keyboard/constants';
import { matchesShortcut } from '@/lib/keyboard/utils';

export type KeyboardShortcutOptions = {
  /** Whether the shortcut is enabled (default: true) */
  enabled?: boolean;
  /** Whether to prevent default behavior (default: true) */
  preventDefault?: boolean;
  /** Whether to stop event propagation (default: false) */
  stopPropagation?: boolean;
  /** Target element (default: window) */
  target?: HTMLElement | Window | null;
};

/**
 * Hook for handling global keyboard shortcuts
 * 
 * @example
 * ```tsx
 * useKeyboardShortcut(SHORTCUTS.SUBMIT, handleSubmit);
 * ```
 */
export function useKeyboardShortcut(
  shortcut: KeyboardShortcut,
  callback: (event: KeyboardEvent) => void,
  options: KeyboardShortcutOptions = {}
) {
  const {
    enabled = true,
    preventDefault = true,
    stopPropagation = false,
    target = typeof window !== 'undefined' ? window : null,
  } = options;

  // Use ref to avoid recreating the handler on every render
  const callbackRef = useRef(callback);
  callbackRef.current = callback;

  const handleKeyDown = useCallback(
    (event: KeyboardEvent) => {
      if (!enabled) return;

      if (matchesShortcut(event, shortcut)) {
        if (preventDefault) {
          event.preventDefault();
        }
        if (stopPropagation) {
          event.stopPropagation();
        }
        callbackRef.current(event);
      }
    },
    [enabled, preventDefault, stopPropagation, shortcut]
  );

  useEffect(() => {
    if (!enabled || !target) return;

    target.addEventListener('keydown', handleKeyDown as EventListener);

    return () => {
      target.removeEventListener('keydown', handleKeyDown as EventListener);
    };
  }, [enabled, target, handleKeyDown]);
}

/**
 * Hook for handling keyboard shortcuts on a specific element (e.g., textarea)
 * Returns a handler to attach to onKeyDown
 *
 * @example
 * ```tsx
 * const handleKeyDown = useKeyboardShortcutHandler(SHORTCUTS.SUBMIT, handleSubmit);
 *
 * return <textarea onKeyDown={handleKeyDown} />;
 * ```
 */
export function useKeyboardShortcutHandler(
  shortcut: KeyboardShortcut,
  callback: (event: React.KeyboardEvent) => void,
  options: Omit<KeyboardShortcutOptions, 'target'> = {}
) {
  const {
    enabled = true,
    preventDefault = true,
    stopPropagation = false,
  } = options;

  // Use ref to avoid recreating the handler on every render
  const callbackRef = useRef(callback);

  // Update ref in useEffect to avoid updating during render
  useEffect(() => {
    callbackRef.current = callback;
  }, [callback]);

  return useCallback(
    (event: React.KeyboardEvent) => {
      if (!enabled) return;

      if (matchesShortcut(event, shortcut)) {
        if (preventDefault) {
          event.preventDefault();
        }
        if (stopPropagation) {
          event.stopPropagation();
        }
        callbackRef.current(event);
      }
    },
    [enabled, preventDefault, stopPropagation, shortcut]
  );
}

