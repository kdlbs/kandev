import type { StoreApi } from "zustand";
import { getWebSocketClient } from "@/lib/ws/connection";
import type { AppState } from "@/lib/state/store";
import { isCurrentWorkspaceContext } from "@/lib/state/workspace-context";
import type { TaskPRScope } from "@/lib/state/slices/github/types";
import type { TaskPR } from "@/lib/types/github";

export type TaskPRSyncScope = TaskPRScope & {
  taskId: string;
};

export type TaskPRSyncResponse =
  | { prs?: TaskPR[]; permanent?: boolean }
  | TaskPR
  | null
  | undefined;

export type TaskPRSyncRequester = (scope: TaskPRSyncScope) => Promise<TaskPRSyncResponse>;

export type TaskPRSyncResourceOptions = {
  retryDelayMs?: number;
  maxRetries?: number;
};

export type TaskPRSyncResource = {
  getSnapshot: (scope: TaskPRSyncScope) => boolean;
  invalidate: (scope: TaskPRSyncScope) => void;
  refresh: (scope: TaskPRSyncScope) => Promise<void>;
  subscribe: (scope: TaskPRSyncScope, listener: () => void) => () => void;
};

const DEFAULT_RETRY_DELAY_MS = 5_000;
const DEFAULT_MAX_RETRIES = 6;
const NO_WEBSOCKET_CLIENT = Symbol();

type ResourceEntry = {
  key: string;
  scope: TaskPRSyncScope;
  loaded: boolean;
  listeners: Set<() => void>;
  promise: Promise<void> | null;
  requestStartedAt: number | null;
  requestGeneration: number;
  retryCount: number;
  permanent: boolean;
  retryTimer: ReturnType<typeof setTimeout> | null;
  stopStoreSubscription: (() => void) | null;
  connectionStatus: AppState["connection"]["status"];
};

type ResourceStore = {
  appStore: StoreApi<AppState>;
  entries: Map<string, ResourceEntry>;
  requester: TaskPRSyncRequester;
  retryDelayMs: number;
  maxRetries: number;
};

async function requestTaskPRSync(scope: TaskPRSyncScope): Promise<TaskPRSyncResponse> {
  const client = getWebSocketClient();
  if (!client) throw NO_WEBSOCKET_CLIENT;
  return client.request<TaskPRSyncResponse>("github.task_pr.sync", { task_id: scope.taskId });
}

function scopeKey(scope: TaskPRSyncScope): string {
  // The NUL delimiter is an in-memory scope boundary; API IDs are opaque
  // stable values and are not expected to contain it.
  return `${scope.workspaceId ?? ""}\u0000${scope.workspaceContextGeneration}\u0000${scope.taskId}`;
}

function createEntry(scope: TaskPRSyncScope): ResourceEntry {
  return {
    key: scopeKey(scope),
    scope,
    loaded: false,
    listeners: new Set(),
    promise: null,
    requestStartedAt: null,
    requestGeneration: 0,
    retryCount: 0,
    permanent: false,
    retryTimer: null,
    stopStoreSubscription: null,
    connectionStatus: "disconnected",
  };
}

function entryFor(store: ResourceStore, scope: TaskPRSyncScope): ResourceEntry {
  const key = scopeKey(scope);
  const existing = store.entries.get(key);
  if (existing) return existing;
  const entry = createEntry(scope);
  store.entries.set(key, entry);
  return entry;
}

function isCurrentEntry(store: ResourceStore, entry: ResourceEntry): boolean {
  return store.entries.get(entry.key) === entry;
}

function isCurrentContext(store: ResourceStore, entry: ResourceEntry): boolean {
  return isCurrentWorkspaceContext(
    storeState(store),
    entry.scope.workspaceId,
    entry.scope.workspaceContextGeneration,
  );
}

function storeState(store: ResourceStore): AppState {
  return store.appStore.getState();
}

function isActiveEntry(store: ResourceStore, entry: ResourceEntry): boolean {
  return entry.listeners.size > 0 && isCurrentEntry(store, entry) && isCurrentContext(store, entry);
}

