import type { StoreApi } from "zustand";
import { getBackendConfig } from "@/lib/config";
import { subscribeIntegrationAvailability } from "@/lib/integrations/integration-availability-events";
import type { AppState } from "@/lib/state/store";

export type IntegrationHealthProvider = "jira" | "linear" | "azure-devops" | "sentry";

export type IntegrationHealthSnapshot<T> = {
  value: T | null;
  loading: boolean;
  loaded: boolean;
  error: unknown;
  lastUpdatedAt: number | null;
};

type Consumer = { fetch: () => Promise<unknown>; refreshMs: number };
type Entry = {
  snapshot: IntegrationHealthSnapshot<unknown>;
  consumers: Map<() => void, Consumer>;
  requestGeneration: number;
  flight: Promise<void> | null;
  queuedRefresh: Promise<void> | null;
  timer: ReturnType<typeof setInterval> | null;
  timerIntervalMs: number | null;
  lastUsed: number;
};
type ScopeOwner = { identity: string; scope: IntegrationHealthResourceScope };

const EMPTY_SNAPSHOT: IntegrationHealthSnapshot<never> = {
  value: null,
  loading: false,
  loaded: false,
  error: null,
  lastUpdatedAt: null,
};
const MAX_INACTIVE_ENTRIES = 48;
const scopes = new WeakMap<StoreApi<AppState>, ScopeOwner>();
let nextUse = 0;

export function integrationHealthAuthScopeIdentity(state: AppState): string {
  return JSON.stringify([
    getBackendConfig().apiBaseUrl,
    state.auth.mode,
    state.auth.authenticated,
    state.auth.user?.id,
    state.workspaces.activeId,
    state.workspaceContextGeneration,
  ]);
}

export class IntegrationHealthResourceScope {
  private entries = new Map<string, Entry>();
  private active = true;
  private unsubscribeAvailability: (() => void) | null = null;

  key(provider: IntegrationHealthProvider, workspaceId?: string | null): string {
    return JSON.stringify([provider, workspaceId ?? null]);
  }

  getSnapshot<T>(key: string): IntegrationHealthSnapshot<T> {
    if (!this.active) return EMPTY_SNAPSHOT as IntegrationHealthSnapshot<T>;
    return (this.entries.get(key)?.snapshot ?? EMPTY_SNAPSHOT) as IntegrationHealthSnapshot<T>;
  }

  subscribe<T>(
    key: string,
    listener: () => void,
    fetch: () => Promise<T>,
    refreshMs: number,
  ): () => void {
    if (!this.active) return () => {};
    const entry = this.entry(key);
    entry.consumers.set(listener, { fetch, refreshMs: Math.max(1, refreshMs) });
    this.touch(key, entry);
    this.startAvailabilitySubscription();
    this.scheduleTimer(key, entry);
    if (!entry.snapshot.loaded || entry.snapshot.error !== null) {
      void this.start(key, entry, fetch);
    }

    return () => {
      entry.consumers.delete(listener);
      this.scheduleTimer(key, entry);
      if (!this.hasConsumers()) {
        this.unsubscribeAvailability?.();
        this.unsubscribeAvailability = null;
      }
      this.trim();
    };
  }

  retire(): void {
    this.active = false;
    this.unsubscribeAvailability?.();
    this.unsubscribeAvailability = null;
    for (const entry of this.entries.values()) {
      entry.requestGeneration++;
      this.clearTimer(entry);
      for (const listener of entry.consumers.keys()) listener();
      entry.consumers.clear();
    }
    this.entries.clear();
  }

  private entry(key: string): Entry {
    let entry = this.entries.get(key);
    if (!entry) {
      entry = {
        snapshot: EMPTY_SNAPSHOT,
        consumers: new Map(),
        requestGeneration: 0,
        flight: null,
        queuedRefresh: null,
        timer: null,
        timerIntervalMs: null,
        lastUsed: 0,
      };
      this.entries.set(key, entry);
    }
    this.touch(key, entry);
    return entry;
  }

  private touch(key: string, entry: Entry): void {
    entry.lastUsed = ++nextUse;
    this.entries.delete(key);
    this.entries.set(key, entry);
  }

  private update(entry: Entry, patch: Partial<IntegrationHealthSnapshot<unknown>>): void {
    entry.snapshot = { ...entry.snapshot, ...patch };
    for (const listener of entry.consumers.keys()) listener();
  }

