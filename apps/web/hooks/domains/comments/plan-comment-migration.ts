import type { StoreApi } from "zustand";
import { createTaskPlanComment, updateTaskPlanComment } from "@/lib/api/domains/plan-comment-api";
import { getTaskPlan } from "@/lib/api/domains/plan-api";
import { planCommentAdmissionConflict } from "@/lib/plan-comment-refs";
import { planCommentRecoveryDelay } from "@/lib/plan-comment-recovery";
import { useCommentsStore, type PlanComment } from "@/lib/state/slices/comments";
import {
  readLegacyPlanComments,
  removeAcknowledgedLegacyPlanComment,
  type LegacyPlanCommentRecord,
} from "@/lib/state/slices/comments/persistence";
import type { PlanCommentMigrationState } from "@/lib/state/slices/session/types";
import type { AppState } from "@/lib/state/store";
import type { TaskPlanComment, TaskPlanCommentSnapshot } from "@/lib/types/http";
import { WebSocketRequestError } from "@/lib/ws/request-error";

type Failure = PlanCommentMigrationState["failure"];
type PendingRecord = { record: LegacyPlanCommentRecord; acknowledged?: TaskPlanComment };
type Discovery = { sessionIds: string[]; complete: boolean; loading: boolean };

const recoveries = new WeakMap<StoreApi<AppState>, Map<string, PlanCommentMigration>>();

function legacyAnchor(comment: PlanComment) {
  const anchorFrom = comment.from ?? 0;
  return {
    selectedText: comment.selectedText,
    anchorFrom,
    anchorTo: comment.to ?? anchorFrom + Math.max(1, comment.selectedText.length),
  };
}

function sameAnchor(persisted: TaskPlanComment, comment: PlanComment) {
  const anchor = legacyAnchor(comment);
  return (
    persisted.selected_text === anchor.selectedText &&
    persisted.anchor_from === anchor.anchorFrom &&
    persisted.anchor_to === anchor.anchorTo
  );
}

function acknowledgeLegacyRecord({ sessionId, comment }: LegacyPlanCommentRecord): Failure {
  if (!removeAcknowledgedLegacyPlanComment(sessionId, comment)) return "transient";
  useCommentsStore.getState().forgetMigratedPlanComment(sessionId, comment.id);
  return null;
}

function classifyFailure(error: unknown): Failure {
  if (planCommentAdmissionConflict(error)) return "conflict";
  if (
    error instanceof WebSocketRequestError &&
    error.code &&
    !["internal_error", "internal", "timeout", "not_found"].includes(error.code)
  )
    return "rejected";
  return "transient";
}

/** One retry owner per task, independent of the number of mounted chat/plan surfaces. */
export class PlanCommentMigration {
  private pending = new Map<string, PendingRecord>();
  private consumers = new Map<symbol, () => Promise<void>>();
  private discovery: Discovery = { sessionIds: [], complete: false, loading: false };
  private storageAvailable = true;
  private failures = 0;
  private failure: Failure = null;
  private dueAt = 0;
  private lastWakeAt = -Infinity;
  private generation = 0;
  private refreshPlan = false;
  private inFlight: Promise<void> | undefined;
  private timer: ReturnType<typeof setTimeout> | undefined;
  private unsubscribe: (() => void) | undefined;

  constructor(
    private store: StoreApi<AppState>,
    private taskId: string,
  ) {}

  attach(discover: () => Promise<void>) {
    const consumer = Symbol();
    this.consumers.set(consumer, discover);
    if (!this.unsubscribe) this.observe();
    return () => {
      this.consumers.delete(consumer);
      if (this.consumers.size) return;
      this.generation++;
      this.cancelTimer();
      this.unsubscribe?.();
      this.unsubscribe = undefined;
    };
  }

  update(discovery: Discovery) {
    this.discovery = discovery;
    this.scan();
    this.kick();
  }

  wake() {
    if (!this.ready()) return this.inFlight;
    if (Date.now() - this.lastWakeAt < 250) return this.inFlight;
    this.lastWakeAt = Date.now();
    this.dueAt = 0;
    this.cancelTimer();
    this.scan();
    return this.kick();
  }

  retry() {
    this.failures = 0;
    this.failure = null;
    this.dueAt = 0;
    this.refreshPlan = this.pending.size > 0 && !this.plan();
    this.cancelTimer();
    this.scan();
    return this.kick();
  }

