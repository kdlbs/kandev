import { useCallback, useSyncExternalStore } from 'react';

const DIFF_FOLD_KEY = 'diff-fold-unchanged';
const DEFAULT_FOLD = true;
const FOLD_CHANGE_EVENT = 'diff-fold-change';

function getStoredFolding(): boolean {
  if (typeof window === 'undefined') return DEFAULT_FOLD;
  const stored = localStorage.getItem(DIFF_FOLD_KEY);
  if (stored === null) return DEFAULT_FOLD;
  return stored === 'true';
}

function setStoredFolding(fold: boolean): void {
  localStorage.setItem(DIFF_FOLD_KEY, String(fold));
  window.dispatchEvent(new CustomEvent(FOLD_CHANGE_EVENT, { detail: fold }));
}

/** Global folding toggle synced via localStorage + CustomEvent. */
export function useGlobalFolding(): [boolean, (fold: boolean) => void] {
  const subscribe = useCallback((callback: () => void) => {
    window.addEventListener(FOLD_CHANGE_EVENT, callback);
    window.addEventListener('storage', callback);
    return () => {
      window.removeEventListener(FOLD_CHANGE_EVENT, callback);
      window.removeEventListener('storage', callback);
    };
  }, []);
  const getSnapshot = useCallback(() => getStoredFolding(), []);
  const getServerSnapshot = useCallback(() => DEFAULT_FOLD, []);
  const foldUnchanged = useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot);
  return [foldUnchanged, setStoredFolding];
}
