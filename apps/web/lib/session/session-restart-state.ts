import { createStore } from "zustand/vanilla";

type PendingStore = ReturnType<typeof createPendingStore>;
const pendingByOwner = new WeakMap<object, PendingStore>();

/** Holds immutable pending-set snapshots for reactive and synchronous readers. */
function createPendingStore() {
  return createStore<ReadonlySet<string>>(() => new Set());
}

/** Shares restart guards within one app store without retaining discarded owners. */
export function getSessionRestartStore(owner: object) {
  let store = pendingByOwner.get(owner);
  if (!store) {
    store = createPendingStore();
    pendingByOwner.set(owner, store);
  }
  return store;
}

/** Reads the current guard without subscribing, including from earlier callbacks. */
export function isSessionRestartPending(owner: object, sessionId: string | null | undefined) {
  return !!sessionId && getSessionRestartStore(owner).getState().has(sessionId);
}
