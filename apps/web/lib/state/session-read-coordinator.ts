import type { StoreApi } from "zustand";
import type { AppState } from "./store";
import { getBackendConfig } from "@/lib/config";
import { getWebSocketClient, subscribeWebSocketClient } from "@/lib/ws/connection";

type Resource = "shells" | "commits" | "diff";
export type SessionReadSnapshot<T> = { data: T | undefined; loading: boolean; error: unknown };
export type SessionReadResult<T> = { data?: T; retryAfter?: number };
export type SessionReadOptions<T> = {
  resource: Resource;
  key: string;
  environmentId: string;
  fetch: (isCurrent: () => boolean) => Promise<SessionReadResult<T>>;
  isCurrent: () => boolean;
  onLoading?: (loading: boolean) => void;
};

const INACTIVE_READ_LIMIT = 32;

export class SessionRead<T> {
  private snapshot: SessionReadSnapshot<T> = { data: undefined, loading: false, error: null };
  private listeners = new Set<() => void>();
  private flight: { promise?: Promise<void> } | null = null;
  private timer: ReturnType<typeof setTimeout> | null = null;
  private revision = 0;
  private observedRevision: number | undefined;
  private initialized = false;

  constructor(
    private scope: SessionReadScope,
    public options: SessionReadOptions<T>,
  ) {}

  getSnapshot = () => this.snapshot;
  get inactive() {
    return this.listeners.size === 0 && !this.flight;
  }
  get group() {
    const { resource, key, environmentId } = this.options;
    return JSON.stringify([resource, resource === "diff" ? key : environmentId]);
  }
  private current = () => this.scope.current() && this.options.isCurrent();
  private writable = () => this.current() && this.scope.owns(this);

  subscribe = (listener: () => void) => {
    this.listeners.add(listener);
    return () => {
      this.listeners.delete(listener);
      if (this.listeners.size === 0) {
        this.clearTimer();
        if (!this.flight) this.update({ loading: false });
        this.scope.trim();
      }
    };
  };

  private update(patch: Partial<SessionReadSnapshot<T>>) {
    this.snapshot = { ...this.snapshot, ...patch };
    if (patch.loading !== undefined && this.writable()) this.options.onLoading?.(patch.loading);
    for (const listener of this.listeners) listener();
  }

  private clearTimer() {
    if (this.timer !== null) clearTimeout(this.timer);
    this.timer = null;
  }

  retire() {
    this.clearTimer();
    this.revision++;
    this.initialized = false;
  }

  ensure = (revision?: number, reset = false) => {
    if (!this.current()) return;
    if (revision !== undefined && revision !== this.observedRevision) {
      if (this.observedRevision !== undefined) this.invalidate();
      this.observedRevision = revision;
    }
    if (reset) this.initialized = false;
    if (!this.scope.owns(this)) {
      this.scope.claim(this);
      this.initialized = false;
    }
    if (!this.initialized && !this.flight && this.timer === null) this.start();
  };

  invalidate = (delay = 0) => {
    this.initialized = false;
    this.revision++;
    this.clearTimer();
    if (this.flight || this.listeners.size === 0 || !this.current()) return;
    if (delay > 0) {
      this.timer = setTimeout(() => {
        this.timer = null;
        this.ensure();
      }, delay);
    } else {
      this.ensure();
    }
  };

  refetch = async () => {
    this.invalidate();
    await this.flight?.promise;
  };

  private start() {
    const flight: { promise?: Promise<void> } = {};
    const revision = this.revision;
    this.flight = flight;
    const isCurrent = () => this.writable() && this.flight === flight && this.revision === revision;
    this.update({ loading: true, error: null });
    flight.promise = this.options
      .fetch(isCurrent)
      .then((result) => {
        if (!isCurrent()) return;
        if (result.retryAfter !== undefined) {
          if (this.listeners.size > 0) {
            this.timer = setTimeout(() => {
              this.timer = null;
              this.ensure();
            }, result.retryAfter);
          }
          return;
        }
        this.initialized = true;
        if ("data" in result) this.update({ data: result.data });
      })
      .catch((error: unknown) => {
        if (isCurrent()) this.update({ error });
      })
      .finally(() => {
        if (this.flight !== flight) return;
        this.flight = null;
        if (!this.current()) {
          this.update({ loading: false });
          this.scope.trim();
          return;
        }
        if (this.revision !== revision && this.listeners.size > 0) {
          this.clearTimer();
          this.ensure();
        } else if (this.timer === null) {
          this.update({ loading: false });
        }
        this.scope.trim();
      });
  }
}

