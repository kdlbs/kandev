import type { StoreApi } from "zustand";
import { getTaskPlan } from "@/lib/api/domains/plan-api";
import { getTaskPlanComments } from "@/lib/api/domains/plan-comment-api";
import { planCommentRecoveryDelay } from "@/lib/plan-comment-recovery";
import type { AppState } from "@/lib/state/store";

const loaders = new WeakMap<StoreApi<AppState>, Map<string, PlanCommentLoader>>();

/** Ordinary reads retain the last snapshot and never participate in Send admission. */
class PlanCommentLoader {
  private consumers = 0;
  private generation = 0;
  private planEpoch = 0;
  private failures = 0;
  private lastWakeAt = -Infinity;
  private timer: ReturnType<typeof setTimeout> | undefined;
  private inFlight: Promise<void> | undefined;
  private activeReadEpoch = 0;
  private needsReload = false;
  private unsubscribe: (() => void) | undefined;

  constructor(
    private store: StoreApi<AppState>,
    private taskId: string,
    private errorMessage: string,
  ) {}

  setErrorMessage(errorMessage: string) {
    this.errorMessage = errorMessage;
  }

  attach() {
    this.consumers++;
    if (!this.unsubscribe) this.observe();
    if (this.failures) void this.load(true);
    return () => {
      this.consumers--;
      if (this.consumers) return;
      this.generation++;
      this.pause();
      this.unsubscribe?.();
      this.unsubscribe = undefined;
    };
  }

  private pause() {
    clearTimeout(this.timer);
    this.timer = undefined;
  }

  private observe() {
    const unsubscribe = this.store.subscribe((state, previous) => {
      if (
        state.taskPlans.byTaskId[this.taskId]?.id !==
          previous.taskPlans.byTaskId[this.taskId]?.id ||
        state.taskPlans.loadedByTaskId[this.taskId] !==
          previous.taskPlans.loadedByTaskId[this.taskId]
      )
        this.planEpoch++;
      if (state.connection.status !== "connected") this.pause();
    });
    const onHidden = () => {
      if (document.visibilityState !== "visible") this.pause();
    };
    document.addEventListener("visibilitychange", onHidden);
    this.unsubscribe = () => {
      unsubscribe();
      document.removeEventListener("visibilitychange", onHidden);
    };
  }

  wake() {
    if (!this.ready()) return this.inFlight ?? Promise.resolve();
    if (Date.now() - this.lastWakeAt < 250) return this.inFlight ?? Promise.resolve();
    this.lastWakeAt = Date.now();
    return this.load(true);
  }

  private ready() {
    return (
      this.consumers > 0 &&
      this.store.getState().connection.status === "connected" &&
      document.visibilityState === "visible"
    );
  }

  load(force: boolean): Promise<void> {
    if (this.inFlight) {
      if (this.planEpoch !== this.activeReadEpoch) this.needsReload = true;
      return this.inFlight;
    }
    if (!this.ready()) return Promise.resolve();
    clearTimeout(this.timer);
    this.timer = undefined;
    const generation = this.generation;
    this.activeReadEpoch = this.planEpoch;
    this.needsReload = false;
    const promise = Promise.resolve()
      .then(() => this.read(force, generation))
      .finally(() => {
        if (this.inFlight === promise) this.inFlight = undefined;
        if (generation !== this.generation && this.ready()) void this.load(true);
        else if (this.needsReload && this.ready()) void this.load(false);
      });
    this.inFlight = promise;
    return promise;
  }

  private current(generation: number) {
    return this.consumers > 0 && generation === this.generation;
  }

  private async resolvePlan(force: boolean, generation: number) {
    const state = this.store.getState();
    const cached = state.taskPlans.byTaskId[this.taskId];
    if (!force && cached !== undefined) return cached;
    const epoch = this.planEpoch;
    state.setTaskPlanLoading(this.taskId, true);
    const next = await getTaskPlan(this.taskId);
    if (!this.current(generation)) return undefined;
    const current = this.store.getState();
    if (this.planEpoch !== epoch || current.taskPlans.byTaskId[this.taskId] !== cached)
      return current.taskPlans.byTaskId[this.taskId];
    current.setTaskPlan(this.taskId, next);
    this.activeReadEpoch = this.planEpoch;
    return next;
  }

  private async read(force: boolean, generation: number) {
    if (!this.current(generation) || !this.ready()) return;
    const state = this.store.getState();
    let expectedEpoch = this.planEpoch;
    state.setTaskPlanCommentsError(this.taskId);
    try {
      const plan = await this.resolvePlan(force, generation);
      if (!this.current(generation)) return;
      expectedEpoch = this.planEpoch;
      this.activeReadEpoch = expectedEpoch;
      this.needsReload = false;
      if (plan && this.ready()) {
        this.store.getState().setTaskPlanCommentsLoading(this.taskId, true);
        const snapshot = await getTaskPlanComments(this.taskId);
        if (!this.current(generation) || expectedEpoch !== this.planEpoch) return;
        this.store.getState().setTaskPlanComments(this.taskId, snapshot);
      }
      this.failures = 0;
    } catch {
      this.failedRead(generation, expectedEpoch);
    } finally {
      this.finishRead();
    }
  }

  private failedRead(generation: number, epoch: number) {
    if (!this.current(generation) || epoch !== this.planEpoch) return;
    this.store.getState().setTaskPlanCommentsError(this.taskId, this.errorMessage);
    this.failures++;
    if (!this.ready()) return;
    this.timer = setTimeout(() => {
      this.timer = undefined;
      void this.load(true);
    }, planCommentRecoveryDelay(this.failures));
  }

  private finishRead() {
    const current = this.store.getState();
    current.setTaskPlanLoading(this.taskId, false);
    current.setTaskPlanCommentsLoading(this.taskId, false);
  }
}

export function planCommentLoaderFor(
  store: StoreApi<AppState>,
  taskId: string,
  errorMessage: string,
) {
  let tasks = loaders.get(store);
  if (!tasks) {
    tasks = new Map();
    loaders.set(store, tasks);
  }
  let loader = tasks.get(taskId);
  if (!loader) {
    loader = new PlanCommentLoader(store, taskId, errorMessage);
    tasks.set(taskId, loader);
  } else loader.setErrorMessage(errorMessage);
  return loader;
}
