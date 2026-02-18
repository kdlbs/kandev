import { useEffect, useRef, type RefObject } from 'react';

type CaptureKeydownShortcut = {
  key: string;
  metaOrCtrl?: boolean;
  alt?: boolean;
  shift?: boolean;
};

/**
 * Attach a capture-phase keydown listener to a wrapper element.
 * Useful for intercepting keyboard shortcuts before embedded editors
 * (Monaco, CodeMirror) consume them.
 */
export function useCaptureKeydown(
  ref: RefObject<HTMLElement | null>,
  shortcut: CaptureKeydownShortcut,
  callback: () => void,
): void {
  const callbackRef = useRef(callback);
  useEffect(() => { callbackRef.current = callback; }, [callback]);

  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const handler = (e: KeyboardEvent) => {
      const metaOrCtrl = shortcut.metaOrCtrl ? (e.metaKey || e.ctrlKey) : true;
      const alt = shortcut.alt ? e.altKey : !e.altKey;
      const shift = shortcut.shift ? e.shiftKey : !e.shiftKey;
      if (metaOrCtrl && alt && shift && e.key === shortcut.key) {
        e.preventDefault();
        e.stopPropagation();
        callbackRef.current();
      }
    };
    el.addEventListener('keydown', handler, true);
    return () => el.removeEventListener('keydown', handler, true);
  }, [ref, shortcut.key, shortcut.metaOrCtrl, shortcut.alt, shortcut.shift]);
}
