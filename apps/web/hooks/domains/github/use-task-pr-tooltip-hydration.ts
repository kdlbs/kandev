"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { StoreApi } from "zustand";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { getTaskCIAutomationOptions, listTaskPRs } from "@/lib/api/domains/github-api";
import type { AppState } from "@/lib/state/store";
import { isCurrentWorkspaceContext } from "@/lib/state/workspace-context";
import type { TaskPRScope } from "@/lib/state/slices/github/types";
import type { TaskCIAutomationOptions, TaskPR } from "@/lib/types/github";

export type TaskPRTooltipHydrationStatus = "idle" | "loading" | "unavailable";

type TaskPRRequestRegistry = Map<string, Promise<TaskPR[]>>;
type TaskAutomationRequestRegistry = Map<string, Promise<TaskCIAutomationOptions | null>>;

const requestRegistryByStore = new WeakMap<StoreApi<AppState>, TaskPRRequestRegistry>();
const automationRequestRegistryByStore = new WeakMap<
  StoreApi<AppState>,
  TaskAutomationRequestRegistry
>();

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

function getAutomationRequestRegistry(store: StoreApi<AppState>): TaskAutomationRequestRegistry {
  const existing = automationRequestRegistryByStore.get(store);
  if (existing) return existing;
  const registry: TaskAutomationRequestRegistry = new Map();
  automationRequestRegistryByStore.set(store, registry);
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

function requestTaskAutomationOptions(
  store: StoreApi<AppState>,
  scope: TaskPRScope & { workspaceId: string },
  taskId: string,
): Promise<TaskCIAutomationOptions | null> {
  if (typeof getTaskCIAutomationOptions !== "function") return Promise.resolve(null);
  const registry = getAutomationRequestRegistry(store);
  const key = requestKey(scope.workspaceId, scope.workspaceContextGeneration, taskId);
  const existing = registry.get(key);
  if (existing) return existing;

  const request = Promise.resolve(getTaskCIAutomationOptions(taskId, { cache: "no-store" }));
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

function automationOptionsForScope(
  options: TaskCIAutomationOptions | null | undefined,
  workspaceId: string | null,
): TaskCIAutomationOptions | null {
  if (!options) return null;
  if (options.workspace_id && workspaceId && options.workspace_id !== workspaceId) return null;
  return options;
}

export function useTaskPRTooltipHydration(
  taskId: string,
  hydrationOptions: { includeAutomation?: boolean } = {},
): {
  status: TaskPRTooltipHydrationStatus;
  hydrate: () => Promise<TaskPR[]>;
  automationOptions: TaskCIAutomationOptions | null;
} {
  const store = useAppStoreApi();
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  const workspaceContextGeneration = useAppStore((state) => state.workspaceContextGeneration);
  const storedAutomationOptions = useAppStore((state) =>
    automationOptionsForScope(state.taskCIAutomation?.byTaskId?.[taskId], workspaceId),
  );
  const [status, setStatus] = useState<TaskPRTooltipHydrationStatus>("idle");
  const [hydratedAutomationOptions, setHydratedAutomationOptions] =
    useState<TaskCIAutomationOptions | null>(null);
  const includeAutomation = hydrationOptions.includeAutomation === true;
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
    setHydratedAutomationOptions(null);
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
    const cachedAutomation = automationOptionsForScope(
      current.taskCIAutomation?.byTaskId?.[taskId],
      scope.workspaceId,
    );
    if (cached && (!includeAutomation || cachedAutomation)) {
      if (isCurrentScope()) {
        setHydratedAutomationOptions(cachedAutomation);
        setStatus("idle");
      }
      return Promise.resolve(cached);
    }

    if (isCurrentScope()) setStatus("loading");
    const prRequest = cached ? Promise.resolve(cached) : requestTaskPRs(store, scope, taskId);
    let automationRequest: Promise<TaskCIAutomationOptions | null> = Promise.resolve(null);
    if (includeAutomation) {
      automationRequest = cachedAutomation
        ? Promise.resolve(cachedAutomation)
        : requestTaskAutomationOptions(store, scope, taskId);
    }
    return Promise.allSettled([prRequest, automationRequest]).then((results) => {
      const prResult = results[0];
      const automationResult = results[1];
      const prs = prResult.status === "fulfilled" ? prResult.value : [];
      const automation = automationResult.status === "fulfilled" ? automationResult.value : null;
      if (isCurrentScope()) {
        if (prs.length > 0) mergeMissingTaskPRs(store, taskId, prs, scope);
        if (automation) {
          store.getState().setTaskCIAutomationOptions(taskId, automation);
          setHydratedAutomationOptions(automation);
        }
        setStatus(getTaskPRsForScope(store.getState(), taskId, scope) ? "idle" : "unavailable");
      }
      return prs;
    });
  }, [includeAutomation, store, taskId, workspaceContextGeneration, workspaceId]);

  return useMemo(
    () => ({
      status,
      hydrate,
      // Prefer the store entry because WebSocket updates replace the fetched
      // snapshot while the disclosure remains mounted. Fall back to the local
      // snapshot for compatibility with callers/tests that provide no store
      // entry (for example, a request that completed before state hydration).
      automationOptions: storedAutomationOptions ?? hydratedAutomationOptions,
    }),
    [status, hydrate, storedAutomationOptions, hydratedAutomationOptions],
  );
}