export class SessionReadScope {
  private reads = new Map<Resource, Map<string, SessionRead<unknown>>>();
  private writers = new Map<string, unknown>();
  private live = true;
  readonly client = getWebSocketClient();

  constructor(private store: StoreApi<AppState>) {}

  current = () =>
    this.live &&
    this.client !== null &&
    this.client === getWebSocketClient() &&
    this.store.getState().connection.status === "connected";

  owns(read: { group: string }) {
    return this.writers.get(read.group) === read;
  }

  claim(read: { group: string }) {
    this.writers.set(read.group, read);
  }

  get<T>(options: SessionReadOptions<T>): SessionRead<T> {
    const reads = this.reads.get(options.resource) ?? new Map();
    this.reads.set(options.resource, reads);
    let read = reads.get(options.key) as SessionRead<T> | undefined;
    if (!read) read = new SessionRead<T>(this, options);
    read.options = options;
    reads.delete(options.key);
    reads.set(options.key, read as SessionRead<unknown>);
    return read;
  }

  invalidate(resource: Resource, environmentId: string, delay = 0) {
    for (const read of this.reads.get(resource)?.values() ?? []) {
      if (read.options.environmentId === environmentId) read.invalidate(delay);
    }
  }

  trim() {
    for (const reads of this.reads.values()) {
      let inactive = [...reads.values()].filter((read) => read.inactive).length;
      for (const [key, read] of reads) {
        if (inactive <= INACTIVE_READ_LIMIT) break;
        if (!read.inactive) continue;
        read.retire();
        reads.delete(key);
        if (this.owns(read)) this.writers.delete(read.group);
        inactive--;
      }
    }
  }

  retire() {
    this.live = false;
    for (const reads of this.reads.values()) {
      for (const read of reads.values()) read.retire();
    }
    this.reads.clear();
    this.writers.clear();
  }
}

function identity(state: AppState) {
  return JSON.stringify([
    getBackendConfig().apiBaseUrl,
    state.auth.mode,
    state.auth.authenticated,
    state.auth.user?.id,
    state.workspaces.activeId,
    state.workspaceContextGeneration,
    state.connection.status,
  ]);
}

type ScopeOwner = {
  scope: SessionReadScope;
  identity: string;
  listeners: Set<() => void>;
};
const owners = new WeakMap<StoreApi<AppState>, ScopeOwner>();

export function getSessionReadScope(store: StoreApi<AppState>): SessionReadScope {
  let owner = owners.get(store);
  const nextIdentity = identity(store.getState());
  if (!owner) {
    owner = { scope: new SessionReadScope(store), identity: nextIdentity, listeners: new Set() };
    owners.set(store, owner);
    store.subscribe(() => refreshScope(store));
  } else if (owner.identity !== nextIdentity || owner.scope.client !== getWebSocketClient()) {
    owner.scope.retire();
    owner.scope = new SessionReadScope(store);
    owner.identity = nextIdentity;
  }
  return owner.scope;
}

function refreshScope(store: StoreApi<AppState>) {
  const owner = owners.get(store);
  const previous = owner?.scope;
  if (getSessionReadScope(store) !== previous) {
    for (const listener of owner?.listeners ?? []) listener();
  }
}

export function subscribeSessionReadScope(store: StoreApi<AppState>, listener: () => void) {
  getSessionReadScope(store);
  const owner = owners.get(store)!;
  owner.listeners.add(listener);
  const unsubscribeClient = subscribeWebSocketClient(() => refreshScope(store));
  return () => {
    owner.listeners.delete(listener);
    unsubscribeClient();
  };
}

export function environmentReadIdentity(state: AppState, environmentId: string) {
  const attempt = state.workspaceRestoration.byEnvironmentId[environmentId];
  return JSON.stringify([attempt?.attemptId, attempt?.revision, attempt?.status]);
}