  private plan() {
    return this.store.getState().taskPlans.byTaskId[this.taskId];
  }

  private ready() {
    return (
      this.consumers.size > 0 &&
      this.store.getState().connection.status === "connected" &&
      document.visibilityState === "visible"
    );
  }

  private observe() {
    const unsubscribe = this.store.subscribe((state, previous) => {
      const planChanged =
        state.taskPlans.byTaskId[this.taskId]?.id !==
          previous.taskPlans.byTaskId[this.taskId]?.id ||
        state.taskPlans.loadedByTaskId[this.taskId] !==
          previous.taskPlans.loadedByTaskId[this.taskId];
      const connectionChanged = state.connection.status !== previous.connection.status;
      if (!planChanged && !connectionChanged) return;
      if (planChanged) {
        this.generation++;
        this.failure = null;
        this.refreshPlan = false;
      }
      this.dueAt = 0;
      this.cancelTimer();
      // Finish the store update before publishing derived recovery state.
      void Promise.resolve().then(() => this.kick());
    });
    const onHidden = () => {
      if (document.visibilityState !== "visible") this.cancelTimer();
    };
    document.addEventListener("visibilitychange", onHidden);
    this.unsubscribe = () => {
      unsubscribe();
      document.removeEventListener("visibilitychange", onHidden);
    };
  }

  private scan() {
    const { records, available } = readLegacyPlanComments(this.discovery.sessionIds);
    this.storageAvailable = available;
    for (const record of records) {
      const key = `${record.sessionId}:${record.comment.id}`;
      const known = this.pending.get(key);
      if (known) known.record = record;
      else this.pending.set(key, { record });
    }
  }

  private publish(status: PlanCommentMigrationState["status"]) {
    const next = { status, pendingCount: this.pending.size, failure: this.failure };
    const current = this.store.getState().taskPlans.commentsMigrationByTaskId[this.taskId];
    if (
      current?.status === next.status &&
      current.pendingCount === next.pendingCount &&
      current.failure === next.failure
    )
      return;
    this.store.getState().setTaskPlanCommentMigrationState(this.taskId, next);
  }

  private cancelTimer() {
    clearTimeout(this.timer);
    this.timer = undefined;
  }

  private schedule() {
    if (this.timer || !this.ready()) return;
    this.timer = setTimeout(
      () => {
        this.timer = undefined;
        void this.kick();
      },
      Math.max(0, this.dueAt - Date.now()),
    );
  }

  private kick(): Promise<void> | undefined {
    if (!this.consumers.size) return this.inFlight;
    this.scan();
    if (this.inFlight) {
      const state = this.store.getState().taskPlans.commentsMigrationByTaskId[this.taskId];
      this.publish(state?.status ?? "idle");
      return this.inFlight;
    }
    if (this.failure === "conflict" || this.failure === "rejected") {
      this.publish("failed");
      return;
    }
    if (!this.pending.size && this.discovery.complete && this.storageAvailable) {
      this.failure = null;
      this.failures = 0;
      this.publish("complete");
      return;
    }
    if (this.pending.size && this.plan() === null && !this.refreshPlan) {
      this.publish("waiting_for_plan");
      return;
    }
    if (!this.ready()) {
      this.publish("idle");
      return;
    }
    if (this.dueAt > Date.now()) {
      this.schedule();
      return;
    }
    const generation = this.generation;
    this.inFlight = Promise.resolve()
      .then(() => this.run(generation))
      .finally(() => {
        this.inFlight = undefined;
        if (generation !== this.generation) void this.kick();
        else if (this.failure === "transient") this.schedule();
        else if (!this.failure && this.pending.size && this.plan() && this.ready())
          void this.kick();
      });
    return this.inFlight;
  }

  private current(generation: number) {
    return this.consumers.size > 0 && generation === this.generation;
  }

  private async discover(generation: number) {
    if (this.refreshPlan || (this.pending.size > 0 && this.plan() === undefined)) {
      const next = await getTaskPlan(this.taskId);
      if (!this.current(generation)) return;
      this.refreshPlan = false;
      this.store.getState().setTaskPlan(this.taskId, next);
    }
    if (!this.discovery.complete && !this.discovery.loading) {
      await this.consumers.values().next().value?.();
    }
  }

