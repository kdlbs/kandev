import { useEffect, useCallback, useState, useRef } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import {
  getTaskPlan,
  createTaskPlan,
  updateTaskPlan,
  deleteTaskPlan,
  listPlanRevisions,
  getPlanRevision,
  revertPlanRevision,
} from "@/lib/api/domains/plan-api";
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
  const plan = useAppStore((state) => (taskId ? state.taskPlans.byTaskId[taskId] : undefined));
  const isLoading = useAppStore((state) =>
    taskId ? (state.taskPlans.loadingByTaskId[taskId] ?? false) : false,
  );
  const isLoaded = useAppStore((state) =>
    taskId ? (state.taskPlans.loadedByTaskId[taskId] ?? false) : false,
  );
  const isSaving = useAppStore((state) =>
    taskId ? (state.taskPlans.savingByTaskId[taskId] ?? false) : false,
  );
  const setTaskPlan = useAppStore((state) => state.setTaskPlan);
  const setTaskPlanLoading = useAppStore((state) => state.setTaskPlanLoading);
  const setTaskPlanSaving = useAppStore((state) => state.setTaskPlanSaving);
  const markTaskPlanSeen = useAppStore((state) => state.markTaskPlanSeen);
  const connectionStatus = useAppStore((state) => state.connection.status);

  const [error, setError] = useState<string | null>(null);

  const fetchPlan = useCallback(async () => {
    if (!taskId) return;

    setTaskPlanLoading(taskId, true);
    setError(null);
    try {
      const fetchedPlan = await getTaskPlan(taskId);
      setTaskPlan(taskId, fetchedPlan);
      // Initial fetch is not a notification — mark as seen so no indicator flashes.
      markTaskPlanSeen(taskId);
    } catch (err) {
      console.error("Failed to fetch task plan:", err);
      setError(err instanceof Error ? err.message : "Failed to fetch plan");
    } finally {
      setTaskPlanLoading(taskId, false);
    }
  }, [taskId, setTaskPlan, setTaskPlanLoading, markTaskPlanSeen]);

  // Fetch plan on mount or when taskId changes
  useEffect(() => {
    if (connectionStatus !== "connected") return;
    if (taskId && !isLoaded && !isLoading) {
      fetchPlan();
    }
  }, [taskId, isLoaded, isLoading, fetchPlan, connectionStatus]);

  // Refetch when becoming visible (e.g., tab switch)
  useEffect(() => {
    const wasHidden = !prevVisibleRef.current;
    const isNowVisible = visible;
    prevVisibleRef.current = visible;

    // Only refetch when transitioning from hidden to visible
    if (wasHidden && isNowVisible && connectionStatus === "connected" && taskId) {
      fetchPlan();
    }
  }, [visible, connectionStatus, taskId, fetchPlan]);

  const savePlan = useCallback(
    async (content: string, title?: string): Promise<TaskPlan | null> => {
      if (!taskId) return null;

      setTaskPlanSaving(taskId, true);
      setError(null);
      try {
        let savedPlan: TaskPlan;
        if (plan) {
          // Update existing plan
          savedPlan = await updateTaskPlan(taskId, content, title);
        } else {
          // Create new plan
          savedPlan = await createTaskPlan(taskId, content, title);
        }
        setTaskPlan(taskId, savedPlan);
        return savedPlan;
      } catch (err) {
        console.error("Failed to save task plan:", err);
        setError(err instanceof Error ? err.message : "Failed to save plan");
        return null;
      } finally {
        setTaskPlanSaving(taskId, false);
      }
    },
    [taskId, plan, setTaskPlan, setTaskPlanSaving],
  );

  const removePlan = useCallback(async (): Promise<boolean> => {
    if (!taskId) return false;

    setTaskPlanSaving(taskId, true);
    setError(null);
    try {
      await deleteTaskPlan(taskId);
      setTaskPlan(taskId, null);
      return true;
    } catch (err) {
      console.error("Failed to delete task plan:", err);
      setError(err instanceof Error ? err.message : "Failed to delete plan");
      return false;
    } finally {
      setTaskPlanSaving(taskId, false);
    }
  }, [taskId, setTaskPlan, setTaskPlanSaving]);

  const revisionsBundle = useTaskPlanRevisions(taskId, setTaskPlanSaving, setError);

  return {
    plan: plan ?? null,
    isLoading,
    isSaving,
    error,
    savePlan,
    deletePlan: removePlan,
    refetch: fetchPlan,
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
  const revisions = useAppStore((state) =>
    taskId ? (state.taskPlans.revisionsByTaskId[taskId] ?? EMPTY_REVISIONS) : EMPTY_REVISIONS,
  ) as TaskPlanRevision[];
  const isLoadingRevisions = useAppStore((state) =>
    taskId ? (state.taskPlans.revisionsLoadingByTaskId[taskId] ?? false) : false,
  );
  const isRevisionsLoaded = useAppStore((state) =>
    taskId ? (state.taskPlans.revisionsLoadedByTaskId[taskId] ?? false) : false,
  );
  const connectionStatus = useAppStore((state) => state.connection.status);
  const storeApi = useAppStoreApi();
  const setPlanRevisions = useAppStore((state) => state.setPlanRevisions);
  const setPlanRevisionsLoading = useAppStore((state) => state.setPlanRevisionsLoading);
  const cachePlanRevisionContent = useAppStore((state) => state.cachePlanRevisionContent);

  const loadRevisions = useCallback(async () => {
    if (!taskId) return;
    setPlanRevisionsLoading(taskId, true);
    try {
      const list = await listPlanRevisions(taskId);
      setPlanRevisions(taskId, list);
    } catch (err) {
      console.error("Failed to load plan revisions:", err);
      setError(err instanceof Error ? err.message : "Failed to load revisions");
    } finally {
      setPlanRevisionsLoading(taskId, false);
    }
  }, [taskId, setPlanRevisions, setPlanRevisionsLoading, setError]);

  // Load revisions once on mount — events may have fired before the WS connected.
  useEffect(() => {
    if (connectionStatus !== "connected") return;
    if (!taskId || isRevisionsLoaded || isLoadingRevisions) return;
    loadRevisions();
  }, [taskId, connectionStatus, isRevisionsLoaded, isLoadingRevisions, loadRevisions]);

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
