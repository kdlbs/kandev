"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import type { StoreApi } from "zustand";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { listTaskPRs } from "@/lib/api/domains/github-api";
import type { AppState } from "@/lib/state/store";
import { isCurrentWorkspaceContext } from "@/lib/state/workspace-context";
import type { TaskPRScope } from "@/lib/state/slices/github/types";
import type { TaskPR } from "@/lib/types/github";

export type TaskPRTooltipHydrationStatus = "idle" | "loading" | "unavailable";

type TaskPRRequestRegistry = Map<string, Promise<TaskPR[]>>;

const requestRegistryByStore = new WeakMap<StoreApi<AppState>, TaskPRRequestRegistry>();

function hasTaskPRs(value: unknown): value is TaskPR[] {
  return Array.isArray(value) && value.length > 0;
}

function requestKey(
  workspaceId: string,
  workspaceContextGeneration: number,
  taskId: string,
): string {
  // The NUL delimiter is only an in-memory scope-key boundary; API IDs are
  // opaque stable values and are not expected to contain it.
  return `${workspaceId}\u0000${workspaceContextGeneration}\u0000${taskId}`;
}

function getRequestRegistry(store: StoreApi<AppState>): TaskPRRequestRegistry {
  const existing = requestRegistryByStore.get(store);
  if (existing) return existing;
  const registry: TaskPRRequestRegistry = new Map();
  requestRegistryByStore.set(store, registry);
  return registry;
}

function isSamePR(left: TaskPR, right: TaskPR): boolean {
  if (left.id && right.id && left.id === right.id) return true;
  return (
    (left.repository_id ?? "") === (right.repository_id ?? "") && left.pr_number === right.pr_number
  );
}

/** Merge only missing identities so a newer WebSocket row remains authoritative. */
function getTaskPRsForScope(state: AppState, taskId: string, scope: TaskPRScope): TaskPR[] | null {
  if (
    state.taskPRs.workspaceId === scope.workspaceId &&
    state.taskPRs.workspaceContextGeneration === scope.workspaceContextGeneration &&
    hasTaskPRs(state.taskPRs.byTaskId[taskId])
  ) {
    return state.taskPRs.byTaskId[taskId];
  }
  return null;
}

export function getTaskPRsForCurrentWorkspace(state: AppState, taskId: string): TaskPR[] | null {
  return getTaskPRsForScope(state, taskId, {
    workspaceId: state.workspaces.activeId,
    workspaceContextGeneration: state.workspaceContextGeneration,
  });
}

function mergeMissingTaskPRs(
  store: StoreApi<AppState>,
  taskId: string,
  prs: TaskPR[],
  scope: TaskPRScope,
): void {
  for (const pr of prs) {
    const taskPRs = store.getState().taskPRs;
    if (taskPRs.deletedAssociationIdsByTaskId?.[taskId]?.[pr.id]) continue;
    const current = taskPRs.byTaskId[taskId];
    const currentPRs = Array.isArray(current) ? current : [];
    if (currentPRs.some((candidate) => isSamePR(candidate, pr))) continue;
    store.getState().setTaskPR(taskId, pr, scope);
  }
}

function requestTaskPRs(
  store: StoreApi<AppState>,
  scope: TaskPRScope & { workspaceId: string },
  taskId: string,
): Promise<TaskPR[]> {
  const registry = getRequestRegistry(store);
  const key = requestKey(scope.workspaceId, scope.workspaceContextGeneration, taskId);
  const existing = registry.get(key);
  if (existing) return existing;

  const request = listTaskPRs([taskId], { cache: "no-store" }).then(
    (response) => response.task_prs?.[taskId] ?? [],
  );
  registry.set(key, request);
  request.then(
    () => {
      if (registry.get(key) === request) registry.delete(key);
    },
    () => {
      if (registry.get(key) === request) registry.delete(key);
    },
  );
  return request;
}

export function useTaskPRTooltipHydration(taskId: string): {
  status: TaskPRTooltipHydrationStatus;
  hydrate: () => Promise<TaskPR[]>;
} {
  const store = useAppStoreApi();
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  const workspaceContextGeneration = useAppStore((state) => state.workspaceContextGeneration);
  const [status, setStatus] = useState<TaskPRTooltipHydrationStatus>("idle");
  const scopeKey = `${workspaceId ?? ""}\u0000${workspaceContextGeneration}\u0000${taskId}`;
  const scopeRef = useRef({ key: scopeKey, generation: 0 });
  if (scopeRef.current.key !== scopeKey) {
    scopeRef.current = {
      key: scopeKey,
      generation: scopeRef.current.generation + 1,
    };
  }
  useEffect(() => {
    setStatus("idle");
  }, [scopeKey]);

  const hydrate = useCallback(() => {
    const generation = scopeRef.current.generation;
    const scope = workspaceId ? { workspaceId, workspaceContextGeneration } : null;
    const isCurrentScope = () =>
      scopeRef.current.generation === generation &&
      scope !== null &&
      isCurrentWorkspaceContext(
        store.getState(),
        scope.workspaceId,
        scope.workspaceContextGeneration,
      );
    if (!scope) {
      if (scopeRef.current.generation === generation) setStatus("unavailable");
      return Promise.resolve([]);
    }
    const current = store.getState();
    const cached = getTaskPRsForScope(current, taskId, scope);
    if (cached) {
      if (isCurrentScope()) setStatus("idle");
      return Promise.resolve(cached);
    }

    if (isCurrentScope()) setStatus("loading");
    return requestTaskPRs(store, scope, taskId).then(
      (prs) => {
        if (isCurrentScope()) {
          mergeMissingTaskPRs(store, taskId, prs, scope);
          setStatus(getTaskPRsForScope(store.getState(), taskId, scope) ? "idle" : "unavailable");
        }
        return prs;
      },
      () => {
        if (isCurrentScope()) setStatus("unavailable");
        return [];
      },
    );
  }, [store, taskId, workspaceContextGeneration, workspaceId]);

  return { status, hydrate };
}
