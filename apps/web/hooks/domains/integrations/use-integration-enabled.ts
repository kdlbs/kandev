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
export type IntegrationEnabledOptions = {
  // Install-wide localStorage key, e.g. `kandev:jira:enabled:v1`.
  storageKey: string;
  // Pre-singleton key prefix used to hunt for any leftover per-workspace
  // entry on first read. The lone surviving value seeds the new global key.
  // Example: `kandev:jira:enabled:` matched the old keys
  // `kandev:jira:enabled:<workspaceId>:v1`.
  legacyKeyPrefix: string;
  // Custom event fired on the window when the toggle changes within a tab.
  // The `storage` event only fires across tabs, so each integration needs
  // its own intra-tab signal — different integrations get different events
  // so toggling one doesn't re-render every consumer of the others.
  syncEvent: string;
};

// migrateLegacyKey is idempotent: the first call moves any leftover
// per-workspace value onto opts.storageKey and removes the legacy entries;
// subsequent calls are no-ops because there are no legacy keys left to scan.
// Cheap enough to run on every snapshot read (a localStorage iteration over
// kandev:* keys), so we don't need to memoize across re-renders.
function migrateLegacyKey(opts: IntegrationEnabledOptions): void {
  if (typeof window === "undefined") return;
  try {
    if (window.localStorage.getItem(opts.storageKey) !== null) return;
    let surviving: string | null = null;
    const stale: string[] = [];
    for (let i = 0; i < window.localStorage.length; i++) {
      const key = window.localStorage.key(i);
      if (!key || !key.startsWith(opts.legacyKeyPrefix) || key === opts.storageKey) continue;
      stale.push(key);
      const value = window.localStorage.getItem(key);
      if (value !== null && surviving === null) surviving = value;
    }
    if (surviving !== null) {
      window.localStorage.setItem(opts.storageKey, surviving);
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

export function useIntegrationEnabled(opts: IntegrationEnabledOptions) {
  // useSyncExternalStore reads localStorage on every render, but the snapshot
  // is referentially stable (a boolean) so React only re-renders when the
  // value changes. This avoids setState-in-effect warnings while still giving
  // SSR a deterministic default and post-mount hydration to the persisted
  // value.
  const subscribe = useMemo(() => {
    return (notify: () => void) => {
      if (typeof window === "undefined") return () => {};
      window.addEventListener("storage", notify);
      window.addEventListener(opts.syncEvent, notify);
      return () => {
        window.removeEventListener("storage", notify);
        window.removeEventListener(opts.syncEvent, notify);
      };
    };
  }, [opts.syncEvent]);

  const getSnapshot = useCallback(() => {
    migrateLegacyKey(opts);
    return readEnabled(opts.storageKey);
  }, [opts]);

  const getServerSnapshot = useCallback(() => true, []);

  const enabled = useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot);

  const setEnabled = useCallback(
    (next: boolean) => {
      if (typeof window === "undefined") return;
      try {
        window.localStorage.setItem(opts.storageKey, String(next));
        window.dispatchEvent(new Event(opts.syncEvent));
      } catch {
        // Quota or private mode: dispatching the event re-reads via the snapshot
        // function on the next render so callers see the in-memory value flip.
        window.dispatchEvent(new Event(opts.syncEvent));
      }
    },
    [opts],
  );

  // `loaded` is always true with useSyncExternalStore — the snapshot is read
  // synchronously on first render. Kept in the return shape so existing
  // callers (which gated effects on `loaded`) don't need to change.
  return { enabled, setEnabled, loaded: true };
}