  private startAvailabilitySubscription(): void {
    if (this.unsubscribeAvailability) return;
    this.unsubscribeAvailability = subscribeIntegrationAvailability(() => this.invalidateActive());
  }

  private hasConsumers(): boolean {
    return [...this.entries.values()].some((entry) => entry.consumers.size > 0);
  }

  private preferredFetch(entry: Entry): () => Promise<unknown> {
    return entry.consumers.values().next().value?.fetch ?? (() => Promise.resolve(null));
  }

  private invalidateActive(): void {
    for (const [key, entry] of this.entries) {
      if (entry.consumers.size > 0) void this.refresh(key, entry);
    }
  }

  private refresh(key: string, entry: Entry): Promise<void> {
    if (!this.active || entry.consumers.size === 0) return Promise.resolve();
    if (entry.queuedRefresh) return entry.queuedRefresh;
    if (!entry.flight) return this.start(key, entry, this.preferredFetch(entry));

    entry.requestGeneration++;
    this.update(entry, { loading: true, error: null });
    const current = entry.flight;
    const queued: Promise<void> = current
      .catch(() => undefined)
      .then(() => {
        if (entry.queuedRefresh === queued) entry.queuedRefresh = null;
        if (!this.active || entry.consumers.size === 0) return;
        return this.start(key, entry, this.preferredFetch(entry));
      });
    entry.queuedRefresh = queued;
    return queued;
  }

  private start(key: string, entry: Entry, fetch: () => Promise<unknown>): Promise<void> {
    if (!this.active) return Promise.resolve();
    if (entry.queuedRefresh) return entry.queuedRefresh;
    if (entry.flight) return entry.flight;
    const generation = ++entry.requestGeneration;
    this.update(entry, { loading: true, error: null });
    const flight: Promise<void> = Promise.resolve()
      .then(fetch)
      .then((value) => {
        if (!this.active || entry.requestGeneration !== generation) return;
        this.update(entry, {
          value,
          loading: false,
          loaded: true,
          error: null,
          lastUpdatedAt: Date.now(),
        });
      })
      .catch((error: unknown) => {
        if (!this.active || entry.requestGeneration !== generation) return;
        this.update(entry, {
          value: null,
          loading: false,
          loaded: true,
          error,
          lastUpdatedAt: Date.now(),
        });
      })
      .finally(() => {
        if (entry.flight === flight) entry.flight = null;
        this.trim();
      });
    entry.flight = flight;
    return flight;
  }

  private scheduleTimer(key: string, entry: Entry): void {
    const intervals = [...entry.consumers.values()].map((consumer) => consumer.refreshMs);
    if (intervals.length === 0) {
      this.clearTimer(entry);
      return;
    }
    const fastest = Math.min(...intervals);
    if (entry.timer && entry.timerIntervalMs === fastest) return;
    this.clearTimer(entry);
    entry.timerIntervalMs = fastest;
    entry.timer = setInterval(() => void this.refresh(key, entry), fastest);
  }

  private clearTimer(entry: Entry): void {
    if (entry.timer) clearInterval(entry.timer);
    entry.timer = null;
    entry.timerIntervalMs = null;
  }

  private trim(): void {
    if (this.entries.size <= MAX_INACTIVE_ENTRIES) return;
    const inactive = [...this.entries.entries()]
      .filter(([, entry]) => !entry.consumers.size && !entry.flight && !entry.queuedRefresh)
      .sort((left, right) => left[1].lastUsed - right[1].lastUsed);
    while (this.entries.size > MAX_INACTIVE_ENTRIES && inactive.length > 0) {
      const [key] = inactive.shift()!;
      this.entries.delete(key);
    }
  }
}

export function getIntegrationHealthResourceScope(
  store: StoreApi<AppState>,
): IntegrationHealthResourceScope {
  let owner = scopes.get(store);
  const identity = integrationHealthAuthScopeIdentity(store.getState());
  if (!owner) {
    owner = { identity, scope: new IntegrationHealthResourceScope() };
    scopes.set(store, owner);
    const scopeOwner = owner;
    store.subscribe((state) => {
      const nextIdentity = integrationHealthAuthScopeIdentity(state);
      if (scopeOwner.identity === nextIdentity) return;
      scopeOwner.scope.retire();
      scopeOwner.identity = nextIdentity;
      scopeOwner.scope = new IntegrationHealthResourceScope();
    });
  } else if (owner.identity !== identity) {
    owner.scope.retire();
    owner.identity = identity;
    owner.scope = new IntegrationHealthResourceScope();
  }
  return owner.scope;
}
