import { useEffect, useCallback, useState, useRef } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import {
  createTaskPlan,
  updateTaskPlan,
  deleteTaskPlan,
  getPlanRevision,
  revertPlanRevision,
} from "@/lib/api/domains/plan-api";
import {
  taskPlanQueryOptions,
  taskPlanRevisionsQueryOptions,
} from "@/lib/query/query-options/session";
import { qk } from "@/lib/query/keys";
import type { TaskPlanData } from "@/lib/query/query-options/session";
import type { TaskPlan, TaskPlanRevision } from "@/lib/types/http";

const EMPTY_REVISIONS: readonly TaskPlanRevision[] = Object.freeze([]);

/**
 * Hook to fetch and manage the plan for a task.
 * Plans are task-scoped (one plan per task, shared across all sessions).
 * @param taskId - The task ID to fetch the plan for
 * @param options.visible - When true, refetches the plan (use for tab visibility)
 */
export function useTaskPlan(taskId: string | null, options?: { visible?: boolean }) {
  const { visible = true } = options ?? {};
  const prevVisibleRef = useRef(visible);
  const queryClient = useQueryClient();

  // Server state (plan + loading) now comes from TanStack Query; the bridge
  // keeps it live from WS events. Disable the query when there is no task.
  const planQuery = useQuery({ ...taskPlanQueryOptions(taskId ?? ""), enabled: !!taskId });
  const plan = planQuery.data?.plan ?? null;
  const isLoading = !!taskId && planQuery.isLoading;
  const isLoaded = !!taskId && planQuery.isSuccess;

  const isSaving = useAppStore((state) =>
    taskId ? (state.taskPlans.savingByTaskId[taskId] ?? false) : false,
  );
  const setTaskPlanSaving = useAppStore((state) => state.setTaskPlanSaving);

  const [error, setError] = useState<string | null>(null);

  // NOTE: this hook intentionally does NOT mark the plan "seen" on load.
  //
  // Seen-state (the Plan tab's unseen-update dot) is owned exclusively by the
  // tab UIs that know whether the user is actually looking at the plan:
  //   - desktop:  `PlanTab` marks seen on dockview tab activation
  //               (`api.onDidActiveChange` / `api.isActive`).
  //   - reload:   `usePlanPanelAutoOpen` marks an already-restored panel's plan
  //               seen only when there is no recorded last-seen value.
  //   - mobile:   the plan-panel badge persists last-seen to localStorage.
  //
  // `useTaskPlan` mounts inside the Plan panel, which dockview keeps mounted
  // even while its tab is inactive — and it only mounts *after* the agent's
  // plan is already in the cache (auto-open is what reveals the panel). So a
  // seen-mark here can never distinguish "already acknowledged" from "fresh
  // unseen agent write"; it would mark every agent write seen the instant the
  // panel mounts, suppressing the indicator entirely.

  const refetch = useCallback(async () => {
    if (!taskId) return;
    setError(null);
    try {
      await planQuery.refetch();
    } catch (err) {
      console.error("Failed to fetch task plan:", err);
      setError(err instanceof Error ? err.message : "Failed to fetch plan");
    }
  }, [taskId, planQuery]);

  // Refetch when becoming visible (e.g., tab switch)
  useEffect(() => {
    const wasHidden = !prevVisibleRef.current;
    const isNowVisible = visible;
    prevVisibleRef.current = visible;

    // Only refetch when transitioning from hidden to visible
    if (wasHidden && isNowVisible && taskId) {
      void refetch();
    }
  }, [visible, taskId, refetch]);

  const writePlanToCache = useCallback(
    (savedPlan: TaskPlan | null) => {
      if (!taskId) return;
      queryClient.setQueryData<TaskPlanData>(qk.taskSession.plans(taskId), (prev) => ({
        plan: savedPlan,
        lastSeenUpdatedAt: prev?.lastSeenUpdatedAt ?? null,
      }));
    },
    [taskId, queryClient],
  );

  const savePlan = useCallback(
    async (content: string, title?: string): Promise<TaskPlan | null> => {
      if (!taskId) return null;

      setTaskPlanSaving(taskId, true);
      setError(null);
      try {
        const savedPlan: TaskPlan = plan
          ? await updateTaskPlan(taskId, content, title)
          : await createTaskPlan(taskId, content, title);
        writePlanToCache(savedPlan);
        return savedPlan;
      } catch (err) {
        console.error("Failed to save task plan:", err);
        setError(err instanceof Error ? err.message : "Failed to save plan");
        return null;
      } finally {
        setTaskPlanSaving(taskId, false);
      }
    },
    [taskId, plan, writePlanToCache, setTaskPlanSaving],
  );

  const removePlan = useCallback(async (): Promise<boolean> => {
    if (!taskId) return false;

    setTaskPlanSaving(taskId, true);
    setError(null);
    try {
      await deleteTaskPlan(taskId);
      writePlanToCache(null);
      return true;
    } catch (err) {
      console.error("Failed to delete task plan:", err);
      setError(err instanceof Error ? err.message : "Failed to delete plan");
      return false;
    } finally {
      setTaskPlanSaving(taskId, false);
    }
  }, [taskId, writePlanToCache, setTaskPlanSaving]);

  const revisionsBundle = useTaskPlanRevisions(taskId, setTaskPlanSaving, setError);

  return {
    plan,
    isLoading,
    isLoaded,
    isSaving,
    error,
    savePlan,
    deletePlan: removePlan,
    refetch,
    ...revisionsBundle,
  };
}

