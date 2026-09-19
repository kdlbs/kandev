import type { PRCommitInfo } from "@/lib/types/github";
import { getWebSocketClient } from "@/lib/ws/connection";
import { t } from "@/lib/i18n";

export type PRCommitsState = {
  commits: PRCommitInfo[];
  authoritativeCommits: PRCommitInfo[];
  providerHead: string | null;
  providerCommitsComplete: boolean;
  loading: boolean;
  error: string | null;
};

export type PRCommitsRequest = {
  workspaceId: string;
  owner: string;
  repo: string;
  prNumber: number;
  sourceKey: string;
};

type PRCommitsResponse = {
  commits?: PRCommitInfo[];
  head_sha?: string;
  complete?: boolean;
};

type PRCommitsRequester = (request: PRCommitsRequest) => Promise<PRCommitsResponse | null>;

type ResourceOptions = {
  retryDelayMs?: number;
  maxEntries?: number;
};

type RetryWait = {
  timer: ReturnType<typeof setTimeout>;
  resolve: () => void;
};

type ResourceEntry = {
  key: string;
  identity: string;
  request: PRCommitsRequest | null;
  snapshot: PRCommitsState;
  successfulCommits: PRCommitInfo[];
  loaded: boolean;
  promise: Promise<PRCommitsState> | null;
  listeners: Set<() => void>;
  orphaned: boolean;
  retryWait: RetryWait | null;
  lastUsed: number;
};

const DEFAULT_RETRY_DELAY_MS = 100;
const DEFAULT_MAX_ENTRIES = 24;
const EMPTY_STATE: PRCommitsState = {
  commits: [],
  authoritativeCommits: [],
  providerHead: null,
  providerCommitsComplete: false,
  loading: false,
  error: null,
};
const PENDING_STATE: PRCommitsState = {
  ...EMPTY_STATE,
  loading: true,
};

function pendingState(commits: PRCommitInfo[] = []): PRCommitsState {
  return { ...PENDING_STATE, commits };
}

function identityFor(request: PRCommitsRequest): string {
  return [request.workspaceId, request.owner, request.repo, request.prNumber].join("\0");
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : t("github:failedToFetchPrCommits");
}

async function requestPRCommits(request: PRCommitsRequest): Promise<PRCommitsResponse | null> {
  const client = getWebSocketClient();
  if (!client) return null;
  return client.request<PRCommitsResponse>("github.pr_commits.get", {
    workspace_id: request.workspaceId,
    owner: request.owner,
    repo: request.repo,
    number: request.prNumber,
  });
}

function successState(response: PRCommitsResponse | null): PRCommitsState {
  const commits = response?.commits ?? [];
  return {
    commits,
    authoritativeCommits: commits,
    providerHead: response?.head_sha ?? null,
    providerCommitsComplete: response?.complete === true,
    loading: false,
    error: null,
  };
}

function failedState(error: unknown, commits: PRCommitInfo[] = []): PRCommitsState {
  return {
    ...EMPTY_STATE,
    commits,
    error: errorMessage(error),
  };
}

function createEntry(
  request: PRCommitsRequest | null,
  key: string,
  successfulCommits: PRCommitInfo[] = [],
): ResourceEntry {
  return {
    key,
    identity: request ? identityFor(request) : key,
    request,
    snapshot: pendingState(successfulCommits),
    successfulCommits,
    loaded: false,
    promise: null,
    listeners: new Set(),
    orphaned: false,
    retryWait: null,
    lastUsed: 0,
  };
}

/**
 * Creates a small external resource for provider commit history. The factory
 * keeps the request policy testable while the hook below owns one app-wide
 * instance.
 */
type PRCommitsResourceStore = {
  entries: Map<string, ResourceEntry>;
  currentKeyByIdentity: Map<string, string>;
  retryDelayMs: number;
  maxEntries: number;
  usageCounter: number;
};

function touch(store: PRCommitsResourceStore, entry: ResourceEntry): void {
  entry.lastUsed = ++store.usageCounter;
}

