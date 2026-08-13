"use client";

import { useCallback, useEffect, useMemo, useState, useSyncExternalStore } from "react";

// useIntegrationEnabled is the per-workspace on/off toggle every third-party
// integration UI (jira, linear, future) needs: a localStorage-backed boolean
// that defaults to true, syncs across tabs via the `storage` event, and within
// a tab via a custom event the integration provides.
//
// The state is scoped to a workspace because everything else about an
// integration already is: credentials, health, and the settings page the
// slider lives on all hang off `/settings/workspaces/<id>/integrations`.
// Disabling GitHub because one workspace doesn't use it must not silence it in
// the workspaces that do.
//
// The value lives at `<storageKey>:<workspaceId>`. `storageKey` itself holds
// the pre-scoping install-wide value and is read as the seed for a workspace
// that has never been toggled, so upgrading does not silently re-enable an
// integration the user had turned off.
//
// The signature takes plain string parameters rather than an options object so
// useSyncExternalStore's getSnapshot stays referentially stable across
// renders — passing an inline `{...}` would give getSnapshot a new identity
// every render and re-run the migration scan on each pass.

// migrateLegacyKey is idempotent: the first call moves any leftover
// pre-`v1` per-workspace value onto storageKey and removes the legacy entries;
// subsequent calls are no-ops because there are no legacy keys left to scan.
// Cheap enough to run on every snapshot read (a localStorage iteration over
// kandev:* keys), so we don't need to memoize across re-renders.
//
// Today's per-workspace keys (`<storageKey>:<workspaceId>`) also start with the
// legacy prefix, so they are skipped explicitly — folding them together would
// let one workspace's toggle overwrite every other workspace's.
function migrateLegacyKey(storageKey: string, legacyKeyPrefix: string): void {
  if (typeof window === "undefined") return;
  try {
    if (window.localStorage.getItem(storageKey) !== null) return;
    let surviving: string | null = null;
    const stale: string[] = [];
    for (let i = 0; i < window.localStorage.length; i++) {
      const key = window.localStorage.key(i);
      if (!key || !key.startsWith(legacyKeyPrefix) || key.startsWith(storageKey)) continue;
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

/** Where a workspace's value lives; the bare key when no workspace is known. */
export function integrationEnabledStorageKey(
  storageKey: string,
  workspaceId?: string | null,
): string {
  return workspaceId ? `${storageKey}:${workspaceId}` : storageKey;
}

/**
 * A workspace's toggle, defaulting to the pre-scoping install-wide value and
 * then to on. Pure read: no subscription, for callers that must sample several
 * workspaces at once (see `useIntegrationEnabledReader`).
 */
export function readIntegrationEnabled(storageKey: string, workspaceId?: string | null): boolean {
  if (typeof window === "undefined") return true;
  try {
    const scoped = window.localStorage.getItem(
      integrationEnabledStorageKey(storageKey, workspaceId),
    );
    const raw = scoped ?? window.localStorage.getItem(storageKey);
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
  workspaceId?: string | null,
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
    return readIntegrationEnabled(storageKey, workspaceId);
  }, [storageKey, legacyKeyPrefix, workspaceId]);

  const getServerSnapshot = useCallback(() => true, []);

  const enabled = useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot);

  const setEnabled = useCallback(
    (next: boolean) => {
      if (typeof window === "undefined") return;
      const key = integrationEnabledStorageKey(storageKey, workspaceId);
      try {
        window.localStorage.setItem(key, String(next));
      } catch (error) {
        throw new Error(`Failed to persist ${key}`, { cause: error });
      }
      window.dispatchEvent(new Event(syncEvent));
    },
    [storageKey, syncEvent, workspaceId],
  );

  // `loaded` is always true with useSyncExternalStore — the snapshot is read
  // synchronously on first render. Kept in the return shape so existing
  // callers (which gated effects on `loaded`) don't need to change.
  return { enabled, setEnabled, loaded: true };
}

/**
 * A `readIntegrationEnabled` bound to a live subscription, for surfaces that
 * read many (integration, workspace) pairs at once — the settings menu tree
 * lists every workspace, and rules of hooks forbid calling `useXEnabled` in a
 * loop over them.
 *
 * The returned function takes a new identity whenever any of `syncEvents`
 * fires or another tab writes, which is what re-runs a consumer's memoized
 * derivation. Pass a module-level constant for `syncEvents`.
 */
export function useIntegrationEnabledReader(
  syncEvents: readonly string[],
): (storageKey: string, workspaceId?: string | null) => boolean {
  const [revision, setRevision] = useState(0);
  // Joined rather than spread so the effect keys off the event names' contents,
  // not the array's identity.
  const eventKey = syncEvents.join("|");

  useEffect(() => {
    if (typeof window === "undefined") return;
    const bump = () => setRevision((current) => current + 1);
    const events = eventKey ? eventKey.split("|") : [];
    window.addEventListener("storage", bump);
    for (const event of events) window.addEventListener(event, bump);
    return () => {
      window.removeEventListener("storage", bump);
      for (const event of events) window.removeEventListener(event, bump);
    };
  }, [eventKey]);

  return useMemo(() => {
    // `revision` is the change signal: a new identity per bump is precisely
    // what invalidates consumers' memoized sets.
    void revision;
    return (storageKey: string, workspaceId?: string | null) =>
      readIntegrationEnabled(storageKey, workspaceId);
  }, [revision]);
}
