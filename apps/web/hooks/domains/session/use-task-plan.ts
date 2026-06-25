import { useEffect, useCallback, useState, useRef } from "react";
import { useQuery, useQueryClient, type QueryClient } from "@tanstack/react-query";
import { useAppStore } from "@/components/state-provider";
import {
  createTaskPlan,
  updateTaskPlan,
  deleteTaskPlan,
  revertPlanRevision,
} from "@/lib/api/domains/plan-api";
import {
  planRevisionQueryOptions,
  taskPlanQueryOptions,
  taskPlanRevisionsQueryOptions,
} from "@/lib/query/query-options";
import type { TaskPlan, TaskPlanRevision } from "@/lib/types/http";

const EMPTY_REVISIONS = Object.freeze([]) as unknown as TaskPlanRevision[];

/**
 * Hook to fetch and manage the plan for a task.
 * Plans are task-scoped (one plan per task, shared across all sessions).
 * @param taskId - The task ID to fetch the plan for
 * @param options.visible - When true, refetches the plan (use for tab visibility)
 */
export function useTaskPlan(taskId: string | null, options?: { visible?: boolean }) {
  const { visible = true } = options ?? {};
  const queryClient = useQueryClient();
  const prevVisibleRef = useRef(visible);
  const connectionStatus = useAppStore((state) => state.connection.status);
  const planQuery = useQuery(taskPlanQueryOptions(taskId ?? "", connectionStatus === "connected"));
  const storePlan = useAppStore((state) => (taskId ? state.taskPlans.byTaskId[taskId] : undefined));
  const storeIsLoading = useAppStore((state) =>
    taskId ? (state.taskPlans.loadingByTaskId[taskId] ?? false) : false,
  );
  const storeIsLoaded = useAppStore((state) =>
    taskId ? (state.taskPlans.loadedByTaskId[taskId] ?? false) : false,
  );
  const isSaving = useAppStore((state) =>
    taskId ? (state.taskPlans.savingByTaskId[taskId] ?? false) : false,
  );
  const setTaskPlan = useAppStore((state) => state.setTaskPlan);
  const setTaskPlanLoading = useAppStore((state) => state.setTaskPlanLoading);
  const setTaskPlanSaving = useAppStore((state) => state.setTaskPlanSaving);
  const markTaskPlanSeen = useAppStore((state) => state.markTaskPlanSeen);
  const plan = planQuery.data !== undefined ? planQuery.data : storePlan;
  const isLoading = planQuery.isFetching || storeIsLoading;
  const isLoaded = planQuery.isSuccess || storeIsLoaded;

  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!taskId || planQuery.data === undefined) return;
    setTaskPlan(taskId, planQuery.data);
  }, [planQuery.data, setTaskPlan, taskId]);

  useEffect(() => {
    if (!taskId) return;
    setTaskPlanLoading(taskId, planQuery.isFetching);
  }, [planQuery.isFetching, setTaskPlanLoading, taskId]);

  const fetchPlan = useCallback(async () => {
    if (!taskId) return;

    setTaskPlanLoading(taskId, true);
    setError(null);
    try {
      const fetchedPlan = await queryClient.fetchQuery(taskPlanQueryOptions(taskId));
      setTaskPlan(taskId, fetchedPlan);
      // Initial fetch is not a notification — mark as seen so no indicator flashes.
      markTaskPlanSeen(taskId);
    } catch (err) {
      console.error("Failed to fetch task plan:", err);
      setError(err instanceof Error ? err.message : "Failed to fetch plan");
    } finally {
      setTaskPlanLoading(taskId, false);
    }
  }, [taskId, setTaskPlan, setTaskPlanLoading, markTaskPlanSeen, queryClient]);

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

  const { savePlan, removePlan } = useTaskPlanMutations({
    taskId,
    plan,
    setTaskPlan,
    setTaskPlanSaving,
    setError,
    queryClient,
  });

  const revisionsBundle = useTaskPlanRevisions(taskId, setTaskPlanSaving, setError, queryClient);

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

type TaskPlanMutationArgs = {
  taskId: string | null;
  plan: TaskPlan | null | undefined;
  setTaskPlan: (taskId: string, plan: TaskPlan | null) => void;
  setTaskPlanSaving: (taskId: string, saving: boolean) => void;
  setError: (error: string | null) => void;
  queryClient: QueryClient;
};

