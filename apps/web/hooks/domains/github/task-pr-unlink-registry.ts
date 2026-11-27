import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { TaskPRScope } from "@/lib/state/slices/github/types";

export type TaskPRUnlinkKey = TaskPRScope & {
  taskId: string;
  associationId: string;
};

type PendingTaskPRUnlink = TaskPRUnlinkKey;

type Registry = {
  pending: Map<string, PendingTaskPRUnlink>;
  revision: number;
  listeners: Set<() => void>;
};

const registries = new WeakMap<StoreApi<AppState>, Registry>();

function getRegistry(store: StoreApi<AppState>): Registry {
  const existing = registries.get(store);
  if (existing) return existing;
  const registry: Registry = { pending: new Map(), revision: 0, listeners: new Set() };
  registries.set(store, registry);
  return registry;
}

function keyOf(key: TaskPRUnlinkKey): string {
  return JSON.stringify([
    key.workspaceId,
    key.workspaceContextGeneration,
    key.taskId,
    key.associationId,
  ]);
}

function notify(registry: Registry): void {
  registry.revision += 1;
  for (const listener of registry.listeners) listener();
}

export function subscribeTaskPRUnlinkPending(
  store: StoreApi<AppState>,
  listener: () => void,
): () => void {
  const registry = getRegistry(store);
  registry.listeners.add(listener);
  return () => registry.listeners.delete(listener);
}

export function getTaskPRUnlinkPendingRevision(store: StoreApi<AppState>): number {
  return getRegistry(store).revision;
}

export function getTaskPRUnlinkPendingIds(
  store: StoreApi<AppState>,
  scope: TaskPRScope & { taskId: string },
): ReadonlySet<string> {
  const ids = new Set<string>();
  for (const pending of getRegistry(store).pending.values()) {
    if (
      pending.workspaceId === scope.workspaceId &&
      pending.workspaceContextGeneration === scope.workspaceContextGeneration &&
      pending.taskId === scope.taskId
    ) {
      ids.add(pending.associationId);
    }
  }
  return ids;
}

export function beginTaskPRUnlink(store: StoreApi<AppState>, key: TaskPRUnlinkKey): boolean {
  const registry = getRegistry(store);
  const encodedKey = keyOf(key);
  if (registry.pending.has(encodedKey)) return false;
  registry.pending.set(encodedKey, key);
  notify(registry);
  return true;
}

export function endTaskPRUnlink(store: StoreApi<AppState>, key: TaskPRUnlinkKey): void {
  const registry = getRegistry(store);
  if (!registry.pending.delete(keyOf(key))) return;
  notify(registry);
}
