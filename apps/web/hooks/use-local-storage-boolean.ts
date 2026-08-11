"use client";

import { useCallback, useMemo, useSyncExternalStore } from "react";

/** Reads a localStorage boolean, treating only the literal string "true" as on. */
function readStoredBoolean(key: string, fallback: boolean): boolean {
  if (typeof window === "undefined") return fallback;
  try {
    const stored = window.localStorage.getItem(key);
    if (stored === null) return fallback;
    return stored === "true";
  } catch {
    // Quota / private mode — fall through; the preference just defaults.
    return fallback;
  }
}

/**
 * Generic install-wide, `localStorage`-backed boolean preference with the
 * `useSyncExternalStore` + `storage` event + custom-event broadcast shape
 * shared by the nav-visibility toggles ("Hide disabled integrations / agent
 * profiles from left panel navigation"). The storage key and sync event are
 * caller-owned, so the two features cannot drift in mechanism — only in
 * identity.
 *
 * Read failures degrade to `defaultValue` (the documented default, `false`
 * for both nav settings); a failed write throws instead of reporting a
 * successful save.
 */
export function useLocalStorageBoolean(
  storageKey: string,
  syncEvent: string,
  defaultValue = false,
) {
  const subscribe = useMemo(
    () => (notify: () => void) => {
      const onCustomEvent = () => notify();
      window.addEventListener(syncEvent, onCustomEvent);
      window.addEventListener("storage", onCustomEvent);
      return () => {
        window.removeEventListener(syncEvent, onCustomEvent);
        window.removeEventListener("storage", onCustomEvent);
      };
    },
    [syncEvent],
  );

  const getSnapshot = useCallback(
    () => readStoredBoolean(storageKey, defaultValue),
    [storageKey, defaultValue],
  );
  const getServerSnapshot = useCallback(() => defaultValue, [defaultValue]);

  const value = useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot);

  const setValue = useCallback(
    (next: boolean) => {
      try {
        window.localStorage.setItem(storageKey, String(next));
      } catch (error) {
        throw new Error(`Failed to persist ${storageKey}`, { cause: error });
      }
      window.dispatchEvent(new Event(syncEvent));
    },
    [storageKey, syncEvent],
  );

  return { value, setValue };
}
