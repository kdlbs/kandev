import { useSyncExternalStore } from "react";
import type { StoreApi } from "zustand";
import { ApiError } from "@/lib/api/client";
import { fetchTask, listTaskSessions } from "@/lib/api";
import { getBackendConfig } from "@/lib/config";
import type { Task, TaskSessionsResponse } from "@/lib/types/http";
import type { AppState } from "./store";

export type TaskNavigationIdentity = {
  task: Task;
  allSessionsResponse: TaskSessionsResponse;
  sessionListUnavailable?: boolean;
};

export type TaskNavigationContext = {
  readonly ownerToken: object;
  readonly identity: string;
  readonly generation: number;
};

export type TaskNavigationReadSnapshot = {
  phase: "idle" | "loading" | "retrying" | "succeeded" | "failed";
  attempt: number;
  identity?: TaskNavigationIdentity;
  error?: unknown;
  temporary: boolean;
  revision: number;
  cycle: number;
};

type LoadTaskNavigationIdentity = (
  taskId: string,
  signal: AbortSignal,
) => Promise<TaskNavigationIdentity>;

type TaskNavigationReadRecord = {
  generation: number;
  taskId: string;
  snapshot: TaskNavigationReadSnapshot;
  promise?: Promise<TaskNavigationIdentity>;
  controller?: AbortController;
  lastForegroundRecoveryEpisode?: object;
};

const MAX_RETAINED_READS = 8;
const TASK_NAVIGATION_ATTEMPT_TIMEOUT_MS = 10_000;
const RETRY_DELAYS_MS = [2_000, 5_000] as const;
const IDLE_SNAPSHOT: TaskNavigationReadSnapshot = Object.freeze({
  phase: "idle",
  attempt: 0,
  temporary: false,
  revision: 0,
  cycle: 0,
});

function readKey(generation: number, taskId: string) {
  return JSON.stringify([generation, taskId]);
}

export function isTaskNavigationAbort(error: unknown): boolean {
  return error instanceof Error && error.name === "AbortError";
}

export function isTemporaryTaskNavigationError(error: unknown): boolean {
  if (error instanceof TypeError) return true;
  if (!(error instanceof ApiError)) return false;
  if ([429, 502, 503, 504].includes(error.status)) return true;
  return (
    error.body !== null &&
    typeof error.body === "object" &&
    "code" in error.body &&
    (error.body as { code?: unknown }).code === "persistence_unavailable"
  );
}

function abortError(): Error {
  const error = new Error("Task navigation read aborted");
  error.name = "AbortError";
  return error;
}

function raceAbort<T>(promise: Promise<T>, signal: AbortSignal): Promise<T> {
  if (signal.aborted) return Promise.reject(abortError());
  return new Promise<T>((resolve, reject) => {
    const onAbort = () => reject(abortError());
    signal.addEventListener("abort", onAbort, { once: true });
    promise.then(resolve, reject).finally(() => signal.removeEventListener("abort", onAbort));
  });
}

async function loadNavigationAttempt(
  taskId: string,
  cycleSignal: AbortSignal,
  loadIdentity: LoadTaskNavigationIdentity,
): Promise<TaskNavigationIdentity> {
  const attemptController = new AbortController();
  const abortAttempt = () => attemptController.abort();
  let timedOut = false;
  if (cycleSignal.aborted) attemptController.abort();
  else cycleSignal.addEventListener("abort", abortAttempt, { once: true });
  const timeout = setTimeout(() => {
    timedOut = true;
    attemptController.abort();
  }, TASK_NAVIGATION_ATTEMPT_TIMEOUT_MS);
  try {
    return await raceAbort(
      loadIdentity(taskId, attemptController.signal),
      attemptController.signal,
    );
  } catch (error) {
    if (timedOut && !cycleSignal.aborted) {
      throw new TypeError("Task navigation read timed out");
    }
    throw error;
  } finally {
    clearTimeout(timeout);
    cycleSignal.removeEventListener("abort", abortAttempt);
    attemptController.abort();
  }
}