function deleteEntry(store: PRCommitsResourceStore, entry: ResourceEntry): void {
  store.entries.delete(entry.key);
  if (store.currentKeyByIdentity.get(entry.identity) === entry.key) {
    store.currentKeyByIdentity.delete(entry.identity);
  }
}

function entryFor(
  store: PRCommitsResourceStore,
  key: string,
  request: PRCommitsRequest | null = null,
): ResourceEntry {
  const existing = store.entries.get(key);
  if (existing) {
    if (request) {
      existing.request = request;
      existing.identity = identityFor(request);
      store.currentKeyByIdentity.set(existing.identity, key);
    }
    touch(store, existing);
    return existing;
  }
  const identity = request ? identityFor(request) : key;
  const retainedKey = store.currentKeyByIdentity.get(identity);
  const retainedEntry = retainedKey ? store.entries.get(retainedKey) : undefined;
  const entry = createEntry(request, key, retainedEntry?.successfulCommits ?? []);
  store.entries.set(key, entry);
  if (request) store.currentKeyByIdentity.set(identity, key);
  touch(store, entry);
  return entry;
}

function prune(store: PRCommitsResourceStore, entry: ResourceEntry): void {
  if (store.currentKeyByIdentity.get(entry.identity) === entry.key) {
    for (const candidate of store.entries.values()) {
      if (
        candidate !== entry &&
        candidate.identity === entry.identity &&
        !candidate.promise &&
        candidate.listeners.size === 0
      ) {
        deleteEntry(store, candidate);
      }
    }
  }

  const candidates = [...store.entries.values()]
    .filter(
      (candidate) => candidate !== entry && !candidate.promise && candidate.listeners.size === 0,
    )
    .sort((left, right) => left.lastUsed - right.lastUsed);
  for (const candidate of candidates) {
    if (store.entries.size <= store.maxEntries) break;
    deleteEntry(store, candidate);
  }
}

function clearRetryWait(entry: ResourceEntry): void {
  if (!entry.retryWait) return;
  clearTimeout(entry.retryWait.timer);
  entry.retryWait.resolve();
  entry.retryWait = null;
}

function waitBeforeRetry(store: PRCommitsResourceStore, entry: ResourceEntry): Promise<void> {
  return new Promise((resolve) => {
    const timer = setTimeout(() => {
      entry.retryWait = null;
      resolve();
    }, store.retryDelayMs);
    entry.retryWait = { timer, resolve };
  });
}

function notify(entry: ResourceEntry): void {
  for (const listener of entry.listeners) listener();
}

function isCurrent(store: PRCommitsResourceStore, entry: ResourceEntry): boolean {
  return store.entries.get(entry.key) === entry;
}

function publish(
  store: PRCommitsResourceStore,
  entry: ResourceEntry,
  snapshot: PRCommitsState,
  loaded: boolean,
): void {
  if (!isCurrent(store, entry)) return;
  entry.snapshot = snapshot;
  if (loaded && !snapshot.error) entry.successfulCommits = snapshot.authoritativeCommits;
  entry.loaded = loaded;
  touch(store, entry);
  notify(entry);
}

function publishFailure(
  store: PRCommitsResourceStore,
  entry: ResourceEntry,
  error: unknown,
): PRCommitsState {
  const snapshot = failedState(error, entry.successfulCommits);
  publish(store, entry, snapshot, false);
  if (
    isCurrent(store, entry) &&
    entry.listeners.size === 0 &&
    (store.currentKeyByIdentity.get(entry.identity) !== entry.key ||
      entry.successfulCommits.length === 0)
  ) {
    deleteEntry(store, entry);
  }
  return snapshot;
}

async function attempt(
  requester: PRCommitsRequester,
  request: PRCommitsRequest,
): Promise<
  { response: PRCommitsResponse | null; error: null } | { response: null; error: unknown }
