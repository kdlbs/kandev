import { useCallback, useMemo, useSyncExternalStore } from "react";
import type { StoreApi } from "zustand";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { getBackendConfig } from "@/lib/config";
import type { AppState } from "@/lib/state/store";
import type { PRFeedback } from "@/lib/types/github";

export type PRFeedbackIdentity = {
  workspaceId: string;
  owner: string;
  repo: string;
  prNumber: number;
};

export type PRFeedbackSnapshot = {
  feedback: PRFeedback | null;
  loading: boolean;
  error: unknown;
  lastUpdatedAt: number | null;
};

type Entry = {
  snapshot: PRFeedbackSnapshot;
  listeners: Set<() => void>;
  requestGeneration: number;
  flight: Promise<void> | null;
  queuedRefresh: Promise<void> | null;
  hasCurrentRead: boolean;
  refreshToken: string | null;
  lastUsed: number;
};

type ScopeOwner = { identity: string; scope: PRFeedbackResourceScope };

const EMPTY_SNAPSHOT: PRFeedbackSnapshot = {
  feedback: null,
  loading: false,
  error: null,
  lastUpdatedAt: null,
};
const MAX_INACTIVE_READS = 32;
const scopes = new WeakMap<StoreApi<AppState>, ScopeOwner>();
let nextUse = 0;

function authScopeIdentity(state: AppState): string {
  return JSON.stringify([
    getBackendConfig().apiBaseUrl,
    state.auth.mode,
    state.auth.authenticated,
    state.auth.user?.id,
    state.workspaces.activeId,
    state.workspaceContextGeneration,
  ]);
}

export class PRFeedbackResourceScope {
  private entries = new Map<string, Entry>();
  private active = true;

  constructor(
    private readonly store: StoreApi<AppState>,
    private readonly identity: string,
  ) {}

  key(pr: PRFeedbackIdentity): string {
    return JSON.stringify([
      this.identity,
      "github",
      pr.workspaceId,
      pr.owner,
      pr.repo,
      pr.prNumber,
    ]);
  }

  getSnapshot = (key: string): PRFeedbackSnapshot => {
    if (!this.active) return EMPTY_SNAPSHOT;
    return this.entry(key).snapshot;
  };

  subscribe = (key: string, listener: () => void) => {
    if (!this.active) return () => {};
    const entry = this.entry(key);
    entry.listeners.add(listener);
    this.touch(key, entry);
    return () => {
      entry.listeners.delete(listener);
      if (entry.listeners.size === 0) {
        entry.hasCurrentRead = false;
        entry.refreshToken = null;
      }
      this.trim();
    };
  };

  ensure(key: string, fetch: () => Promise<PRFeedback>, refreshToken?: string): Promise<void> {
    const entry = this.entry(key);
    if (!this.active) return Promise.resolve();
    if (refreshToken !== undefined) {
      if (entry.refreshToken === refreshToken) {
        return (
          entry.queuedRefresh ??
          entry.flight ??
          (entry.hasCurrentRead ? Promise.resolve() : this.start(key, entry, fetch))
        );
      }
      const isFirstToken = entry.refreshToken === null;
      entry.refreshToken = refreshToken;
      if (isFirstToken) {
        if (entry.flight || entry.hasCurrentRead) {
          return entry.queuedRefresh ?? entry.flight ?? Promise.resolve();
        }
        return this.start(key, entry, fetch);
      }
      return this.invalidate(key, fetch);
    }
    if (entry.flight) return entry.flight;
    if (entry.hasCurrentRead) return Promise.resolve();
    return this.start(key, entry, fetch);
  }

  ensureFresh(key: string, fetch: () => Promise<PRFeedback>, refreshToken?: string): Promise<void> {
    const entry = this.entry(key);
    if (!this.active) return Promise.resolve();
    if (refreshToken !== undefined) entry.refreshToken = refreshToken;
    if (entry.queuedRefresh) return entry.queuedRefresh;
    if (entry.flight) return entry.flight;
    return this.start(key, entry, fetch);
  }

  invalidate(key: string, fetch: () => Promise<PRFeedback>): Promise<void> {
    const entry = this.entry(key);
    if (!this.active) return Promise.resolve();
    if (!entry.flight) return this.start(key, entry, fetch);
    if (entry.queuedRefresh) return entry.queuedRefresh;

    entry.requestGeneration++;
    this.update(entry, { loading: true, error: null });
    const current = entry.flight;
    const queued = current
      .catch(() => undefined)
      .then(() => {
        if (entry.queuedRefresh === queued) entry.queuedRefresh = null;
        if (!this.active) return;
        return this.start(key, entry, fetch);
      });
    entry.queuedRefresh = queued;
    return queued;
  }