function waitForVisibleRetry(delayMs: number, signal: AbortSignal): Promise<void> {
  if (signal.aborted) return Promise.reject(abortError());
  return new Promise<void>((resolve, reject) => {
    let remaining = delayMs;
    let startedAt = 0;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const hasVisibility = typeof document !== "undefined";
    const visible = () => !hasVisibility || document.visibilityState !== "hidden";
    const cleanup = () => {
      if (timer !== undefined) clearTimeout(timer);
      if (hasVisibility) document.removeEventListener("visibilitychange", onVisibilityChange);
      signal.removeEventListener("abort", onAbort);
    };
    const finish = () => {
      cleanup();
      resolve();
    };
    const onAbort = () => {
      cleanup();
      reject(abortError());
    };
    const schedule = () => {
      if (!visible() || signal.aborted) return;
      startedAt = Date.now();
      timer = setTimeout(finish, remaining);
    };
    const onVisibilityChange = () => {
      if (!visible() && timer !== undefined) {
        clearTimeout(timer);
        timer = undefined;
        remaining = Math.max(0, remaining - (Date.now() - startedAt));
      } else if (visible() && timer === undefined) {
        schedule();
      }
    };
    if (hasVisibility) document.addEventListener("visibilitychange", onVisibilityChange);
    signal.addEventListener("abort", onAbort, { once: true });
    schedule();
  });
}

export class TaskNavigationReads {
  private generation = 0;
  private owner: object | null = null;
  private routeKey: string | null = null;
  private records = new Map<string, TaskNavigationReadRecord>();
  private listeners = new Map<string, Set<() => void>>();
  private retained = new Map<string, number>();

  constructor(
    private loadIdentity: LoadTaskNavigationIdentity,
    private scopeIsCurrent: () => boolean = () => true,
  ) {}

  beginNavigation(owner: object, routeKey: string): number {
    if (this.owner === owner && this.routeKey === routeKey) return this.generation;
    this.cancelAll();
    this.owner = owner;
    this.routeKey = routeKey;
    this.generation++;
    this.records.clear();
    return this.generation;
  }

  currentGeneration() {
    return this.generation;
  }

  isCurrent(generation: number) {
    return generation === this.generation && this.scopeIsCurrent();
  }

  getSnapshot(taskId: string, generation = this.generation): TaskNavigationReadSnapshot {
    return this.records.get(readKey(generation, taskId))?.snapshot ?? IDLE_SNAPSHOT;
  }

  subscribe(taskId: string, generation: number, listener: () => void): () => void {
    const key = readKey(generation, taskId);
    const listeners = this.listeners.get(key) ?? new Set<() => void>();
    listeners.add(listener);
    this.listeners.set(key, listeners);
    return () => {
      listeners.delete(listener);
      if (listeners.size === 0) this.listeners.delete(key);
    };
  }

  retain(taskId: string, generation: number): () => void {
    const key = readKey(generation, taskId);
    this.retained.set(key, (this.retained.get(key) ?? 0) + 1);
    let released = false;
    return () => {
      if (released) return;
      released = true;
      const count = (this.retained.get(key) ?? 1) - 1;
      if (count > 0) this.retained.set(key, count);
      else {
        this.retained.delete(key);
        queueMicrotask(() => {
          if (this.generation === generation && !this.retained.has(key)) this.cancel(key);
        });
      }
    };
  }

  read(
    taskId: string,
    generation = this.generation,
    options: {
      refresh?: boolean;
      manualRetry?: boolean;
      supersede?: boolean;
      foregroundRecoveryEpisode?: object;
    } = {},
  ): Promise<TaskNavigationIdentity> {
    if (!this.scopeIsCurrent() || generation !== this.generation)
      return Promise.reject(abortError());
    const key = readKey(generation, taskId);
    const record = this.records.get(key) ?? this.createRecord(taskId, generation, key);
    return this.reuseRead(record, options) ?? this.start(record);
  }

  dispose() {
    this.cancelAll();
    this.records.clear();
  }

  private createRecord(taskId: string, generation: number, key: string) {
    const record: TaskNavigationReadRecord = { taskId, generation, snapshot: IDLE_SNAPSHOT };
    this.records.set(key, record);
    while (this.records.size > MAX_RETAINED_READS) {
      const oldestEligible = [...this.records.keys()].find((candidateKey) => {
        if (candidateKey === key || this.retained.has(candidateKey)) return false;
        const candidate = this.records.get(candidateKey);
        return !candidate?.promise && !this.listeners.get(candidateKey)?.size;
      });
      if (!oldestEligible) break;
      this.records.delete(oldestEligible);
    }
    return record;
  }

