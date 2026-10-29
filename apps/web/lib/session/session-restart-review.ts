import { createStore } from "zustand/vanilla";

export type SessionRestartReview = {
  taskId: string;
  sessionId: string;
  attemptId: string;
  identity: string;
  navigationRevision?: number;
};
export type SessionRestartDecision =
  | { mode: "captured" }
  | { mode: "current"; selectionDigest: string };

type ReviewOwner = {
  store: ReturnType<typeof createReviewStore>;
  resolve: ((decision: SessionRestartDecision | null) => void) | null;
};
const reviews = new WeakMap<object, ReviewOwner>();

function createReviewStore() {
  return createStore<SessionRestartReview | null>(() => null);
}

function reviewOwner(owner: object) {
  let entry = reviews.get(owner);
  if (!entry) {
    entry = { store: createReviewStore(), resolve: null };
    reviews.set(owner, entry);
  }
  return entry;
}

/** Keeps one transient review alive outside menus and session tab consumers. */
export function getSessionRestartReviewStore(owner: object) {
  return reviewOwner(owner).store;
}

export function requestSessionRestartReview(
  owner: object,
  request: SessionRestartReview,
): Promise<SessionRestartDecision | null> {
  const entry = reviewOwner(owner);
  if (entry.store.getState()) return Promise.resolve(null);
  return new Promise((resolve) => {
    entry.resolve = resolve;
    entry.store.setState({ ...request }, true);
  });
}

/** Reference identity prevents a late dialog from confirming a newer review. */
export function settleSessionRestartReview(
  owner: object,
  request: SessionRestartReview,
  decision: SessionRestartDecision | null,
) {
  const entry = reviewOwner(owner);
  if (entry.store.getState() !== request) return;
  const resolve = entry.resolve;
  entry.resolve = null;
  entry.store.setState(null, true);
  resolve?.(decision);
}
