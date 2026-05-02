"use client";

import { useCallback, useMemo, useSyncExternalStore } from "react";

// useIntegrationEnabled is the install-wide on/off toggle every third-party
// integration UI (jira, linear, future) needs: a localStorage-backed boolean
// that defaults to true, syncs across tabs via the `storage` event, and within
// a tab via a custom event the integration provides.
//
// Connection state is install-wide (one Jira account, one Linear account), so
// the toggle is too. The legacy per-workspace key is migrated transparently on
// first read.
//
// The signature takes plain string parameters rather than an options object so
// useSyncExternalStore's getSnapshot stays referentially stable across
// renders — passing an inline `{...}` would give getSnapshot a new identity
// every render and re-run the migration scan on each pass.

// migrateLegacyKey is idempotent: the first call moves any leftover
// per-workspace value onto storageKey and removes the legacy entries;
// subsequent calls are no-ops because there are no legacy keys left to scan.
// Cheap enough to run on every snapshot read (a localStorage iteration over
// kandev:* keys), so we don't need to memoize across re-renders.
function migrateLegacyKey(storageKey: string, legacyKeyPrefix: string): void {
  if (typeof window === "undefined") return;
  try {
    if (window.localStorage.getItem(storageKey) !== null) return;
    let surviving: string | null = null;
    const stale: string[] = [];
    for (let i = 0; i < window.localStorage.length; i++) {
      const key = window.localStorage.key(i);
      if (!key || !key.startsWith(legacyKeyPrefix) || key === storageKey) continue;
      stale.push(key);
      const value = window.localStorage.getItem(key);
      if (value !== null && surviving === null) surviving = value;
    }
    if (surviving !== null) {
      window.localStorage.setItem(storageKey, surviving);
    }
    for (const k of stale) window.localStorage.removeItem(k);
  } catch {
    // Quota / private mode — fall through; the toggle just defaults to on.
  }
}

function readEnabled(storageKey: string): boolean {
  if (typeof window === "undefined") return true;
  try {
    const raw = window.localStorage.getItem(storageKey);
    if (raw === null) return true;
    return raw !== "false";
  } catch {
    return true;
  }
}

export function useIntegrationEnabled(
  storageKey: string,
  legacyKeyPrefix: string,
  syncEvent: string,
) {
  // useSyncExternalStore reads localStorage on every render, but the snapshot
  // is referentially stable (a boolean) so React only re-renders when the
  // value changes. This avoids setState-in-effect warnings while still giving
  // SSR a deterministic default and post-mount hydration to the persisted
  // value.
  const subscribe = useMemo(
    () => (notify: () => void) => {
      if (typeof window === "undefined") return () => {};
      window.addEventListener("storage", notify);
      window.addEventListener(syncEvent, notify);
      return () => {
        window.removeEventListener("storage", notify);
        window.removeEventListener(syncEvent, notify);
      };
    },
    [syncEvent],
  );

  const getSnapshot = useCallback(() => {
    migrateLegacyKey(storageKey, legacyKeyPrefix);
    return readEnabled(storageKey);
  }, [storageKey, legacyKeyPrefix]);

  const getServerSnapshot = useCallback(() => true, []);

  const enabled = useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot);

  const setEnabled = useCallback(
    (next: boolean) => {
      if (typeof window === "undefined") return;
      try {
        window.localStorage.setItem(storageKey, String(next));
      } catch {
        // Quota / private mode: the write failed, so a re-read via
        // getSnapshot would still return the old value. Don't dispatch the
        // sync event — the previous in-memory fallback is gone with
        // useSyncExternalStore, so silently re-dispatching would just look
        // like a no-op flip. Surface the failure by leaving the toggle stuck
        // at its previous value; the caller can detect persistence failure
        // by observing that `enabled` did not change.
        return;
      }
      window.dispatchEvent(new Event(syncEvent));
    },
    [storageKey, syncEvent],
  );

  // `loaded` is always true with useSyncExternalStore — the snapshot is read
  // synchronously on first render. Kept in the return shape so existing
  // callers (which gated effects on `loaded`) don't need to change.
  return { enabled, setEnabled, loaded: true };
}
