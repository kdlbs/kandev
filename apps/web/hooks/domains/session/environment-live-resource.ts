import { ApiError } from "@/lib/api/client";
import {
  fetchTaskEnvironmentLive,
  type TaskEnvironmentLiveResponse,
} from "@/lib/api/domains/task-environment-api";

export type EnvironmentLiveSnapshot = {
  response: TaskEnvironmentLiveResponse | null;
  loading: boolean;
  notFound: boolean;
};

type EnvironmentLiveRequester = (taskId: string) => Promise<TaskEnvironmentLiveResponse>;

type Entry = {
  owner: object | null;
  taskId: string;
  snapshot: EnvironmentLiveSnapshot;
  consumers: Map<symbol, boolean>;
  listeners: Map<symbol, () => void>;
  timer: ReturnType<typeof setInterval> | null;
  intervalMs: number | null;
  request: Promise<void> | null;
  generation: number;
  refreshAfterRequest: boolean;
};

const EMPTY_SNAPSHOT: EnvironmentLiveSnapshot = {
  response: null,
  loading: false,
  notFound: false,
};
const ACTIVE_INTERVAL_MS = 3000;
const BACKGROUND_INTERVAL_MS = 7000;

type Registry = {
  requester: EnvironmentLiveRequester;
  fallbackEntries: Map<string, Entry>;
  storeEntries: WeakMap<object, Map<string, Entry>>;
};

function entriesFor(registry: Registry, owner: object | null, create: boolean) {
  if (!owner) return registry.fallbackEntries;
  const existing = registry.storeEntries.get(owner);
  if (existing || !create) return existing;
  const entries = new Map<string, Entry>();
  registry.storeEntries.set(owner, entries);
  return entries;
}

function getEntry(registry: Registry, owner: object | null, taskId: string) {
  return entriesFor(registry, owner, false)?.get(taskId);
}

function createEntry(registry: Registry, owner: object | null, taskId: string): Entry {
  const entry: Entry = {
    owner,
    taskId,
    snapshot: EMPTY_SNAPSHOT,
    consumers: new Map(),
    listeners: new Map(),
    timer: null,
    intervalMs: null,
    request: null,
    generation: 0,
    refreshAfterRequest: false,
  };
  entriesFor(registry, owner, true)!.set(taskId, entry);
  return entry;
}

function entryFor(registry: Registry, owner: object | null, taskId: string): Entry {
  return getEntry(registry, owner, taskId) ?? createEntry(registry, owner, taskId);
}

function setSnapshot(entry: Entry, snapshot: EnvironmentLiveSnapshot): void {
  entry.snapshot = snapshot;
  entry.listeners.forEach((listener) => listener());
}

function stopTimer(entry: Entry): void {
  if (entry.timer !== null) clearInterval(entry.timer);
  entry.timer = null;
  entry.intervalMs = null;
}

function retireIfUnused(registry: Registry, entry: Entry): void {
  if (entry.consumers.size !== 0 || entry.request) return;
  const entries = entriesFor(registry, entry.owner, false);
  if (entries?.get(entry.taskId) === entry) entries.delete(entry.taskId);
}

function updateTimer(registry: Registry, entry: Entry): void {
  if (entry.consumers.size === 0) {
    stopTimer(entry);
    return;
  }
  const intervalMs = [...entry.consumers.values()].some(Boolean)
    ? ACTIVE_INTERVAL_MS
    : BACKGROUND_INTERVAL_MS;
  if (entry.intervalMs === intervalMs && entry.timer !== null) return;
  stopTimer(entry);
  entry.intervalMs = intervalMs;
  entry.timer = setInterval(() => {
    void loadEntry(registry, entry);
  }, intervalMs);
}

function loadEntry(registry: Registry, entry: Entry): Promise<void> {
  if (entry.request) return entry.request;
  const generation = entry.generation;
  setSnapshot(entry, { ...entry.snapshot, loading: true });
  const request = registry
    .requester(entry.taskId)
    .then(
      (response) => {
        if (generation === entry.generation) {
          setSnapshot(entry, { response, loading: false, notFound: false });
        }
      },
      (error: unknown) => {
        if (generation !== entry.generation) return;
        if (error instanceof ApiError && error.status === 404) {
          setSnapshot(entry, { response: null, loading: false, notFound: true });
          return;
        }
        setSnapshot(entry, { ...entry.snapshot, loading: false });
      },
    )
    .finally(() => {
      if (entry.request === request) entry.request = null;
      if (entry.refreshAfterRequest && entry.consumers.size > 0) {
        entry.refreshAfterRequest = false;
        void loadEntry(registry, entry);
        return;
      }
      retireIfUnused(registry, entry);
    });
  entry.request = request;
  return request;
}

/** Shares raw task-environment reads within one store and task identity. */
export function createEnvironmentLiveResource(
  requester: EnvironmentLiveRequester = fetchTaskEnvironmentLive,
) {
  const registry: Registry = {
    requester,
    fallbackEntries: new Map(),
    storeEntries: new WeakMap(),
  };
  return {
    getSnapshot(owner: object | null, taskId: string | null | undefined): EnvironmentLiveSnapshot {
      if (!taskId) return EMPTY_SNAPSHOT;
      return getEntry(registry, owner, taskId)?.snapshot ?? EMPTY_SNAPSHOT;
    },
    subscribe(owner: object | null, taskId: string, consumerId: symbol, listener: () => void) {
      const entry = entryFor(registry, owner, taskId);
      entry.consumers.set(consumerId, entry.consumers.get(consumerId) ?? false);
      entry.listeners.set(consumerId, listener);
      updateTimer(registry, entry);
      if (!entry.snapshot.response && !entry.snapshot.notFound && !entry.request) {
        void loadEntry(registry, entry);
      }
      return () => {
        entry.consumers.delete(consumerId);
        entry.listeners.delete(consumerId);
        updateTimer(registry, entry);
        retireIfUnused(registry, entry);
      };
    },
    setActive(owner: object | null, taskId: string, consumerId: symbol, active: boolean): void {
      const entry = getEntry(registry, owner, taskId);
      if (!entry || !entry.consumers.has(consumerId)) return;
      entry.consumers.set(consumerId, active);
      updateTimer(registry, entry);
    },
    refresh(owner: object | null, taskId: string | null | undefined): Promise<void> {
      if (!taskId) return Promise.resolve();
      const entry = getEntry(registry, owner, taskId);
      return entry ? loadEntry(registry, entry) : Promise.resolve();
    },
    invalidate(owner: object | null, taskId: string | null | undefined): void {
      if (!taskId) return;
      const entry = getEntry(registry, owner, taskId);
      if (!entry) return;
      entry.generation += 1;
      setSnapshot(entry, EMPTY_SNAPSHOT);
      if (entry.request) {
        entry.refreshAfterRequest = true;
      } else if (entry.consumers.size > 0) {
        void loadEntry(registry, entry);
      }
    },
  };
}

export const environmentLiveResource = createEnvironmentLiveResource();
