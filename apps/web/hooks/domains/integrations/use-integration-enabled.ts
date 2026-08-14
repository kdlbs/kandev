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

const LEGACY_KEY_SUFFIX = ":v1";

// Prefixes already scanned this session. The scan is a full localStorage
// iteration and `getSnapshot` runs on every render, so without this a browser
// that has no legacy keys (every install since Aug 2026) would re-scan forever.
const scannedLegacyPrefixes = new Set<string>();

/** Test-only: forget which prefixes were scanned, so a case can re-run one. */
export function resetIntegrationEnabledMigrations(): void {
  scannedLegacyPrefixes.clear();
}

// The pre-`v1` keys were `kandev:<slug>:enabled:<workspaceId>:v1` — already one
// per workspace. Each is restored to that workspace's own key rather than
// folded into the install-wide one: folding is what let a single workspace's
// preference answer for every other workspace, which is the bug this scoping
// exists to fix. An id-less or unrecognized key is left alone rather than
// guessed at.
//
// Today's keys (`<storageKey>:<workspaceId>`) share the legacy prefix, so they
// are skipped explicitly — otherwise the migration would eat its own output.
function migrateLegacyKeys(storageKey: string, legacyKeyPrefix: string): void {
  if (typeof window === "undefined" || scannedLegacyPrefixes.has(storageKey)) return;
  scannedLegacyPrefixes.add(storageKey);
  try {
    // Collected before writing: mutating localStorage mid-scan shifts the
    // indices `key(i)` walks.
    const legacy: Array<{ key: string; workspaceId: string }> = [];
    for (let i = 0; i < window.localStorage.length; i++) {
      const key = window.localStorage.key(i);
      if (!key || !key.startsWith(legacyKeyPrefix) || key.startsWith(storageKey)) continue;
      const scoped = key.slice(legacyKeyPrefix.length);
      if (!scoped.endsWith(LEGACY_KEY_SUFFIX)) continue;
      const workspaceId = scoped.slice(0, -LEGACY_KEY_SUFFIX.length);
      if (workspaceId) legacy.push({ key, workspaceId });
    }
    for (const { key, workspaceId } of legacy) {
      const value = window.localStorage.getItem(key);
      const target = integrationEnabledStorageKey(storageKey, workspaceId);
      // A value the user has since set for that workspace wins over the
      // migration; the legacy entry goes either way.
      if (value !== null && window.localStorage.getItem(target) === null) {
        window.localStorage.setItem(target, value);
      }
      window.localStorage.removeItem(key);
    }
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
    migrateLegacyKeys(storageKey, legacyKeyPrefix);
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

/** The storage identity of one integration's toggle, as the reader needs it. */
export type IntegrationEnabledKeyPair = {
  storageKey: string;
  legacyKeyPrefix: string;
};

/** Reads one (integration, workspace) pair, migrating legacy keys first. */
export type IntegrationEnabledReader = (
  keys: IntegrationEnabledKeyPair,
  workspaceId?: string | null,
) => boolean;

function makeIntegrationEnabledReader(): IntegrationEnabledReader {
  return ({ storageKey, legacyKeyPrefix }, workspaceId) => {
    // The hook form migrates in its own snapshot read; a surface built only on
    // the reader (the settings tree lists every workspace and mounts no
    // `useXEnabled`) would otherwise never see a legacy value at all.
    migrateLegacyKeys(storageKey, legacyKeyPrefix);
    return readIntegrationEnabled(storageKey, workspaceId);
  };
}

/**
 * A `readIntegrationEnabled` bound to a live subscription, for surfaces that
 * read many (integration, workspace) pairs at once — the settings menu tree
 * lists every workspace, and rules of hooks forbid calling `useXEnabled` in a
 * loop over them.
 *
 * The reader is held in state and replaced on every toggle: its identity *is*
 * the change signal, which is what re-runs a consumer's memoized derivation.
 * Returning a fresh closure each render would work too, but it would defeat the
 * memo the settings menu builds its whole branch forest behind. Pass a
 * module-level constant for `syncEvents`.
 */
export function useIntegrationEnabledReader(
  syncEvents: readonly string[],
): IntegrationEnabledReader {
  const [read, setRead] = useState<IntegrationEnabledReader>(makeIntegrationEnabledReader);
  // Joined rather than spread so the effect keys off the event names' contents,
  // not the array's identity.
  const eventKey = syncEvents.join("|");

  useEffect(() => {
    if (typeof window === "undefined") return;
    const bump = () => setRead(() => makeIntegrationEnabledReader());
    const events = eventKey ? eventKey.split("|") : [];
    window.addEventListener("storage", bump);
    for (const event of events) window.addEventListener(event, bump);
    return () => {
      window.removeEventListener("storage", bump);
      for (const event of events) window.removeEventListener(event, bump);
    };
  }, [eventKey]);

  return read;
}
