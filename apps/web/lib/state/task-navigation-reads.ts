import { useSyncExternalStore } from "react";
import type { StoreApi } from "zustand";
import { ApiError } from "@/lib/api/client";
import { fetchTask, listTaskSessions } from "@/lib/api";
import { getBackendConfig } from "@/lib/config";
import { toKanbanTask } from "@/lib/kanban/map-task";
import {
  TaskNavigationCycleDeadlineError,
  TaskNavigationRecoveryClock,
} from "@/lib/state/task-navigation-recovery-clock";
import {
  abortError,
  loadNavigationAttempt,
  raceAbort,
  type LoadTaskNavigationIdentity,
} from "@/lib/state/task-navigation-attempt";
import { getTaskSessionReads, type TaskSessionReads } from "@/lib/state/task-session-reads";
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

type TaskNavigationReadRecord = {
  generation: number;
  taskId: string;
  snapshot: TaskNavigationReadSnapshot;
  promise?: Promise<TaskNavigationIdentity>;
  controller?: AbortController;
  lastForegroundRecoveryEpisode?: object;
};

const MAX_RETAINED_READS = 8;
const RETRY_DELAYS_MS = [2_000, 5_000, 10_000, 10_000] as const;
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
  if (error instanceof TaskNavigationCycleDeadlineError) return true;
  if (!(error instanceof ApiError)) return false;
  if ([429, 502, 503, 504].includes(error.status)) return true;
  return (
    error.body !== null &&
    typeof error.body === "object" &&
    "code" in error.body &&
    (error.body as { code?: unknown }).code === "persistence_unavailable"
  );
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
    let cycleExpired = false;
    let latestTemporaryError: unknown = record.snapshot.temporary
      ? record.snapshot.error
      : undefined;
    const clock = new TaskNavigationRecoveryClock({
      onDeadline: () => {
        cycleExpired = true;
        controller.abort();
      },
    });
    const run = async () => {
      clock.start();
      for (let attempt = 0; attempt <= RETRY_DELAYS_MS.length; attempt++) {
        const retainedError = latestTemporaryError;
        this.publish(record, {
          phase: attempt === 0 ? "loading" : "retrying",
          attempt,
          temporary: retainedError !== undefined,
          ...(retainedError !== undefined ? { error: retainedError } : {}),
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
          const outcome = this.classifyAttemptFailure({
            record,
            controller,
            error,
            attempt,
            cycle,
            cycleExpired,
          });
          if (outcome.kind === "fatal") throw outcome.error;
          latestTemporaryError = outcome.error;
          this.publish(record, {
            phase: "retrying",
            attempt: attempt + 1,
            error: outcome.error,
            temporary: true,
            cycle,
          });
          try {
            await clock.wait(outcome.delayMs, controller.signal);
          } catch (waitError) {
            throw this.classifyWaitFailure(record, waitError, attempt, cycle, cycleExpired);
          }
        }
      }
      throw new TaskNavigationCycleDeadlineError();
    };
    const promise = run().finally(() => {
      clock.dispose();
      if (record.promise === promise) record.promise = undefined;
      if (record.controller === controller) record.controller = undefined;
    });
    record.promise = promise;
    return promise;
  }

  private classifyAttemptFailure(args: {
    record: TaskNavigationReadRecord;
    controller: AbortController;
    error: unknown;
    attempt: number;
    cycle: number;
    cycleExpired: boolean;
  }): { kind: "fatal"; error: unknown } | { kind: "retry"; error: unknown; delayMs: number } {
    const { record, controller, error, attempt, cycle, cycleExpired } = args;
    if (cycleExpired) {
      return { kind: "fatal", error: this.publishCycleDeadline(record, attempt, cycle) };
    }
    if (controller.signal.aborted || isTaskNavigationAbort(error)) {
      return { kind: "fatal", error: abortError() };
    }
    this.assertCurrent(record, controller.signal);
    const temporary = isTemporaryTaskNavigationError(error);
    if (!temporary || attempt === RETRY_DELAYS_MS.length) {
      this.publish(record, { phase: "failed", attempt, error, temporary, cycle });
      return { kind: "fatal", error };
    }
    const retryAfterMs = error instanceof ApiError ? (error.retryAfterSeconds ?? 0) * 1_000 : 0;
    return {
      kind: "retry",
      error,
      delayMs: Math.max(RETRY_DELAYS_MS[attempt], retryAfterMs),
    };
  }

  private classifyWaitFailure(
    record: TaskNavigationReadRecord,
    waitError: unknown,
    attempt: number,
    cycle: number,
    cycleExpired: boolean,
  ): unknown {
    if (cycleExpired || waitError instanceof TaskNavigationCycleDeadlineError) {
      return this.publishCycleDeadline(record, attempt, cycle);
    }
    return waitError;
  }

  private publishCycleDeadline(
    record: TaskNavigationReadRecord,
    attempt: number,
    cycle: number,
  ): TaskNavigationCycleDeadlineError {
    const deadlineError = new TaskNavigationCycleDeadlineError();
    this.publish(record, {
      phase: "failed",
      attempt,
      error: deadlineError,
      temporary: true,
      cycle,
    });
    return deadlineError;
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

async function fetchTaskNavigationIdentity(
  store: StoreApi<AppState>,
  taskId: string,
  signal: AbortSignal,
  sessionReads: TaskSessionReads,
  scopeIsCurrent: () => boolean,
): Promise<TaskNavigationIdentity> {
  // The HTTP attempt owns reconciliation; joined and cached callers do not publish again.
  const overviewReadId = store.getState().beginTaskOverviewRead();
  try {
    const taskRequest = fetchTask(taskId, { cache: "no-store", init: { signal } });
    const release = sessionReads.retain(taskId);
    const sessionList = raceAbort(
      sessionReads.read(taskId, (readSignal) =>
        listTaskSessions(taskId, { cache: "no-store", init: { signal: readSignal } }),
      ),
      signal,
    ).finally(release);
    const [task, sessionResponse] = await Promise.all([
      taskRequest,
      sessionList.then(
        (allSessionsResponse) => ({ allSessionsResponse, sessionListUnavailable: false }),
        (error) => {
          if (isTaskNavigationAbort(error) || signal.aborted) throw abortError();
          return { allSessionsResponse: { sessions: [], total: 0 }, sessionListUnavailable: true };
        },
      ),
    ]);
    if (signal.aborted || !scopeIsCurrent()) throw abortError();
    if (isCurrentTaskDetailRead(store, taskId)) {
      store.getState().retainTaskOverviews("detail", [toKanbanTask(task)], overviewReadId);
    }
    return { task, ...sessionResponse };
  } finally {
    store.getState().finishTaskOverviewRead(overviewReadId);
  }
}

function isCurrentTaskDetailRead(store: StoreApi<AppState>, taskId: string): boolean {
  return (
    store.getState().tasks.activeTaskId === taskId ||
    owners.get(store)?.navigationContext?.taskId === taskId
  );
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
    taskId?: string;
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
    const sessionReads = getTaskSessionReads(store);
    const scopeIsCurrent = () =>
      owners.get(store) === nextOwner && storeIdentity(store.getState()) === identity;
    nextOwner.reads = new TaskNavigationReads(
      (taskId, signal) =>
        fetchTaskNavigationIdentity(store, taskId, signal, sessionReads, scopeIsCurrent),
      scopeIsCurrent,
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
  // Route identity can be admitted before its shell activates the task.
  if (options.context && owner.navigationContext?.context === options.context) {
    owner.navigationContext.taskId = taskId;
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
