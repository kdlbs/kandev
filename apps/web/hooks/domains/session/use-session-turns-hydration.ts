import { listSessionTurns } from "@/lib/api/domains/session-api";
import { useAppStoreApi } from "@/components/state-provider";
import { createDebugLogger } from "@/lib/debug/log";

const debug = createDebugLogger("messages:fetch:turns");

// Single-flight promises for in-progress turn hydration per session. The
// loaded marker lives in the store (`turns.loadedBySession`) because it must
// be seeded by SSR hydration too; the module maps deduplicate in-flight work
// and associate a successful load with the current subscription readiness.
const inFlightTurnsLoad = new Map<string, Promise<void>>();
const lastHydrationReadiness = new Map<string, Promise<void> | undefined>();

// Transient failures (network blip, backend restart) must not leave a
// session's turn metadata unresolved indefinitely: message loading proceeds
// either way, so without a retry the regression stays visible until an
// unrelated lifecycle event (visibility change, reconnect, session switch)
// triggers another fetch. Retry a bounded number of times with backoff; the
// in-flight entry covers the retry window. After exhaustion a DELAYED
// recovery hydration is scheduled (see scheduleTurnHydrationRecovery), so a
// session whose turn history keeps failing resolves as soon as the backend
// recovers — no unrelated trigger required.
const TURN_HYDRATION_MAX_ATTEMPTS = 3;
const TURN_HYDRATION_RETRY_BASE_MS = 250;
const TURN_HYDRATION_RETRY_MAX_MS = 1_000;

// Prolonged outages: the recovery delay doubles per schedule (capped) so a
// long outage retries occasionally instead of hammering the endpoint, and
// only one recovery timer per session is ever pending.
const TURN_HYDRATION_RECOVERY_BASE_MS = 30_000;
const TURN_HYDRATION_RECOVERY_MAX_MS = 300_000;
const recoveryTimers = new Map<string, ReturnType<typeof setTimeout>>();
const recoveryDelayBySession = new Map<string, number>();

/** Test seam: drop in-flight entries and pending recovery timers so tests
 * don't leak dedup state or scheduled work. */
export function clearInFlightTurnsLoadForTest(): void {
  inFlightTurnsLoad.clear();
  lastHydrationReadiness.clear();
  for (const timer of recoveryTimers.values()) clearTimeout(timer);
  recoveryTimers.clear();
  recoveryDelayBySession.clear();
}

/**
 * Schedules a delayed recovery hydration after retry exhaustion, so a
 * session whose turn-history fetch keeps failing does not stay broken until
 * an unrelated lifecycle event happens to trigger another fetch. No-op when
 * a recovery is already scheduled, the loaded marker was set meanwhile
 * (hydration succeeded), or the session is gone. When the timer fires it
 * re-enters ensureSessionTurnsLoaded; on success the marker is set and the
 * chain stops. Backoff doubles per schedule (capped at
 * TURN_HYDRATION_RECOVERY_MAX_MS); the per-session delay entry is a bounded
 * bookkeeping value that is only consulted while the session stays unloaded.
 */
function scheduleTurnHydrationRecovery(
  sessionId: string,
  store: ReturnType<typeof useAppStoreApi>,
): void {
  if (recoveryTimers.has(sessionId)) return;
  const state = store.getState();
  if (state.turns.loadedBySession[sessionId] || !state.taskSessions.items[sessionId]) return;
  const delay = Math.min(
    recoveryDelayBySession.get(sessionId) ?? TURN_HYDRATION_RECOVERY_BASE_MS,
    TURN_HYDRATION_RECOVERY_MAX_MS,
  );
  recoveryDelayBySession.set(sessionId, Math.min(delay * 2, TURN_HYDRATION_RECOVERY_MAX_MS));
  const timer = setTimeout(() => {
    recoveryTimers.delete(sessionId);
    void ensureSessionTurnsLoaded(sessionId, store);
  }, delay);
  recoveryTimers.set(sessionId, timer);
}

/**
 * One hydration attempt: fetch the session's full persisted turn history and
 * merge it into the store. Returns true on success (marker set) and false on
 * a transient failure the caller may retry.
 */
