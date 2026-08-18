"use client";

import { useCallback, useMemo, useSyncExternalStore } from "react";

/**
 * Broadcast event for per-session queue pin changes. Shared across sessions:
 * listeners re-read their own session's key when it fires, so the event
 * itself carries no payload.
 */
export const QUEUE_PIN_SYNC_EVENT = "kandev:queue:pinned-changed";

export function queuePinStorageKey(sessionId: string): string {
  return `kandev:queue:pinned:${sessionId}:v1`;
}

/**
 * In-memory fallback for when localStorage writes fail (private mode /
 * quota). The toggle must still flip the current view's pin even though
 * nothing is persisted; the value lives for this tab only and is cleared by
 * the next successful write.
 */
const volatilePins = new Map<string, boolean>();

/** Reads the pin flag, treating only the literal string "true" as on. */
function readStoredPin(sessionId: string): boolean {
  const volatilePin = volatilePins.get(sessionId);
  if (volatilePin !== undefined) return volatilePin;
  if (typeof window === "undefined") return false;
  try {
    return window.localStorage.getItem(queuePinStorageKey(sessionId)) === "true";
  } catch {
    // Quota / private mode — fall through; the pin just defaults to off.
    return false;
  }
}

/**
 * Per-session "keep the message queue panel open across navigation" pin,
 * persisted in `localStorage` with the `useSyncExternalStore` + `storage`
 * event + custom-event broadcast shape shared by the install-wide boolean
 * preferences. Pins are isolated per session via a session-scoped key; a
 * `null` session degrades to an unpinned value without touching storage.
 *
 * Read failures degrade to `false`; `setValue` throws on a failed write while
 * `toggle` swallows it, so the UI button keeps working for the current view
 * when persistence is unavailable (private mode / quota).
 */
export function useQueuePinned(sessionId: string | null) {
  const subscribe = useMemo(
    () => (notify: () => void) => {
      const onCustomEvent = () => notify();
      window.addEventListener(QUEUE_PIN_SYNC_EVENT, onCustomEvent);
      window.addEventListener("storage", onCustomEvent);
      return () => {
        window.removeEventListener(QUEUE_PIN_SYNC_EVENT, onCustomEvent);
        window.removeEventListener("storage", onCustomEvent);
      };
    },
    [],
  );

  const getSnapshot = useCallback(
    () => (sessionId === null ? false : readStoredPin(sessionId)),
    [sessionId],
  );
  const getServerSnapshot = useCallback(() => false, []);

  const value = useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot);

  const setValue = useCallback(
    (next: boolean) => {
      if (sessionId === null) return;
      try {
        window.localStorage.setItem(queuePinStorageKey(sessionId), String(next));
        // A successful write supersedes any volatile fallback.
        volatilePins.delete(sessionId);
      } catch (error) {
        throw new Error(`Failed to persist queue pin for ${sessionId}`, { cause: error });
      }
      window.dispatchEvent(new Event(QUEUE_PIN_SYNC_EVENT));
    },
    [sessionId],
  );

  const toggle = useCallback(() => {
    try {
      setValue(!value);
    } catch {
      // Persistence failed (private mode / quota): keep the current view's
      // pin flipped via the in-memory fallback so the toggle visibly works.
      if (sessionId !== null) {
        volatilePins.set(sessionId, !value);
        window.dispatchEvent(new Event(QUEUE_PIN_SYNC_EVENT));
      }
    }
  }, [sessionId, value, setValue]);

  return { value, setValue, toggle };
}