function hasTaskPRs(state: AppState, scope: TaskPRSyncScope): boolean {
  return (
    state.taskPRs.workspaceId === scope.workspaceId &&
    state.taskPRs.workspaceContextGeneration === scope.workspaceContextGeneration &&
    Array.isArray(state.taskPRs.byTaskId[scope.taskId]) &&
    state.taskPRs.byTaskId[scope.taskId].length > 0
  );
}

function notify(entry: ResourceEntry): void {
  for (const listener of entry.listeners) listener();
}

function setLoaded(entry: ResourceEntry, loaded: boolean): void {
  if (entry.loaded === loaded) return;
  entry.loaded = loaded;
  notify(entry);
}

function clearRetryTimer(entry: ResourceEntry): void {
  if (entry.retryTimer === null) return;
  clearTimeout(entry.retryTimer);
  entry.retryTimer = null;
}

function canScheduleRetry(store: ResourceStore, entry: ResourceEntry): boolean {
  return (
    isActiveEntry(store, entry) &&
    !entry.permanent &&
    entry.retryCount < store.maxRetries &&
    !entry.retryTimer
  );
}

function scheduleRetry(
  store: ResourceStore,
  entry: ResourceEntry,
  delayMs = store.retryDelayMs,
): void {
  if (!canScheduleRetry(store, entry)) return;
  const timer = setTimeout(() => {
    if (entry.retryTimer !== timer) return;
    entry.retryTimer = null;
    if (!isActiveEntry(store, entry) || entry.permanent || entry.retryCount >= store.maxRetries) {
      return;
    }
    entry.retryCount += 1;
    void startRequest(store, entry);
  }, delayMs);
  entry.retryTimer = timer;
}

function normalizeSyncResponse(result: TaskPRSyncResponse): TaskPR[] {
  if (!result) return [];
  const envelope = result as { prs?: TaskPR[] };
  if (Array.isArray(envelope.prs)) return envelope.prs;
  const single = result as TaskPR;
  return single.task_id ? [single] : [];
}

function canPublishRequest(
  store: ResourceStore,
  entry: ResourceEntry,
  requestGeneration: number,
): boolean {
  return entry.requestGeneration === requestGeneration && isActiveEntry(store, entry);
}

function publishResponse(
  store: ResourceStore,
  entry: ResourceEntry,
  requestGeneration: number,
  result: TaskPRSyncResponse,
): void {
  if (!canPublishRequest(store, entry, requestGeneration)) return;

  const permanent = Boolean((result as { permanent?: boolean } | null)?.permanent);
  const list = normalizeSyncResponse(result);
  if (permanent) {
    entry.permanent = true;
    entry.retryCount = store.maxRetries;
    clearRetryTimer(entry);
  }

  setLoaded(entry, true);
  if (list.length === 0) return;

  const state = storeState(store);
  if (
    !isCurrentWorkspaceContext(
      state,
      entry.scope.workspaceId,
      entry.scope.workspaceContextGeneration,
    )
  ) {
    return;
  }
  for (const pr of list) {
    if (pr.task_id) store.appStore.getState().setTaskPR(entry.scope.taskId, pr, entry.scope);
  }
  entry.retryCount = 0;
  clearRetryTimer(entry);
}

function publishFailure(
  store: ResourceStore,
  entry: ResourceEntry,
  requestGeneration: number,
): void {
  if (!canPublishRequest(store, entry, requestGeneration)) return;
  if (entry.retryCount >= store.maxRetries) setLoaded(entry, true);
}

async function runRequest(
  store: ResourceStore,
  entry: ResourceEntry,
  requestGeneration: number,
): Promise<void> {
  let result: TaskPRSyncResponse;
  try {
    result = await store.requester(entry.scope);
  } catch (error) {
    if (error !== NO_WEBSOCKET_CLIENT) publishFailure(store, entry, requestGeneration);
    return;
  }
  publishResponse(store, entry, requestGeneration, result);
}

function settleRequest(store: ResourceStore, entry: ResourceEntry, promise: Promise<void>): void {
  if (entry.promise !== promise) return;
  const requestStartedAt = entry.requestStartedAt;
  entry.promise = null;
  entry.requestStartedAt = null;
  if (!isCurrentEntry(store, entry)) return;
  if (entry.listeners.size === 0) {
    store.entries.delete(entry.key);
    return;
  }
  const state = storeState(store);
  if (hasTaskPRs(state, entry.scope)) clearRetryTimer(entry);
  else {
    const retryDelay =
      requestStartedAt === null
        ? store.retryDelayMs
        : Math.max(0, requestStartedAt + store.retryDelayMs - Date.now());
    scheduleRetry(store, entry, retryDelay);
  }
}