function useTaskPlanMutations({
  taskId,
  plan,
  setTaskPlan,
  setTaskPlanSaving,
  setError,
  queryClient,
}: TaskPlanMutationArgs) {
  const savePlan = useCallback(
    async (content: string, title?: string): Promise<TaskPlan | null> => {
      if (!taskId) return null;
      setTaskPlanSaving(taskId, true);
      setError(null);
      try {
        const savedPlan = plan
          ? await updateTaskPlan(taskId, content, title)
          : await createTaskPlan(taskId, content, title);
        queryClient.setQueryData(taskPlanQueryOptions(taskId).queryKey, savedPlan);
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
    [taskId, plan, setTaskPlan, setTaskPlanSaving, setError, queryClient],
  );

  const removePlan = useCallback(async (): Promise<boolean> => {
    if (!taskId) return false;
    setTaskPlanSaving(taskId, true);
    setError(null);
    try {
      await deleteTaskPlan(taskId);
      queryClient.setQueryData(taskPlanQueryOptions(taskId).queryKey, null);
      setTaskPlan(taskId, null);
      return true;
    } catch (err) {
      console.error("Failed to delete task plan:", err);
      setError(err instanceof Error ? err.message : "Failed to delete plan");
      return false;
    } finally {
      setTaskPlanSaving(taskId, false);
    }
  }, [taskId, setTaskPlan, setTaskPlanSaving, setError, queryClient]);

  return { savePlan, removePlan };
}

const EMPTY_PAIR: readonly [string | null, string | null] = Object.freeze([
  null,
  null,
]) as readonly [string | null, string | null];

function useTaskPlanRevisions(
  taskId: string | null,
  setTaskPlanSaving: (taskId: string, saving: boolean) => void,
  setError: (err: string | null) => void,
  queryClient: QueryClient,
) {
  const connectionStatus = useAppStore((state) => state.connection.status);
  const revisionsQuery = useQuery(
    taskPlanRevisionsQueryOptions(taskId ?? "", connectionStatus === "connected"),
  );
  const revisions = revisionsQuery.data ?? EMPTY_REVISIONS;
  const isLoadingRevisions = revisionsQuery.isFetching;
  const isRevisionsLoaded = revisionsQuery.isSuccess;

  const loadRevisions = useCallback(async () => {
    if (!taskId) return;
    try {
      await queryClient.fetchQuery(taskPlanRevisionsQueryOptions(taskId));
    } catch (err) {
      console.error("Failed to load plan revisions:", err);
      setError(err instanceof Error ? err.message : "Failed to load revisions");
    }
  }, [taskId, setError, queryClient]);

  // Load revisions once on mount — events may have fired before the WS connected.
  useEffect(() => {
    if (connectionStatus !== "connected") return;
    if (!taskId || isRevisionsLoaded || isLoadingRevisions) return;
    loadRevisions();
  }, [taskId, connectionStatus, isRevisionsLoaded, isLoadingRevisions, loadRevisions]);

  const loadRevisionContent = useCallback(
    async (revisionId: string): Promise<string> => {
      if (!taskId) return "";
      // Pass taskId so the backend can enforce revision-belongs-to-task.
      const queryOptions = planRevisionQueryOptions(taskId, revisionId);
      const cached = queryClient.getQueryData<TaskPlanRevision>(queryOptions.queryKey);
      if (cached?.content !== undefined) return cached.content ?? "";
      const rev = await queryClient.fetchQuery(queryOptions);
      return rev.content ?? "";
    },
    [taskId, queryClient],
  );

  const revertTo = useCallback(
    async (revisionId: string, authorName?: string): Promise<TaskPlanRevision | null> => {
      if (!taskId) return null;
      setTaskPlanSaving(taskId, true);
      setError(null);
      try {
        const revision = await revertPlanRevision(taskId, revisionId, authorName);
        queryClient.invalidateQueries({
          exact: true,
          queryKey: taskPlanQueryOptions(taskId).queryKey,
        });
        queryClient.invalidateQueries({
          exact: true,
          queryKey: taskPlanRevisionsQueryOptions(taskId).queryKey,
        });
        return revision;
      } catch (err) {
        console.error("Failed to revert plan:", err);
        setError(err instanceof Error ? err.message : "Failed to revert plan");
        return null;
      } finally {
        setTaskPlanSaving(taskId, false);
      }
    },
    [taskId, setTaskPlanSaving, setError, queryClient],
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