const EMPTY_PAIR: readonly [string | null, string | null] = Object.freeze([
  null,
  null,
]) as readonly [string | null, string | null];

function useTaskPlanRevisions(
  taskId: string | null,
  setTaskPlanSaving: (taskId: string, saving: boolean) => void,
  setError: (err: string | null) => void,
) {
  // Revisions list (metadata-only) now comes from TanStack Query; the bridge
  // upserts new revisions from WS events. The query backfills the full list on
  // mount / cache miss, replacing the old "load once when connected" effect.
  const revisionsQuery = useQuery({
    ...taskPlanRevisionsQueryOptions(taskId ?? ""),
    enabled: !!taskId,
  });
  const revisions = (revisionsQuery.data?.revisions ?? EMPTY_REVISIONS) as TaskPlanRevision[];
  const isLoadingRevisions = !!taskId && revisionsQuery.isLoading;
  const storeApi = useAppStoreApi();
  const cachePlanRevisionContent = useAppStore((state) => state.cachePlanRevisionContent);

  const loadRevisions = useCallback(async () => {
    if (!taskId) return;
    setError(null);
    try {
      await revisionsQuery.refetch();
    } catch (err) {
      console.error("Failed to load plan revisions:", err);
      setError(err instanceof Error ? err.message : "Failed to load revisions");
    }
  }, [taskId, revisionsQuery, setError]);

  const loadRevisionContent = useCallback(
    async (revisionId: string): Promise<string> => {
      // Read the cache lazily via the store API inside the callback so this
      // function's identity stays stable across cache updates. Selecting the
      // cache object as a hook input would re-create the callback whenever
      // any task's content was cached, which retriggers the dialogs'
      // content-fetch effects (cache short-circuits, but the work is wasted).
      const cached = storeApi.getState().taskPlans.revisionContentCache[revisionId];
      if (cached !== undefined) return cached;
      // Pass taskId so the backend can enforce revision-belongs-to-task.
      const rev = await getPlanRevision(revisionId, taskId ?? undefined);
      const content = rev.content ?? "";
      cachePlanRevisionContent(revisionId, content);
      return content;
    },
    [taskId, storeApi, cachePlanRevisionContent],
  );

  const revertTo = useCallback(
    async (revisionId: string, authorName?: string): Promise<TaskPlanRevision | null> => {
      if (!taskId) return null;
      setTaskPlanSaving(taskId, true);
      setError(null);
      try {
        return await revertPlanRevision(taskId, revisionId, authorName);
      } catch (err) {
        console.error("Failed to revert plan:", err);
        setError(err instanceof Error ? err.message : "Failed to revert plan");
        return null;
      } finally {
        setTaskPlanSaving(taskId, false);
      }
    },
    [taskId, setTaskPlanSaving, setError],
  );

  return {
    revisions,
    isLoadingRevisions,
    loadRevisions,
    loadRevisionContent,
    revertTo,
    ...usePreviewCompareState(taskId),
  };
}

/** Phase 6: preview + compare selectors and actions, scoped to the active task. */
function usePreviewCompareState(taskId: string | null) {
  const previewRevisionId = useAppStore((state) =>
    taskId ? (state.taskPlans.previewRevisionIdByTaskId[taskId] ?? null) : null,
  );
  const comparePair = useAppStore((state) =>
    taskId ? (state.taskPlans.comparePairByTaskId[taskId] ?? EMPTY_PAIR) : EMPTY_PAIR,
  ) as [string | null, string | null];
  const setPreviewRevisionStore = useAppStore((state) => state.setPreviewRevision);
  const toggleComparePairStore = useAppStore((state) => state.toggleComparePair);
  const clearComparePairStore = useAppStore((state) => state.clearComparePair);

  const setPreviewRevision = useCallback(
    (revisionId: string | null) => {
      if (!taskId) return;
      setPreviewRevisionStore(taskId, revisionId);
    },
    [taskId, setPreviewRevisionStore],
  );
  const toggleCompareSelection = useCallback(
    (revisionId: string) => {
      if (!taskId) return;
      toggleComparePairStore(taskId, revisionId);
    },
    [taskId, toggleComparePairStore],
  );
  const clearComparePair = useCallback(() => {
    if (!taskId) return;
    clearComparePairStore(taskId);
  }, [taskId, clearComparePairStore]);

  return {
    previewRevisionId,
    setPreviewRevision,
    comparePair,
    toggleCompareSelection,
    clearComparePair,
  };
}
