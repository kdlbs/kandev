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

const migrationRuns = new Map<string, Promise<void>>();

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
  for (const { sessionId, comment } of records) {
    if (!planIdentityMatches(store.getState(), taskId, plan)) return;
    try {
      const snapshot = await createTaskPlanComment({
        taskId,
        planId: plan.id,
        id: comment.id,
        body: comment.text,
        selectedText: comment.selectedText,
        anchorFrom: comment.from ?? 0,
        anchorTo: comment.to ?? Math.max(1, comment.selectedText.length),
      });
      store.getState().setTaskPlanComments(taskId, snapshot);
      const removed = removeAcknowledgedLegacyPlanComment(sessionId, comment);
      const sameIdStillStored = listLegacyPlanComments([sessionId]).some(
        (record) => record.comment.id === comment.id,
      );
      if (removed || !sameIdStillStored) {
        useCommentsStore.getState().forgetMigratedPlanComment(sessionId, comment.id);
      } else {
        failed = true;
      }
    } catch (error) {
      failed = true;
      const snapshot = planCommentAdmissionConflict(error)?.snapshot;
      if (snapshot) store.getState().setTaskPlanComments(taskId, snapshot);
      // i18n-exempt: developer diagnostic; the migration notice is localized.
      console.error("Failed to migrate legacy plan comment:", error);
    }
  }

  try {
    const snapshot = await getTaskPlanComments(taskId);
    store.getState().setTaskPlanComments(taskId, snapshot);
  } catch (error) {
    failed = true;
    // i18n-exempt: developer diagnostic; the migration notice is localized.
    console.error("Failed to refresh task plan comments after migration:", error);
  }
  setStatusIfCurrent(store, taskId, plan, failed ? "failed" : "complete");
}

function startMigration(
  taskId: string,
  plan: TaskPlan | null,
  sessionIds: string[],
  store: AppStore,
) {
  const existing = migrationRuns.get(taskId);
  if (existing) return existing;
  const request = migrateLegacyComments(taskId, plan, sessionIds, store).finally(() => {
    if (migrationRuns.get(taskId) === request) migrationRuns.delete(taskId);
  });
  migrationRuns.set(taskId, request);
  return request;
}

/** Losslessly promotes sessionStorage plan drafts into the task-owned collection. */
export function usePlanCommentMigration(taskId: string | null | undefined) {
  const store = useAppStoreApi();
  const { sessions, isLoaded: sessionsLoaded } = useTaskSessions(taskId ?? null);
  const plan = useAppStore((state) => (taskId ? state.taskPlans.byTaskId[taskId] : undefined));
  const planLoaded = useAppStore((state) =>
    taskId ? (state.taskPlans.loadedByTaskId[taskId] ?? false) : true,
  );
  const storedStatus = useAppStore((state) =>
    taskId ? state.taskPlans.commentsMigrationStatusByTaskId[taskId] : "complete",
  );
  const status = storedStatus ?? "idle";
  const sessionIds = useMemo(() => sessions.map((session) => session.id), [sessions]);

  useEffect(() => {
    if (!taskId || !sessionsLoaded || !planLoaded || plan === undefined) return;
    if (status === "running" || status === "complete" || status === "failed") return;
    if (status === "waiting_for_plan" && plan === null) return;
    void startMigration(taskId, plan, sessionIds, store);
  }, [plan, planLoaded, sessionIds, sessionsLoaded, status, store, taskId]);

  const retry = useCallback(() => {
    if (taskId) store.getState().setTaskPlanCommentMigrationStatus(taskId, "idle");
  }, [store, taskId]);

  return {
    status,
    isReady: status === "complete",
    isBlocking: Boolean(taskId) && status !== "complete",
    retry,
  };
}