function startRequest(store: ResourceStore, entry: ResourceEntry): Promise<void> {
  if (entry.promise) return entry.promise;
  const requestGeneration = ++entry.requestGeneration;
  entry.requestStartedAt = Date.now();
  const promise = runRequest(store, entry, requestGeneration);
  entry.promise = promise;
  void promise.then(
    () => settleRequest(store, entry, promise),
    () => settleRequest(store, entry, promise),
  );
  return promise;
}

function invalidate(store: ResourceStore, scope: TaskPRSyncScope): void {
  const entry = store.entries.get(scopeKey(scope));
  if (!entry) return;
  entry.requestGeneration += 1;
}

function observeStore(store: ResourceStore, entry: ResourceEntry): void {
  entry.connectionStatus = storeState(store).connection.status;
  entry.stopStoreSubscription = store.appStore.subscribe((state) => {
    if (!isActiveEntry(store, entry)) return;
    const previousStatus = entry.connectionStatus;
    entry.connectionStatus = state.connection.status;
    if (state.connection.status === "connected" && previousStatus !== "connected") {
      entry.retryCount = 0;
      entry.permanent = false;
      clearRetryTimer(entry);
      entry.promise = null;
      entry.requestStartedAt = null;
      void startRequest(store, entry);
      return;
    }
    if (hasTaskPRs(state, entry.scope)) clearRetryTimer(entry);
    else if (!entry.promise) scheduleRetry(store, entry);
  });
}

function releaseEntry(store: ResourceStore, entry: ResourceEntry): void {
  clearRetryTimer(entry);
  entry.stopStoreSubscription?.();
  entry.stopStoreSubscription = null;
  if (entry.promise) return;
  if (isCurrentEntry(store, entry)) store.entries.delete(entry.key);
}

function subscribe(store: ResourceStore, scope: TaskPRSyncScope, listener: () => void): () => void {
  const entry = entryFor(store, scope);
  entry.listeners.add(listener);
  if (entry.listeners.size === 1) {
    observeStore(store, entry);
    if (!entry.promise) void startRequest(store, entry);
  }

  let released = false;
  return () => {
    if (released) return;
    released = true;
    entry.listeners.delete(listener);
    if (entry.listeners.size > 0) return;
    releaseEntry(store, entry);
  };
}

function createResource(
  store: StoreApi<AppState>,
  requester: TaskPRSyncRequester,
  options: TaskPRSyncResourceOptions,
): TaskPRSyncResource {
  const resourceStore: ResourceStore = {
    appStore: store,
    entries: new Map(),
    requester,
    retryDelayMs: Math.max(0, options.retryDelayMs ?? DEFAULT_RETRY_DELAY_MS),
    maxRetries: Math.max(0, options.maxRetries ?? DEFAULT_MAX_RETRIES),
  };
  return {
    getSnapshot: (scope) => resourceStore.entries.get(scopeKey(scope))?.loaded ?? false,
    invalidate: (scope) => invalidate(resourceStore, scope),
    refresh: (scope) => startRequest(resourceStore, entryFor(resourceStore, scope)),
    subscribe: (scope, listener) => subscribe(resourceStore, scope, listener),
  };
}

export function createTaskPRSyncResource(
  store: StoreApi<AppState>,
  requester: TaskPRSyncRequester = requestTaskPRSync,
  options: TaskPRSyncResourceOptions = {},
): TaskPRSyncResource {
  return createResource(store, requester, options);
}

const resourcesByStore = new WeakMap<StoreApi<AppState>, TaskPRSyncResource>();

export function getTaskPRSyncResource(store: StoreApi<AppState>): TaskPRSyncResource {
  const existing = resourcesByStore.get(store);
  if (existing) return existing;
  const resource = createTaskPRSyncResource(store);
  resourcesByStore.set(store, resource);
  return resource;
}
