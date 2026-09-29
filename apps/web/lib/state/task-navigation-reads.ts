import type { StoreApi } from "zustand";
import { fetchTask, listTaskSessions } from "@/lib/api";
import { getBackendConfig } from "@/lib/config";
import type { Task, TaskSessionsResponse } from "@/lib/types/http";
import type { AppState } from "./store";

export type TaskNavigationIdentity = {
  task: Task;
  allSessionsResponse: TaskSessionsResponse;
};

export type TaskNavigationContext = {
  readonly ownerToken: object;
  readonly identity: string;
  readonly generation: number;
};

type LoadTaskNavigationIdentity = (taskId: string) => Promise<TaskNavigationIdentity>;

const MAX_RETAINED_READS = 8;

export class TaskNavigationReads {
  private generation = 0;
  private owner: object | null = null;
  private routeKey: string | null = null;
  private requestSequence = 0;
  private reads = new Map<string, Promise<TaskNavigationIdentity>>();

  constructor(
    private loadIdentity: LoadTaskNavigationIdentity,
    private scopeIsCurrent: () => boolean = () => true,
  ) {}

  beginNavigation(owner: object, routeKey: string): number {
    if (this.owner === owner && this.routeKey === routeKey) return this.generation;
    this.owner = owner;
    this.routeKey = routeKey;
    this.generation++;
    this.reads.clear();
    return this.generation;
  }

  currentGeneration() {
    return this.generation;
  }

  isCurrent(generation: number) {
    return generation === this.generation && this.scopeIsCurrent();
  }

  read(
    taskId: string,
    generation = this.generation,
    options: { refresh?: boolean } = {},
  ): Promise<TaskNavigationIdentity> {
    if (!this.scopeIsCurrent()) {
      return Promise.reject(new Error("Task navigation read scope changed"));
    }
    const requestId = options.refresh ? `refresh:${++this.requestSequence}` : "shared";
    const key = JSON.stringify([generation, taskId, requestId]);
    const existing = this.reads.get(key);
    if (existing) return existing;

    const promise = Promise.resolve(this.loadIdentity(taskId)).then((identity) => {
      if (!this.scopeIsCurrent()) {
        throw new Error("Task navigation read scope changed");
      }
      return identity;
    });
    this.reads.set(key, promise);
    while (this.reads.size > MAX_RETAINED_READS) {
      const oldest = this.reads.keys().next().value;
      if (oldest === undefined) break;
      this.reads.delete(oldest);
    }
    return promise;
  }
}

export function createTaskNavigationReads(
  loadIdentity: LoadTaskNavigationIdentity,
  scopeIsCurrent?: () => boolean,
): TaskNavigationReads {
  return new TaskNavigationReads(loadIdentity, scopeIsCurrent);
}

function fetchTaskNavigationIdentity(taskId: string): Promise<TaskNavigationIdentity> {
  return Promise.all([
    fetchTask(taskId, { cache: "no-store" }),
    listTaskSessions(taskId, { cache: "no-store" }),
  ]).then(([task, allSessionsResponse]) => ({ task, allSessionsResponse }));
}

function storeIdentity(state: AppState) {
  return JSON.stringify([
    getBackendConfig().apiBaseUrl,
    state.auth.mode,
    state.auth.authenticated,
    state.auth.user?.id,
    state.workspaceContextGeneration,
  ]);
}

type NavigationReadOwner = {
  readonly identity: string;
  readonly token: object;
  reads: TaskNavigationReads;
  navigationContext?: {
    navigationOwner: object;
    routeKey: string;
    context: TaskNavigationContext;
  };
};

const owners = new WeakMap<StoreApi<AppState>, NavigationReadOwner>();

function getTaskNavigationOwner(store: StoreApi<AppState>): NavigationReadOwner {
  const identity = storeIdentity(store.getState());
  let owner = owners.get(store);
  if (!owner || owner.identity !== identity) {
    const nextOwner = {
      identity,
      token: Object.freeze({}),
    } as NavigationReadOwner;
    nextOwner.reads = new TaskNavigationReads(
      fetchTaskNavigationIdentity,
      () => owners.get(store) === nextOwner && storeIdentity(store.getState()) === identity,
    );
    owner = nextOwner;
    owners.set(store, nextOwner);
  }
  return owner;
}

export function beginTaskNavigation(
  store: StoreApi<AppState>,
  owner: object,
  routeKey: string,
): TaskNavigationContext {
  const readOwner = getTaskNavigationOwner(store);
  const generation = readOwner.reads.beginNavigation(owner, routeKey);
  const previous = readOwner.navigationContext;
  if (
    previous?.navigationOwner === owner &&
    previous.routeKey === routeKey &&
    previous.context.generation === generation
  ) {
    return previous.context;
  }
  const context = Object.freeze({
    ownerToken: readOwner.token,
    identity: readOwner.identity,
    generation,
  });
  readOwner.navigationContext = { navigationOwner: owner, routeKey, context };
  return context;
}

export function currentTaskNavigationGeneration(store: StoreApi<AppState>): number {
  return getTaskNavigationOwner(store).reads.currentGeneration();
}

export function isTaskNavigationCurrent(
  store: StoreApi<AppState>,
  context: TaskNavigationContext,
): boolean {
  const owner = owners.get(store);
  return (
    owner?.token === context.ownerToken &&
    owner.identity === context.identity &&
    storeIdentity(store.getState()) === context.identity &&
    owner.reads.isCurrent(context.generation)
  );
}

export function readTaskNavigationIdentity(
  store: StoreApi<AppState>,
  taskId: string,
  options: { context?: TaskNavigationContext; generation?: number; refresh?: boolean } = {},
): Promise<TaskNavigationIdentity> {
  const owner = getTaskNavigationOwner(store);
  if (
    options.context &&
    (owner.token !== options.context.ownerToken || owner.identity !== options.context.identity)
  ) {
    return Promise.reject(new Error("Task navigation read scope changed"));
  }
  return owner.reads.read(taskId, options.context?.generation ?? options.generation, {
    refresh: options.refresh,
  });
}
