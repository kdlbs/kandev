"use client";

import { useCallback, useMemo, useSyncExternalStore } from "react";

const STORAGE_KEY = "kandev:agents:collapsedBlocks:v1";
const SYNC_EVENT = "kandev:agents:collapsed-blocks-changed";

type CollapsedBlocks = Record<string, boolean>;

/**
 * Session-only collapse overrides for agents whose write failed (quota /
 * private mode). Keyed by agent name; an entry lives until the next
 * successful write for that agent, which makes storage authoritative again.
 * Module-level on purpose: every use of the hook in this document shares it,
 * matching the per-browser scope of the preference.
 */
const sessionOverrides = new Map<string, boolean>();

/** Reads the stored collapse record as a raw string; absent/read-failing storage
 *  degrades to the empty record (all expanded). */
function readStoredBlocksRaw(): string {
  if (typeof window === "undefined") return "";
  try {
    return window.localStorage.getItem(STORAGE_KEY) ?? "";
  } catch {
    return "";
  }
}

/** Parses a raw stored record; invalid JSON and non-objects become `{}`. */
function parseBlocks(raw: string): CollapsedBlocks {
  if (raw === "") return {};
  try {
    const parsed = JSON.parse(raw) as unknown;
    if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) return {};
    return parsed as CollapsedBlocks;
  } catch {
    return {};
  }
}

/** Parses the session-override JSON fragment; invalid input becomes `{}`. */
function parseOverrides(raw: string): CollapsedBlocks {
  try {
    const parsed = JSON.parse(raw) as unknown;
    if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) return {};
    return parsed as CollapsedBlocks;
  } catch {
    return {};
  }
}

/**
 * The `useSyncExternalStore` snapshot: the stored record plus a signature of
 * the session overrides, joined with a NUL. A primitive string, so unchanged
 * storage yields a stable snapshot (no infinite re-render loop) while a
 * changed override changes it. JSON text never contains a literal NUL
 * (`JSON.stringify` escapes control characters), so the split is unambiguous.
 */
function readSnapshot(): string {
  const raw = readStoredBlocksRaw();
  if (sessionOverrides.size === 0) return raw;
  return `${raw}\u0000${JSON.stringify(Object.fromEntries(sessionOverrides))}`;
}

/** Splits a snapshot into the stored record and the session overrides. */
function parseSnapshot(snapshot: string): { stored: CollapsedBlocks; overrides: CollapsedBlocks } {
  const sep = snapshot.indexOf("\u0000");
  if (sep === -1) return { stored: parseBlocks(snapshot), overrides: {} };
  return {
    stored: parseBlocks(snapshot.slice(0, sep)),
    overrides: parseOverrides(snapshot.slice(sep + 1)),
  };
}

/**
 * Whether an agent is collapsed: a session override wins while present, then
 * the stored record (only the literal `true` counts).
 */
function isCollapsed(snapshot: string, agentName: string): boolean {
  const { stored, overrides } = parseSnapshot(snapshot);
  return agentName in overrides ? overrides[agentName] === true : stored[agentName] === true;
}

/**
 * Per-agent collapsed/expanded preference for the Settings > Agents installed
 * agent cards, persisted as one JSON record in `localStorage` keyed by agent
 * name (absent entry = expanded). Uses the same
 * `useSyncExternalStore` + `storage` event + custom-event broadcast shape as
 * the shared `useLocalStorageBoolean` primitive, so every tab re-renders when
 * the preference changes anywhere. Read failures degrade to expanded. A failed
 * write still applies the toggle for the session (via `sessionOverrides`, so
 * the user is never stuck in a state they cannot toggle out of) but throws
 * instead of reporting a successful save.
 */
export function useCollapsedAgentBlocks() {
  const subscribe = useMemo(
    () => (notify: () => void) => {
      const onCustomEvent = () => notify();
      // Skip foreign-key storage events: they cannot change this snapshot, so
      // notifying would only re-run `getSnapshot` to read the same key.
      const onStorageEvent = (event: Event) => {
        if (event instanceof StorageEvent && event.key !== null && event.key !== STORAGE_KEY) {
          return;
        }
        notify();
      };
      window.addEventListener(SYNC_EVENT, onCustomEvent);
      window.addEventListener("storage", onStorageEvent);
      return () => {
        window.removeEventListener(SYNC_EVENT, onCustomEvent);
        window.removeEventListener("storage", onStorageEvent);
      };
    },
    [],
  );

  const getSnapshot = useCallback(() => readSnapshot(), []);
  const getServerSnapshot = useCallback(() => "", []);

  const snapshot = useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot);

  const collapsed = useCallback(
    (agentName: string) => isCollapsed(snapshot, agentName),
    [snapshot],
  );

  const setCollapsed = useCallback((agentName: string, next: boolean) => {
    // Apply in memory first so the toggle always works, even when the write
    // fails (quota / private mode): the user must never be stuck in a state
    // they cannot toggle out of. A successful write makes storage
    // authoritative again and drops the override.
    sessionOverrides.set(agentName, next);
    let failed = false;
    try {
      const stored = parseBlocks(readStoredBlocksRaw());
      window.localStorage.setItem(STORAGE_KEY, JSON.stringify({ ...stored, [agentName]: next }));
      sessionOverrides.delete(agentName);
    } catch {
      failed = true;
    }
    // Notify before throwing so the applied in-memory state renders.
    window.dispatchEvent(new Event(SYNC_EVENT));
    if (failed) {
      throw new Error(`Failed to persist ${STORAGE_KEY}`);
    }
  }, []);

  return { collapsed, setCollapsed };
}