  private reuseRead(
    record: TaskNavigationReadRecord,
    options: {
      refresh?: boolean;
      manualRetry?: boolean;
      supersede?: boolean;
      foregroundRecoveryEpisode?: object;
    },
  ): Promise<TaskNavigationIdentity> | undefined {
    const activeRead = this.joinActiveRead(record, options);
    if (activeRead) return activeRead;
    const repeatedEpisode = this.reuseRepeatedForegroundEpisode(
      record,
      options.foregroundRecoveryEpisode,
    );
    if (repeatedEpisode) return repeatedEpisode;
    const canRecoverForeground = this.beginForegroundRecoveryEpisode(
      record,
      options.foregroundRecoveryEpisode,
    );
    if (this.isBlockedFailure(record, options, canRecoverForeground)) {
      return Promise.reject(record.snapshot.error);
    }
    if (record.snapshot.identity && !options.refresh && !options.manualRetry) {
      return Promise.resolve(record.snapshot.identity);
    }
    if (options.foregroundRecoveryEpisode) {
      record.lastForegroundRecoveryEpisode = options.foregroundRecoveryEpisode;
    }
    return undefined;
  }

  private joinActiveRead(
    record: TaskNavigationReadRecord,
    options: { supersede?: boolean; foregroundRecoveryEpisode?: object },
  ): Promise<TaskNavigationIdentity> | undefined {
    if (!record.promise) return undefined;
    if (options.foregroundRecoveryEpisode) {
      record.lastForegroundRecoveryEpisode = options.foregroundRecoveryEpisode;
    }
    if (!options.supersede) return record.promise;
    record.controller?.abort();
    return undefined;
  }

  private reuseRepeatedForegroundEpisode(
    record: TaskNavigationReadRecord,
    episode?: object,
  ): Promise<TaskNavigationIdentity> | undefined {
    if (!episode || episode !== record.lastForegroundRecoveryEpisode) return undefined;
    if (record.snapshot.phase === "failed") return Promise.reject(record.snapshot.error);
    if (record.snapshot.identity) return Promise.resolve(record.snapshot.identity);
    return undefined;
  }

  private beginForegroundRecoveryEpisode(
    record: TaskNavigationReadRecord,
    episode?: object,
  ): boolean {
    if (
      !episode ||
      episode === record.lastForegroundRecoveryEpisode ||
      record.snapshot.phase !== "failed" ||
      !record.snapshot.temporary
    ) {
      return false;
    }
    record.lastForegroundRecoveryEpisode = episode;
    return true;
  }

  private isBlockedFailure(
    record: TaskNavigationReadRecord,
    options: { manualRetry?: boolean; supersede?: boolean },
    canRecoverForeground: boolean,
  ) {
    return (
      record.snapshot.phase === "failed" &&
      !options.manualRetry &&
      !options.supersede &&
      !canRecoverForeground
    );
  }

  private start(record: TaskNavigationReadRecord) {
    const controller = new AbortController();
    record.controller = controller;
    const cycle = record.snapshot.cycle + 1;
    const run = async () => {
      for (let attempt = 0; attempt <= RETRY_DELAYS_MS.length; attempt++) {
        this.publish(record, {
          phase: attempt === 0 ? "loading" : "retrying",
          attempt,
          temporary: attempt > 0,
          cycle,
        });
        try {
          const identity = await loadNavigationAttempt(
            record.taskId,
            controller.signal,
            this.loadIdentity,
          );
          this.assertCurrent(record, controller.signal);
          this.publish(record, { phase: "succeeded", attempt, identity, temporary: false, cycle });
          return identity;
        } catch (error) {
          if (controller.signal.aborted || isTaskNavigationAbort(error)) throw abortError();
          this.assertCurrent(record, controller.signal);
          const temporary = isTemporaryTaskNavigationError(error);
          if (!temporary || attempt === RETRY_DELAYS_MS.length) {
            this.publish(record, { phase: "failed", attempt, error, temporary, cycle });
            throw error;
          }
          const retryAfterMs =
            error instanceof ApiError ? (error.retryAfterSeconds ?? 0) * 1_000 : 0;
          this.publish(record, {
            phase: "retrying",
            attempt: attempt + 1,
            error,
            temporary,
            cycle,
          });
          await waitForVisibleRetry(
            Math.max(RETRY_DELAYS_MS[attempt], retryAfterMs),
            controller.signal,
          );
        }
      }
      throw abortError();
    };
    const promise = run().finally(() => {
      if (record.promise === promise) record.promise = undefined;
      if (record.controller === controller) record.controller = undefined;
    });
    record.promise = promise;
    return promise;
  }

