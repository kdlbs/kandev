import type { ClarificationAnswer } from "@/lib/types/http";

/**
 * Browser-memory retention for unsubmitted clarification answers.
 *
 * The application store is the owner. Task providers read and write this cache
 * through exact task/source-session/pending-bundle/form-mode identities, so a
 * task-page unmount keeps the draft while another bundle can never read it.
 */

export const MAX_RETAINED_ANSWER_DRAFTS = 8;

export type RetainedAnswerDraft = {
  /** Uncommitted custom-input text keyed by question ID. */
  customDrafts: Record<string, string>;
  activeIndex: number;
  answers: readonly ClarificationAnswer[];
};

type DraftCache = {
  generation: number;
  drafts: Map<string, RetainedAnswerDraft>;
};

const caches = new WeakMap<object, DraftCache>();

/** Exact cache identity for one task and one bundle/form-mode key. */
export function answerDraftIdentity(taskId: string, bundleKey: string): string {
  return JSON.stringify([taskId, bundleKey]);
}

function sameAnswers(
  left: readonly ClarificationAnswer[],
  right: readonly ClarificationAnswer[],
): boolean {
  if (left.length !== right.length) return false;
  const byQuestion = new Map(left.map((answer) => [answer.question_id, answer]));
  return right.every((answer) => {
    const other = byQuestion.get(answer.question_id);
    if (!other) return false;
    return (
      other.custom_text === answer.custom_text &&
      (other.selected_options ?? []).join("\u0000") ===
        (answer.selected_options ?? []).join("\u0000")
    );
  });
}

function sameDraft(left: RetainedAnswerDraft, right: RetainedAnswerDraft): boolean {
  if (left.activeIndex !== right.activeIndex) return false;
  const leftKeys = Object.keys(left.customDrafts);
  if (leftKeys.length !== Object.keys(right.customDrafts).length) return false;
  if (leftKeys.some((key) => left.customDrafts[key] !== right.customDrafts[key])) return false;
  return sameAnswers(left.answers, right.answers);
}

function cacheFor(owner: object): DraftCache {
  const existing = caches.get(owner);
  if (existing) return existing;
  const created: DraftCache = { generation: 0, drafts: new Map() };
  caches.set(owner, created);
  return created;
}

/** Current cache generation for a store owner. */
export function answerDraftGeneration(owner: object): number {
  return cacheFor(owner).generation;
}

/**
 * Read a retained draft. A successful access refreshes its recency so the
 * eight-entry bound evicts the least recently touched bundle.
 */
export function readAnswerDraft(owner: object, identity: string): RetainedAnswerDraft | undefined {
  const cache = cacheFor(owner);
  const draft = cache.drafts.get(identity);
  if (!draft) return undefined;
  cache.drafts.delete(identity);
  cache.drafts.set(identity, draft);
  return draft;
}

/**
 * Write a draft captured under `generation`. A stale generation (the auth
 * authority changed while the form was open) cannot populate the replacement
 * cache.
 */
export function writeAnswerDraft(
  owner: object,
  identity: string,
  generation: number,
  draft: RetainedAnswerDraft,
): boolean {
  const cache = cacheFor(owner);
  if (cache.generation !== generation) return false;
  const existing = cache.drafts.get(identity);
  // An unchanged render is not an access: keep the entry where it is so the
  // recency bound reflects reads and edits rather than mount re-renders.
  if (existing && sameDraft(existing, draft)) return true;
  cache.drafts.delete(identity);
  cache.drafts.set(identity, draft);
  while (cache.drafts.size > MAX_RETAINED_ANSWER_DRAFTS) {
    cache.drafts.delete(cache.drafts.keys().next().value!);
  }
  return true;
}

/**
 * Retire an accepted or terminal bundle. Captured stale generations cannot
 * remove a newer draft.
 */
export function retireAnswerDraft(owner: object, identity: string, generation: number): boolean {
  const cache = cacheFor(owner);
  if (cache.generation !== generation) return false;
  return cache.drafts.delete(identity);
}

/** Clear every draft and fence captured callbacks behind a new generation. */
export function clearAnswerDrafts(owner: object): void {
  const cache = cacheFor(owner);
  cache.drafts.clear();
  cache.generation += 1;
}

export type AnswerDraftAuth = {
  mode: string;
  authenticated: boolean;
  user: { id: string; role: string; status: string } | null;
};

/** Identity whose change invalidates every retained answer draft. */
export function answerDraftAuthorityKey(auth: AnswerDraftAuth): string {
  return JSON.stringify([
    auth.mode,
    auth.authenticated,
    auth.user?.id ?? null,
    auth.user?.role ?? null,
    auth.user?.status ?? null,
  ]);
}

/** Unauthenticated enabled-auth surfaces cannot retain drafts. */
export function canRetainAnswerDrafts(auth: AnswerDraftAuth): boolean {
  return auth.mode !== "enabled" || auth.authenticated;
}

type AnswerDraftAuthorityStore = {
  getState: () => { auth: AnswerDraftAuth };
  subscribe: (
    listener: (state: { auth: AnswerDraftAuth }, previous: { auth: AnswerDraftAuth }) => void,
  ) => () => void;
};

const observedStores = new WeakSet<object>();

/**
 * Install the one store-lifetime authority observer. It stays active without a
 * mounted task page, so a logout or identity change while offscreen still
 * clears drafts and fences captured callbacks.
 */
export function observeAnswerDraftAuthority(store: AnswerDraftAuthorityStore): void {
  if (observedStores.has(store)) return;
  observedStores.add(store);
  let previousKey = answerDraftAuthorityKey(store.getState().auth);
  store.subscribe((state) => {
    const key = answerDraftAuthorityKey(state.auth);
    if (key === previousKey) return;
    previousKey = key;
    clearAnswerDrafts(store);
  });
}
