import { useEffect, useRef, useState } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { listSessionTurns } from "@/lib/api/domains/session-api";
import type { Turn } from "@/lib/types/http";

const EMPTY_TURNS: Turn[] = [];
let fetchSequence = 0;
const lastAppliedSequence = new Map<string, number>();
/** Bounded retries with backoff for transient turn-fetch failures. */
const MAX_TURN_FETCH_ATTEMPTS = 3;
/** Slow recovery poll once the bounded budget is exhausted: keeps retrying
 *  while the session stays mounted so a later reconnect can still hydrate,
 *  instead of stranding durations until a remount or session switch. */
const SLOW_RETRY_INTERVAL_MS = 30_000;
/** Bounds a single turn fetch so a hung request (never settling) cannot stop
 *  hydration forever — the timeout aborts it and routes it into the retry
 *  path like any other transient failure. */
const TURN_FETCH_TIMEOUT_MS = 15_000;

export type SessionTurnsState = {
  turns: Turn[];
  /** True when the initial server snapshot has been merged into the store. */
  isHydrated: boolean;
};

/** Subscribe to a session's turns and expose readiness for derived views. */
export function useSessionTurnsState(sessionId: string | null): SessionTurnsState {
  const turns = useAppStore((state) => (sessionId ? state.turns.bySession[sessionId] : undefined));
  const loaded = useAppStore((state) =>
    sessionId ? Boolean(state.turns.loadedBySession[sessionId]) : true,
  );
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const store = useAppStoreApi();
  const activeGeneration = useRef({ sessionId: activeSessionId, value: 0 });
  const [retryTick, setRetryTick] = useState(0);
  const failedAttemptsRef = useRef(0);

  useEffect(() => {
    // A new session starts a fresh retry budget.
    failedAttemptsRef.current = 0;
  }, [sessionId]);

  useEffect(() => {
    if (activeGeneration.current.sessionId === activeSessionId) return;
    activeGeneration.current = {
      sessionId: activeSessionId,
      value: activeGeneration.current.value + 1,
    };
  }, [activeSessionId]);

  useEffect(() => {
    if (!sessionId || loaded) return;
    const generation = activeGeneration.current.value;
    const sequence = ++fetchSequence;
    // Capture the reconciliation epoch when the fetch starts: an
    // authoritative active-marker clear that lands mid-flight must not be
    // resurrected by this hydration (see mergeTurnsSnapshot).
    const hydrationEpoch = store.getState().turns.reconcileEpochBySession[sessionId] ?? 0;
    let disposed = false;
    const controller = new AbortController();
    let retryTimer: number | undefined;
    let slowRetryTimer: number | undefined;
    /** True when the fetch-timeout aborted the request (as opposed to a
     *  cleanup abort), so the rejection is retried rather than ignored. */
    let timedOut = false;
    const timeoutTimer = window.setTimeout(() => {
      timedOut = true;
      controller.abort();
    }, TURN_FETCH_TIMEOUT_MS);
    /** Immediate recovery trigger: reconnect or returning to the foreground
     *  restarts the fetch while the session is still unhydrated. */
    const wake = () => setRetryTick((tick) => tick + 1);
    window.addEventListener("online", wake);
    const onVisibility = () => {
      if (document.visibilityState === "visible") wake();
    };
    document.addEventListener("visibilitychange", onVisibility);

    void listSessionTurns(sessionId, { init: { signal: controller.signal } })
      .then(({ turns: fetchedTurns }) => {
        if (disposed || activeGeneration.current.value !== generation) return;
        const appliedSequence = lastAppliedSequence.get(sessionId) ?? 0;
        if (sequence < appliedSequence) return;
        failedAttemptsRef.current = 0;
        lastAppliedSequence.set(sessionId, sequence);
        store.getState().mergeTurnsSnapshot(sessionId, fetchedTurns, hydrationEpoch);
      })
      .catch((error: unknown) => {
        if (disposed) return;
        if (controller.signal.aborted && !timedOut) return;
        const attempts = failedAttemptsRef.current + 1;
        failedAttemptsRef.current = attempts;
        console.error("[useSessionTurns] failed to fetch turns for", sessionId, error);
        if (attempts < MAX_TURN_FETCH_ATTEMPTS) {
          retryTimer = window.setTimeout(
            () => setRetryTick((tick) => tick + 1),
            2000 * 2 ** attempts,
          );
        } else {
          // Budget exhausted: fall back to a slow recovery poll instead of
          // stranding the session unhydrated until a remount/switch.
          slowRetryTimer = window.setTimeout(
            () => setRetryTick((tick) => tick + 1),
            SLOW_RETRY_INTERVAL_MS,
          );
        }
      });

    return () => {
      disposed = true;
      controller.abort();
      window.clearTimeout(timeoutTimer);
      if (retryTimer !== undefined) window.clearTimeout(retryTimer);
      if (slowRetryTimer !== undefined) window.clearTimeout(slowRetryTimer);
      window.removeEventListener("online", wake);
      document.removeEventListener("visibilitychange", onVisibility);
      // Clear the stale sequence guard when a session is abandoned before
      // hydration, so a re-created same-ID session cannot be poisoned. Fully
      // hydrated sessions keep the entry — session IDs are UUIDs, so reuse is
      // not a production concern.
      if (!store.getState().turns.loadedBySession[sessionId]) {
        lastAppliedSequence.delete(sessionId);
      }
    };
  }, [sessionId, loaded, store, activeSessionId, retryTick]);

  return {
    turns: loaded ? (turns ?? EMPTY_TURNS) : EMPTY_TURNS,
    isHydrated: loaded,
  };
}

/** Backward-compatible turns-only selector for callers that do not need readiness. */
export function useSessionTurns(sessionId: string | null): Turn[] {
  return useSessionTurnsState(sessionId).turns;
}