  private assertCurrent(record: TaskNavigationReadRecord, signal: AbortSignal) {
    if (signal.aborted || !this.scopeIsCurrent() || record.generation !== this.generation) {
      throw abortError();
    }
  }

  private publish(
    record: TaskNavigationReadRecord,
    update: Omit<TaskNavigationReadSnapshot, "revision">,
  ) {
    if (this.records.get(readKey(record.generation, record.taskId)) !== record) return;
    record.snapshot = { ...update, revision: record.snapshot.revision + 1 };
    this.listeners
      .get(readKey(record.generation, record.taskId))
      ?.forEach((listener) => listener());
  }

  private cancel(key: string) {
    this.records.get(key)?.controller?.abort();
  }

  private cancelAll() {
    for (const record of this.records.values()) record.controller?.abort();
  }
}

export function createTaskNavigationReads(
  loadIdentity: LoadTaskNavigationIdentity,
  scopeIsCurrent?: () => boolean,
): TaskNavigationReads {
  return new TaskNavigationReads(loadIdentity, scopeIsCurrent);
}

function fetchTaskNavigationIdentity(
  taskId: string,
  signal: AbortSignal,
): Promise<TaskNavigationIdentity> {
  return Promise.all([
    fetchTask(taskId, { cache: "no-store", init: { signal } }),
    listTaskSessions(taskId, { cache: "no-store", init: { signal } }).then(
      (allSessionsResponse) => ({ allSessionsResponse, sessionListUnavailable: false }),
      (error) => {
        if (isTaskNavigationAbort(error) || signal.aborted) throw abortError();
        return { allSessionsResponse: { sessions: [], total: 0 }, sessionListUnavailable: true };
      },
    ),
  ]).then(([task, sessionList]) => ({ task, ...sessionList }));
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
    owner?.reads.dispose();
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
  options: {
    context?: TaskNavigationContext;
    generation?: number;
    refresh?: boolean;
    manualRetry?: boolean;
    supersede?: boolean;
    foregroundRecoveryEpisode?: object;
  } = {},
): Promise<TaskNavigationIdentity> {
  const owner = getTaskNavigationOwner(store);
  if (
    options.context &&
    (owner.token !== options.context.ownerToken || owner.identity !== options.context.identity)
  ) {
    return Promise.reject(abortError());
  }
  return owner.reads.read(taskId, options.context?.generation ?? options.generation, {
    refresh: options.refresh,
    manualRetry: options.manualRetry,
    supersede: options.supersede,
    foregroundRecoveryEpisode: options.foregroundRecoveryEpisode,
  });
}

export function useTaskNavigationReadState(
  store: StoreApi<AppState>,
  taskId: string | null,
  context?: TaskNavigationContext,
): TaskNavigationReadSnapshot {
  const owner = getTaskNavigationOwner(store);
  const generation = context?.generation ?? owner.reads.currentGeneration();
  return useSyncExternalStore(
    (listener) => (taskId ? owner.reads.subscribe(taskId, generation, listener) : () => {}),
    () => (taskId ? owner.reads.getSnapshot(taskId, generation) : IDLE_SNAPSHOT),
    () => IDLE_SNAPSHOT,
  );
}

export function retainTaskNavigationRead(
  store: StoreApi<AppState>,
  taskId: string,
  context: TaskNavigationContext,
): () => void {
  const owner = getTaskNavigationOwner(store);
  if (owner.token !== context.ownerToken || owner.identity !== context.identity) return () => {};
  return owner.reads.retain(taskId, context.generation);
}
