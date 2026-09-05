import { useCallback, useEffect, useMemo } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useTaskSessions } from "@/hooks/use-task-sessions";
import { createTaskPlanComment, getTaskPlanComments } from "@/lib/api/domains/plan-comment-api";
import { useCommentsStore } from "@/lib/state/slices/comments";
import {
  listLegacyPlanComments,
  removeAcknowledgedLegacyPlanComment,
} from "@/lib/state/slices/comments/persistence";
import type { AppState } from "@/lib/state/store";
import type { TaskPlan } from "@/lib/types/http";
import { planCommentAdmissionConflict } from "@/lib/plan-comment-refs";

type AppStore = ReturnType<typeof useAppStoreApi>;

const migrationRunsByStore = new WeakMap<AppStore, Map<string, Promise<void>>>();

function migrationRunsFor(store: AppStore) {
  const existing = migrationRunsByStore.get(store);
  if (existing) return existing;
  const runs = new Map<string, Promise<void>>();
  migrationRunsByStore.set(store, runs);
  return runs;
}

function planIdentityMatches(state: AppState, taskId: string, plan: TaskPlan | null) {
  return (state.taskPlans.byTaskId[taskId]?.id ?? null) === (plan?.id ?? null);
}

function setStatusIfCurrent(
  store: AppStore,
  taskId: string,
  plan: TaskPlan | null,
  status: "running" | "complete" | "waiting_for_plan" | "failed",
) {
  const state = store.getState();
  if (planIdentityMatches(state, taskId, plan)) {
    state.setTaskPlanCommentMigrationStatus(taskId, status);
  }
}

type LegacyPlanCommentRecord = ReturnType<typeof listLegacyPlanComments>[number];

function acknowledgeLegacyComment({ sessionId, comment }: LegacyPlanCommentRecord) {
  const removed = removeAcknowledgedLegacyPlanComment(sessionId, comment);
  const sameIdStillStored = listLegacyPlanComments([sessionId]).some(
    (record) => record.comment.id === comment.id,
  );
  if (!removed && sameIdStillStored) return false;
  useCommentsStore.getState().forgetMigratedPlanComment(sessionId, comment.id);
  return true;
}

async function migrateLegacyComment(
  taskId: string,
  plan: TaskPlan,
  record: LegacyPlanCommentRecord,
  store: AppStore,
) {
  const { comment } = record;
  try {
    const anchorFrom = comment.from ?? 0;
    const snapshot = await createTaskPlanComment({
      taskId,
      planId: plan.id,
      id: comment.id,
      body: comment.text,
      selectedText: comment.selectedText,
      anchorFrom,
      anchorTo: comment.to ?? anchorFrom + Math.max(1, comment.selectedText.length),
    });
    store.getState().setTaskPlanComments(taskId, snapshot);
    return acknowledgeLegacyComment(record);
  } catch (error) {
    const snapshot = planCommentAdmissionConflict(error)?.snapshot;
    if (snapshot) store.getState().setTaskPlanComments(taskId, snapshot);
    const authoritativeMatch = snapshot?.comments.some(
      (candidate) => candidate.id === comment.id && candidate.plan_id === plan.id,
    );
    if (authoritativeMatch && acknowledgeLegacyComment(record)) return true;
    // i18n-exempt: developer diagnostic; the migration notice is localized.
    console.error("Failed to migrate legacy plan comment:", error);
    return false;
  }
}

async function refreshMigratedComments(taskId: string, store: AppStore) {
  try {
    const snapshot = await getTaskPlanComments(taskId);
    store.getState().setTaskPlanComments(taskId, snapshot);
    return true;
  } catch (error) {
    // i18n-exempt: developer diagnostic; the migration notice is localized.
    console.error("Failed to refresh task plan comments after migration:", error);
    return false;
  }
}

async function migrateLegacyComments(
  taskId: string,
  plan: TaskPlan | null,
  sessionIds: string[],
  store: AppStore,
) {
  const records = listLegacyPlanComments(sessionIds);
  if (!plan) {
    setStatusIfCurrent(store, taskId, plan, records.length === 0 ? "complete" : "waiting_for_plan");
    return;
  }

  setStatusIfCurrent(store, taskId, plan, "running");
  let failed = false;
  for (const record of records) {
    if (!planIdentityMatches(store.getState(), taskId, plan)) return;
    if (!(await migrateLegacyComment(taskId, plan, record, store))) failed = true;
  }
  if (!(await refreshMigratedComments(taskId, store))) failed = true;
  setStatusIfCurrent(store, taskId, plan, failed ? "failed" : "complete");
}

function startMigration(
  taskId: string,
  plan: TaskPlan | null,
  sessionIds: string[],
  store: AppStore,
) {
  const migrationRuns = migrationRunsFor(store);
  const runKey = `${taskId}:${plan?.id ?? "none"}`;
  const existing = migrationRuns.get(runKey);
  if (existing) return existing;
  const request = migrateLegacyComments(taskId, plan, sessionIds, store).finally(() => {
    if (migrationRuns.get(runKey) === request) migrationRuns.delete(runKey);
  });
  migrationRuns.set(runKey, request);
  return request;
}

/** Losslessly promotes sessionStorage plan drafts into the task-owned collection. */
export function usePlanCommentMigration(taskId: string | null | undefined) {
  const store = useAppStoreApi();
  const {
    sessions,
    isLoaded: sessionsLoaded,
    error: sessionsError,
    loadSessions,
  } = useTaskSessions(taskId ?? null);
  const plan = useAppStore((state) => (taskId ? state.taskPlans.byTaskId[taskId] : undefined));
  const planLoaded = useAppStore((state) =>
    taskId ? (state.taskPlans.loadedByTaskId[taskId] ?? false) : true,
  );
  const storedStatus = useAppStore((state) =>
    taskId ? state.taskPlans.commentsMigrationStatusByTaskId[taskId] : "complete",
  );
  const planLoadError = useAppStore((state) =>
    taskId ? (state.taskPlans.commentsErrorByTaskId[taskId] ?? null) : null,
  );
  const status = storedStatus ?? "idle";
  const sessionIds = useMemo(() => sessions.map((session) => session.id), [sessions]);

  useEffect(() => {
    if (
      taskId &&
      status !== "complete" &&
      status !== "running" &&
      (sessionsError || planLoadError)
    ) {
      store.getState().setTaskPlanCommentMigrationStatus(taskId, "failed");
      return;
    }
    if (!taskId || !sessionsLoaded || !planLoaded || plan === undefined) return;
    if (status === "running" || status === "complete" || status === "failed") return;
    if (status === "waiting_for_plan" && plan === null) return;
    void startMigration(taskId, plan, sessionIds, store);
  }, [
    plan,
    planLoadError,
    planLoaded,
    sessionIds,
    sessionsError,
    sessionsLoaded,
    status,
    store,
    taskId,
  ]);

  const retry = useCallback(async () => {
    if (!taskId) return;
    const state = store.getState();
    state.setTaskPlanCommentMigrationStatus(taskId, "running");
    state.setTaskPlanCommentsError(taskId);
    await loadSessions(true);
    const next = store.getState();
    next.setTaskPlanCommentMigrationStatus(
      taskId,
      next.taskSessionsByTask.errorByTaskId?.[taskId] ? "failed" : "idle",
    );
  }, [loadSessions, store, taskId]);

  return {
    status,
    isReady: status === "complete",
    isBlocking: Boolean(taskId) && status !== "complete",
    retry,
  };
}
