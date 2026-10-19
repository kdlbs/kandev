import { createStore } from "zustand/vanilla";

type PendingStore = ReturnType<typeof createPendingStore>;
const pendingByOwner = new WeakMap<object, PendingStore>();

function createPendingStore() {
  return createStore<ReadonlySet<string>>(() => new Set());
}

export function getSessionRestartStore(owner: object) {
  let store = pendingByOwner.get(owner);
  if (!store) {
    store = createPendingStore();
    pendingByOwner.set(owner, store);
  }
  return store;
}

export function isSessionRestartPending(owner: object, sessionId: string | null | undefined) {
  return !!sessionId && getSessionRestartStore(owner).getState().has(sessionId);
}