  retire() {
    this.active = false;
    for (const entry of this.entries.values()) {
      entry.requestGeneration++;
      entry.snapshot = EMPTY_SNAPSHOT;
      for (const listener of entry.listeners) listener();
    }
    this.entries.clear();
  }

  private entry(key: string): Entry {
    let entry = this.entries.get(key);
    if (!entry) {
      const cached = this.store.getState().prFeedbackCache.byKey[key];
      entry = {
        snapshot: cached
          ? {
              feedback: cached.feedback,
              loading: false,
              error: null,
              lastUpdatedAt: cached.lastUpdatedAt,
            }
          : EMPTY_SNAPSHOT,
        listeners: new Set(),
        requestGeneration: 0,
        flight: null,
        queuedRefresh: null,
        hasCurrentRead: false,
        refreshToken: null,
        lastUsed: 0,
      };
      this.entries.set(key, entry);
    }
    this.touch(key, entry);
    return entry;
  }

  private touch(key: string, entry: Entry) {
    entry.lastUsed = ++nextUse;
    this.entries.delete(key);
    this.entries.set(key, entry);
  }

  private update(entry: Entry, patch: Partial<PRFeedbackSnapshot>) {
    entry.snapshot = { ...entry.snapshot, ...patch };
    for (const listener of entry.listeners) listener();
  }

  private start(key: string, entry: Entry, fetch: () => Promise<PRFeedback>) {
    if (!this.active) return Promise.resolve();
    const generation = ++entry.requestGeneration;
    this.update(entry, { loading: true, error: null });
    const flight: Promise<void> = Promise.resolve()
      .then(fetch)
      .then((feedback) => {
        if (!this.active || entry.requestGeneration !== generation) return;
        entry.hasCurrentRead = entry.listeners.size > 0;
        if (feedback) {
          this.store.getState().setPRFeedbackCacheEntry(key, feedback);
          const cached = this.store.getState().prFeedbackCache.byKey[key];
          this.update(entry, {
            feedback,
            lastUpdatedAt: cached?.lastUpdatedAt ?? Date.now(),
          });
        }
      })
      .catch((error: unknown) => {
        if (this.active && entry.requestGeneration === generation) {
          entry.hasCurrentRead = entry.snapshot.feedback !== null && entry.listeners.size > 0;
          this.update(entry, { error });
        }
      })
      .finally(() => {
        if (entry.flight === flight) entry.flight = null;
        if (this.active && entry.requestGeneration === generation && !entry.queuedRefresh) {
          this.update(entry, { loading: false });
        }
        this.trim();
      });
    entry.flight = flight;
    return flight;
  }

  private trim() {
    if (this.entries.size <= MAX_INACTIVE_READS) return;
    const inactive = [...this.entries.entries()]
      .filter(([, entry]) => entry.listeners.size === 0 && !entry.flight && !entry.queuedRefresh)
      .sort((left, right) => left[1].lastUsed - right[1].lastUsed);
    while (this.entries.size > MAX_INACTIVE_READS && inactive.length > 0) {
      const [key] = inactive.shift()!;
      this.entries.delete(key);
    }
  }
}

export function getPRFeedbackResourceScope(store: StoreApi<AppState>): PRFeedbackResourceScope {
  let owner = scopes.get(store);
  const identity = authScopeIdentity(store.getState());
  if (!owner) {
    owner = { identity, scope: new PRFeedbackResourceScope(store, identity) };
    scopes.set(store, owner);
    const scopeOwner = owner;
    store.subscribe((state) => {
      const nextIdentity = authScopeIdentity(state);
      if (scopeOwner.identity === nextIdentity) return;
      scopeOwner.scope.retire();
      scopeOwner.identity = nextIdentity;
      scopeOwner.scope = new PRFeedbackResourceScope(store, nextIdentity);
    });
  } else if (owner.identity !== identity) {
    owner.scope.retire();
    owner.identity = identity;
    owner.scope = new PRFeedbackResourceScope(store, identity);
  }
  return owner.scope;
}

export function usePRFeedbackResourceScope() {
  const store = useAppStoreApi();
  const identity = useAppStore(authScopeIdentity);
  return useMemo(() => getPRFeedbackResourceScope(store), [identity, store]);
}

export function usePRFeedbackResourceSnapshot(scope: PRFeedbackResourceScope, key: string | null) {
  const subscribe = useCallback(
    (listener: () => void) => (key ? scope.subscribe(key, listener) : () => {}),
    [key, scope],
  );
  const getSnapshot = useCallback(
    () => (key ? scope.getSnapshot(key) : EMPTY_SNAPSHOT),
    [key, scope],
  );
  return useSyncExternalStore(subscribe, getSnapshot, getSnapshot);
}
