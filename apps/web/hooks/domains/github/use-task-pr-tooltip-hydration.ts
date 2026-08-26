"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import type { StoreApi } from "zustand";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { listTaskPRs } from "@/lib/api/domains/github-api";
import type { AppState } from "@/lib/state/store";
import type { TaskPR } from "@/lib/types/github";

export type TaskPRTooltipHydrationStatus = "idle" | "loading" | "unavailable";

type TaskPRRequestRegistry = Map<string, Promise<TaskPR[]>>;

const requestRegistryByStore = new WeakMap<StoreApi<AppState>, TaskPRRequestRegistry>();

function hasTaskPRs(value: unknown): value is TaskPR[] {
  return Array.isArray(value) && value.length > 0;
}

function requestKey(workspaceId: string, taskId: string): string {
  return `${workspaceId}\u0000${taskId}`;
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
function mergeMissingTaskPRs(store: StoreApi<AppState>, taskId: string, prs: TaskPR[]): void {
  for (const pr of prs) {
    const current = store.getState().taskPRs.byTaskId[taskId];
    const currentPRs = Array.isArray(current) ? current : [];
    if (currentPRs.some((candidate) => isSamePR(candidate, pr))) continue;
    store.getState().setTaskPR(taskId, pr);
  }
}

function requestTaskPRs(
  store: StoreApi<AppState>,
  workspaceId: string,
  taskId: string,
): Promise<TaskPR[]> {
  const registry = getRequestRegistry(store);
  const key = requestKey(workspaceId, taskId);
  const existing = registry.get(key);
  if (existing) return existing;

  const request = listTaskPRs([taskId], { cache: "no-store" }).then((response) => {
    if (store.getState().workspaces.activeId !== workspaceId) return [];
    const prs = response.task_prs?.[taskId] ?? [];
    mergeMissingTaskPRs(store, taskId, prs);
    return prs;
  });
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
  const [status, setStatus] = useState<TaskPRTooltipHydrationStatus>("idle");
  const scopeKey = `${workspaceId ?? ""}\u0000${taskId}`;
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
    const isCurrentScope = () =>
      scopeRef.current.generation === generation &&
      store.getState().workspaces.activeId === workspaceId;
    const current = store.getState().taskPRs.byTaskId[taskId];
    if (hasTaskPRs(current)) {
      if (isCurrentScope()) setStatus("idle");
      return Promise.resolve(current);
    }
    if (!workspaceId) {
      if (isCurrentScope()) setStatus("unavailable");
      return Promise.resolve([]);
    }

    if (isCurrentScope()) setStatus("loading");
    return requestTaskPRs(store, workspaceId, taskId).then(
      (prs) => {
        if (isCurrentScope()) {
          const settled = store.getState().taskPRs.byTaskId[taskId];
          setStatus(hasTaskPRs(settled) ? "idle" : "unavailable");
        }
        return prs;
      },
      () => {
        if (isCurrentScope()) setStatus("unavailable");
        return [];
      },
    );
  }, [store, taskId, workspaceId]);

  return { status, hydrate };
}