async function fetchAndReconcileSessionTurns(
  store: ReturnType<typeof useAppStoreApi>,
  sessionId: string,
  hydrationEpoch: number,
): Promise<boolean> {
  try {
    const { turns } = await listSessionTurns(sessionId, { cache: "no-store" });
    // Re-check before merging: the session may have been removed while the
    // request was in flight.
    const state = store.getState();
    if (!state.taskSessions.items[sessionId]) return true;
    // Merge rows, reconcile the active marker, and record the loaded marker in
    // one store transaction. This keeps a live WS update from landing between
    // separate row/marker writes and avoids one Zustand notification per row.
    store.getState().mergeTurnsSnapshot(sessionId, turns, hydrationEpoch);
    return true;
  } catch (err) {
    debug("turn fetch failed", { sessionId, err });
    return false;
  }
}

/**
 * Ensures the store holds this session's FULL persisted turn history.
 *
 * The boot/SSR state hydrates turns only for the page-load active session;
 * switching to another session fetched its messages but never its turns, so
 * every message of that session resolved to `turn = null` and the debug
 * dialog showed `turn_metadata: null` (and turn-derived UI like agent status
 * stayed empty) even though the turns exist server-side with metadata.
 *
 * Completion is recorded in `turns.loadedBySession[sessionId]`, NOT by array
 * presence: WS `session.turn.*` events seed individual live turns without the
 * history, so a non-empty `bySession` list must never suppress the full
 * hydration. The marker is set even when the REST list is empty, so sessions
 * without turns are fetched exactly once; it is cleared only by session
 * removal (the store slice deletes it) or not set at all on failure.
 * Transient failures are retried with bounded backoff inside this call
 * (see fetchAndReconcileSessionTurns); after exhaustion a delayed recovery
 * hydration is scheduled (see scheduleTurnHydrationRecovery), so a
 * prolonged outage resolves on its own once the endpoint recovers — no
 * unrelated lifecycle trigger required. Enrichment only — never delays or
 * fails message loading.
 */
export async function ensureSessionTurnsLoaded(
  sessionId: string,
  store: ReturnType<typeof useAppStoreApi>,
  options?: { readiness?: Promise<void> },
): Promise<void> {
  const readiness = options?.readiness;
  while (true) {
    const state = store.getState();
    if (!state.taskSessions.items[sessionId]) {
      lastHydrationReadiness.delete(sessionId);
      return;
    }
    const loaded = state.turns.loadedBySession[sessionId];
    // A caller without a subscription-generation token keeps the historical
    // one-shot behavior. A readiness promise identifies the current durable
    // WebSocket subscription, so a reconnect gets one fresh REST snapshot.
    if (
      loaded &&
      (readiness === undefined || lastHydrationReadiness.get(sessionId) === readiness)
    ) {
      return;
    }

    const inFlight = inFlightTurnsLoad.get(sessionId);
    if (inFlight) {
      // If this call represents a newer subscription generation, wait for the
      // older request, then loop so it can start a refresh instead of being
      // incorrectly deduplicated by the session-only key.
      await inFlight;
      continue;
    }

    // Capture the active-marker generation: if an authoritative clear (source
    // adoption) bumps it while the request is in flight, the store-owned
    // reconciliation rejects this hydration's marker write.
    const hydrationEpoch = state.turns.reconcileEpochBySession[sessionId] ?? 0;
    const promise = (async () => {
      try {
        for (let attempt = 1; ; attempt++) {
          if (await fetchAndReconcileSessionTurns(store, sessionId, hydrationEpoch)) {
            if (store.getState().taskSessions.items[sessionId]) {
              lastHydrationReadiness.set(sessionId, readiness);
            }
            return;
          }
          debug("turn fetch attempt failed", { sessionId, attempt });
          if (attempt >= TURN_HYDRATION_MAX_ATTEMPTS) {
            scheduleTurnHydrationRecovery(sessionId, store);
            return;
          }
          const delay = Math.min(
            TURN_HYDRATION_RETRY_BASE_MS * 2 ** (attempt - 1),
            TURN_HYDRATION_RETRY_MAX_MS,
          );
          await new Promise((resolve) => window.setTimeout(resolve, delay));
        }
      } finally {
        inFlightTurnsLoad.delete(sessionId);
      }
    })();
    inFlightTurnsLoad.set(sessionId, promise);
    await promise;
    return;
  }
}