  private async run(generation: number) {
    if (!this.current(generation) || !this.ready()) return;
    this.publish(this.pending.size ? "running" : "idle");
    let failure: Failure = null;
    try {
      await this.discover(generation);
    } catch {
      failure = "transient";
    }
    if (!this.current(generation)) return;
    failure = (await this.uploadPending(generation)) ?? failure;
    if (!this.current(generation)) return;
    this.scan();
    if (!this.discovery.complete || !this.storageAvailable) failure ??= "transient";
    this.finishRun(failure);
  }

  private async uploadPending(generation: number): Promise<Failure> {
    const plan = this.plan();
    if (!plan) return null;
    let failure: Failure = null;
    for (const [key, pending] of this.pending) {
      if (!this.current(generation) || !this.ready()) return failure;
      const outcome = await this.upload(pending, plan.id, generation);
      if (!this.current(generation)) return failure;
      if (!outcome) this.pending.delete(key);
      else if (failure !== "conflict" && failure !== "rejected") failure = outcome;
    }
    return failure;
  }

  private finishRun(failure: Failure) {
    this.failure = failure;
    if (failure) {
      this.failures++;
      this.dueAt = Date.now() + planCommentRecoveryDelay(this.failures);
      this.publish(failure !== "transient" || this.failures >= 3 ? "failed" : "retrying");
    } else if (this.pending.size) {
      this.publish(this.plan() === null ? "waiting_for_plan" : "idle");
    } else {
      this.failures = 0;
      this.publish("complete");
    }
  }

  private async upload(
    pending: PendingRecord,
    planId: string,
    generation: number,
  ): Promise<Failure> {
    const record = pending.record;
    const { comment } = record;
    try {
      const acknowledged =
        pending.acknowledged?.plan_id === planId ? pending.acknowledged : undefined;
      if (acknowledged && !sameAnchor(acknowledged, comment)) return "conflict";
      if (!acknowledged || acknowledged.body !== comment.text) {
        const input = {
          taskId: this.taskId,
          planId,
          id: comment.id,
          body: comment.text,
        };
        const snapshot = acknowledged
          ? await updateTaskPlanComment({ ...input, expectedVersion: acknowledged.version })
          : await createTaskPlanComment({ ...input, ...legacyAnchor(comment) });
        if (!this.current(generation)) return "transient";
        this.store.getState().setTaskPlanComments(this.taskId, snapshot);
        if (!this.recordAcknowledgement(pending, planId, comment, snapshot)) return "conflict";
      }
      if (!this.current(generation)) return "transient";
      return acknowledgeLegacyRecord(record);
    } catch (error) {
      if (!this.current(generation)) return "transient";
      return this.handleUploadFailure(error, pending, planId, record);
    }
  }

  private handleUploadFailure(
    error: unknown,
    pending: PendingRecord,
    planId: string,
    record: LegacyPlanCommentRecord,
  ): Failure {
    const snapshot = planCommentAdmissionConflict(error)?.snapshot;
    if (snapshot) this.store.getState().setTaskPlanComments(this.taskId, snapshot);
    if (
      snapshot &&
      pending.acknowledged?.plan_id === planId &&
      this.recordAcknowledgement(pending, planId, record.comment, snapshot)
    ) {
      return acknowledgeLegacyRecord(record);
    }
    if (error instanceof WebSocketRequestError && error.code === "not_found")
      this.refreshPlan = true;
    return classifyFailure(error);
  }

  private recordAcknowledgement(
    pending: PendingRecord,
    planId: string,
    comment: PlanComment,
    snapshot: TaskPlanCommentSnapshot,
  ) {
    if (snapshot.task_id !== this.taskId || snapshot.plan_id !== planId) return false;
    const persisted = snapshot.comments.find((row) => row.id === comment.id);
    if (!persisted || persisted.body !== comment.text || !sameAnchor(persisted, comment))
      return false;
    pending.acknowledged = persisted;
    return true;
  }
}

export function planCommentMigrationFor(store: StoreApi<AppState>, taskId: string) {
  let tasks = recoveries.get(store);
  if (!tasks) {
    tasks = new Map();
    recoveries.set(store, tasks);
  }
  let recovery = tasks.get(taskId);
  if (!recovery) {
    recovery = new PlanCommentMigration(store, taskId);
    tasks.set(taskId, recovery);
  }
  return recovery;
}