> {
  try {
    const response = await requester(request);
    if (response === null) {
      return { response: null, error: new Error(t("github:failedToFetchPrCommits")) };
    }
    return { response, error: null };
  } catch (error) {
    return { response: null, error };
  }
}

async function run(
  store: PRCommitsResourceStore,
  requester: PRCommitsRequester,
  entry: ResourceEntry,
  request: PRCommitsRequest,
): Promise<PRCommitsState> {
  const first = await attempt(requester, request);
  if (!first.error) {
    const snapshot = successState(first.response);
    publish(store, entry, snapshot, true);
    prune(store, entry);
    return snapshot;
  }

  if (isCurrent(store, entry) && !entry.orphaned) await waitBeforeRetry(store, entry);
  if (!isCurrent(store, entry) || entry.orphaned) {
    return failedState(first.error, entry.successfulCommits);
  }

  const second = await attempt(requester, request);
  if (!second.error) {
    const snapshot = successState(second.response);
    publish(store, entry, snapshot, true);
    prune(store, entry);
    return snapshot;
  }
  return publishFailure(store, entry, second.error);
}

function load(
  store: PRCommitsResourceStore,
  requester: PRCommitsRequester,
  request: PRCommitsRequest,
  force = false,
): Promise<PRCommitsState> {
  const entry = entryFor(store, request.sourceKey, request);
  entry.orphaned = false;
  prune(store, entry);
  if (entry.promise) return entry.promise;
  if (!force && entry.loaded && !entry.snapshot.error) return Promise.resolve(entry.snapshot);

  entry.loaded = false;
  entry.snapshot = pendingState(entry.successfulCommits);
  notify(entry);
  const promise = run(store, requester, entry, request);
  entry.promise = promise;
  void promise.then(
    () => {
      if (entry.promise === promise) entry.promise = null;
    },
    () => {
      if (entry.promise === promise) entry.promise = null;
    },
  );
  return promise;
}

function subscribe(
  store: PRCommitsResourceStore,
  request: PRCommitsRequest | null,
  listener: () => void,
): () => void {
  if (!request) return () => undefined;
  const entry = entryFor(store, request.sourceKey, request);
  entry.listeners.add(listener);
  entry.orphaned = false;
  touch(store, entry);
  return () => {
    entry.listeners.delete(listener);
    if (entry.listeners.size > 0) return;
    entry.orphaned = Boolean(entry.promise);
    clearRetryWait(entry);
    if (
      !entry.promise &&
      entry.snapshot.error &&
      (store.currentKeyByIdentity.get(entry.identity) !== entry.key ||
        entry.successfulCommits.length === 0)
    ) {
      deleteEntry(store, entry);
    }
  };
}

function getSnapshot(
  store: PRCommitsResourceStore,
  request: PRCommitsRequest | null,
): PRCommitsState {
  if (!request) return EMPTY_STATE;
  const entry = entryFor(store, request.sourceKey, request);
  // useSyncExternalStore reads during render; touching here keeps actively
  // rendered entries recent for bounded LRU eviction.
  touch(store, entry);
  return entry.snapshot;
}

export function createPRCommitsResource(
  requester: PRCommitsRequester = requestPRCommits,
  options: ResourceOptions = {},
) {
  const store: PRCommitsResourceStore = {
    entries: new Map(),
    currentKeyByIdentity: new Map(),
    retryDelayMs: Math.max(0, options.retryDelayMs ?? DEFAULT_RETRY_DELAY_MS),
    maxEntries: Math.max(1, options.maxEntries ?? DEFAULT_MAX_ENTRIES),
    usageCounter: 0,
  };
  return {
    getSnapshot: (request: PRCommitsRequest | null) => getSnapshot(store, request),
    load: (request: PRCommitsRequest, force = false) => load(store, requester, request, force),
    subscribe: (request: PRCommitsRequest | null, listener: () => void) =>
      subscribe(store, request, listener),
  };
}

export const prCommitsResource = createPRCommitsResource();
