import type { StoreApi } from "zustand";
import { fetchTaskSessionConditional } from "@/lib/api";
import { buildSessionModelsState } from "@/lib/state/slices/session-runtime/model-hydration";
import type { AppState } from "@/lib/state/store";
import { captureTaskSessionHydrationEpoch } from "@/lib/state/slices/session/hydration-epochs";
import { isStaleSessionStateEvent } from "@/lib/ws/handlers/agent-session";

const BUSY_SESSION_STATES = new Set(["STARTING", "RUNNING", "CREATED"]);
const RECONCILE_POLL_INTERVAL_MS = 750;
const RECONCILE_POLL_MAX_MS = 30_000;

type Reconciliation = {
  consumers: number;
  inFlight: boolean;
  startedAt: number;
  etag: string | null;
  requestGeneration: number;
  timer?: ReturnType<typeof setTimeout>;
};

type ReconciliationRequest = {
  generation: number;
  hydrationEpochAtRequestStart: ReturnType<typeof captureTaskSessionHydrationEpoch>;
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
    if (existing.consumers === 0) existing.startedAt = Date.now();
    existing.consumers += 1;
    return () => releaseReconciliation(reconciliations, sessionId, existing);
  }

  const reconciliation: Reconciliation = {
    consumers: 1,
    inFlight: false,
    startedAt: Date.now(),
    etag: null,
    requestGeneration: 0,
  };
  reconciliations.set(sessionId, reconciliation);

  const scheduleNext = (requestGeneration: number) => {
    if (!stillOwnsReconciliation(reconciliations, sessionId, reconciliation)) return;
    reconciliation.inFlight = false;
    if (reconciliation.consumers === 0) {
      reconciliations.delete(sessionId);
      return;
    }
    if (requestGeneration !== reconciliation.requestGeneration) {
      reconciliation.startedAt = Date.now();
      reconciliation.etag = null;
      reconcile();
      return;
    }
    const current = store.getState().taskSessions.items[sessionId];
    if (current && !BUSY_SESSION_STATES.has(current.state)) return;
    if (Date.now() - reconciliation.startedAt >= RECONCILE_POLL_MAX_MS) return;
    reconciliation.timer = setTimeout(reconcile, RECONCILE_POLL_INTERVAL_MS);
  };

  function reconcile() {
    reconciliation.inFlight = true;
    const requestGeneration = reconciliation.requestGeneration;
    const hydrationEpochAtRequestStart = captureTaskSessionHydrationEpoch(
      store.getState(),
      sessionId,
    );
    readAndReconcile(store, sessionId, reconciliation, reconciliations, {
      generation: requestGeneration,
      hydrationEpochAtRequestStart,
    })
      .catch(() => {})
      .finally(() => scheduleNext(requestGeneration));
  }

  reconcile();
  return () => releaseReconciliation(reconciliations, sessionId, reconciliation);
}

async function readAndReconcile(
  store: StoreApi<AppState>,
  sessionId: string,
  reconciliation: Reconciliation,
  reconciliations: Map<string, Reconciliation>,
  request: ReconciliationRequest,
): Promise<void> {
  const { generation: requestGeneration, hydrationEpochAtRequestStart } = request;
  const owns = () =>
    reconciliation.consumers > 0 &&
    reconciliation.requestGeneration === requestGeneration &&
    stillOwnsReconciliation(reconciliations, sessionId, reconciliation);
  if (!owns()) return;

  let result = reconciliation.etag
    ? await fetchTaskSessionConditional(sessionId, reconciliation.etag)
    : await fetchTaskSessionConditional(sessionId);
  if (!owns()) return;

  if (result.status === "not-modified") {
    if (store.getState().taskSessions.items[sessionId]) return;

    // A 304 can only reuse a full representation held by this owner. If the
    // store entry disappeared while the request was in flight, clear the
    // validator and immediately recover with an unconditional snapshot.
    reconciliation.etag = null;
    result = await fetchTaskSessionConditional(sessionId);
    if (!owns() || result.status === "not-modified") return;
  }

  const session = result.data.session;
  if (!session) return;

  const current = store.getState().taskSessions.items[sessionId];
  if (isStaleSessionStateEvent(current, session.updated_at)) {
    if (result.etag !== reconciliation.etag) reconciliation.etag = null;
    return;
  }

  if (result.etag && result.etag === reconciliation.etag) return;

  store.getState().setTaskSession(session, hydrationEpochAtRequestStart);
  const modelState = buildSessionModelsState(session);
  if (modelState.sessionModels && !store.getState().sessionModels.bySessionId[sessionId]) {
    store.getState().hydrate(modelState);
  }
  reconciliation.etag = result.etag;
}

function releaseReconciliation(
  reconciliations: Map<string, Reconciliation>,
  sessionId: string,
  reconciliation: Reconciliation,
): void {
  if (!stillOwnsReconciliation(reconciliations, sessionId, reconciliation)) return;
  reconciliation.consumers -= 1;
  if (reconciliation.consumers > 0) return;
  reconciliation.requestGeneration += 1;
  reconciliation.etag = null;
  if (reconciliation.timer) clearTimeout(reconciliation.timer);
  // Keep an in-flight owner registered until its request settles. React can
  // replace one useSession consumer with another during the same navigation;
  // deleting immediately would start a duplicate authoritative fetch while
  // the first request is still pending.
  if (reconciliation.inFlight) return;
  reconciliations.delete(sessionId);
}
