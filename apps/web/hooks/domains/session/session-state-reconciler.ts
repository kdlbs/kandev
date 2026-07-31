import type { StoreApi } from "zustand";
import { fetchTaskSession } from "@/lib/api";
import type { AppState } from "@/lib/state/store";
import { isStaleSessionStateEvent } from "@/lib/ws/handlers/agent-session";

const BUSY_SESSION_STATES = new Set(["STARTING", "RUNNING", "CREATED"]);
const RECONCILE_POLL_INTERVAL_MS = 750;
const RECONCILE_POLL_MAX_MS = 30_000;

type Reconciliation = {
  consumers: number;
  inFlight: boolean;
  startedAt: number;
  timer?: ReturnType<typeof setTimeout>;
};

const reconciliationsByStore = new WeakMap<StoreApi<AppState>, Map<string, Reconciliation>>();

function reconciliationMap(store: StoreApi<AppState>): Map<string, Reconciliation> {
  let reconciliations = reconciliationsByStore.get(store);
  if (!reconciliations) {
    reconciliations = new Map();
    reconciliationsByStore.set(store, reconciliations);
  }
  return reconciliations;
}

function stillOwnsReconciliation(
  reconciliations: Map<string, Reconciliation>,
  sessionId: string,
  reconciliation: Reconciliation,
): boolean {
  return reconciliations.get(sessionId) === reconciliation;
}

/**
 * Starts or joins one bounded state-reconciliation loop per app store/session.
 * Multiple `useSession` consumers share the same HTTP polling owner, matching
 * the WebSocket client's keyed subscription deduplication.
 */
export function acquireSessionStateReconciliation(
  store: StoreApi<AppState>,
  sessionId: string,
): () => void {
  const reconciliations = reconciliationMap(store);
  const existing = reconciliations.get(sessionId);
  if (existing) {
    existing.consumers += 1;
    return () => releaseReconciliation(reconciliations, sessionId, existing);
  }

  const reconciliation: Reconciliation = {
    consumers: 1,
    inFlight: false,
    startedAt: Date.now(),
  };
  reconciliations.set(sessionId, reconciliation);

  const scheduleNext = () => {
    if (!stillOwnsReconciliation(reconciliations, sessionId, reconciliation)) return;
    reconciliation.inFlight = false;
    if (reconciliation.consumers === 0) {
      reconciliations.delete(sessionId);
      return;
    }
    const current = store.getState().taskSessions.items[sessionId];
    if (current && !BUSY_SESSION_STATES.has(current.state)) return;
    if (Date.now() - reconciliation.startedAt >= RECONCILE_POLL_MAX_MS) return;
    reconciliation.timer = setTimeout(reconcile, RECONCILE_POLL_INTERVAL_MS);
  };

  function reconcile() {
    reconciliation.inFlight = true;
    fetchTaskSession(sessionId)
      .then((res) => {
        if (
          !res.session ||
          reconciliation.consumers === 0 ||
          !stillOwnsReconciliation(reconciliations, sessionId, reconciliation)
        ) {
          return;
        }
        const current = store.getState().taskSessions.items[sessionId];
        if (isStaleSessionStateEvent(current, res.session.updated_at)) return;
        store.getState().setTaskSession(res.session);
      })
      .catch(() => {})
      .finally(scheduleNext);
  }

  reconcile();
  return () => releaseReconciliation(reconciliations, sessionId, reconciliation);
}

function releaseReconciliation(
  reconciliations: Map<string, Reconciliation>,
  sessionId: string,
  reconciliation: Reconciliation,
): void {
  if (!stillOwnsReconciliation(reconciliations, sessionId, reconciliation)) return;
  reconciliation.consumers -= 1;
  if (reconciliation.consumers > 0) return;
  if (reconciliation.timer) clearTimeout(reconciliation.timer);
  // Keep an in-flight owner registered until its request settles. React can
  // replace one useSession consumer with another during the same navigation;
  // deleting immediately would start a duplicate authoritative fetch while
  // the first request is still pending.
  if (reconciliation.inFlight) return;
  reconciliations.delete(sessionId);
}
