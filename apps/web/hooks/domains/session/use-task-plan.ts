import { useEffect, useCallback, useState, useRef } from 'react';
import { useAppStore } from '@/components/state-provider';
import {
  getTaskPlan,
  createTaskPlan,
  updateTaskPlan,
  deleteTaskPlan,
} from '@/lib/api/domains/plan-api';
import type { TaskPlan } from '@/lib/types/http';

/**
 * Hook to fetch and manage the plan for a task.
 * Plans are task-scoped (one plan per task, shared across all sessions).
 * @param taskId - The task ID to fetch the plan for
 * @param options.visible - When true, refetches the plan (use for tab visibility)
 */
export function useTaskPlan(taskId: string | null, options?: { visible?: boolean }) {
  const { visible = true } = options ?? {};
  const prevVisibleRef = useRef(visible);
  const plan = useAppStore((state) =>
    taskId ? state.taskPlans.byTaskId[taskId] : undefined
  );
  const isLoading = useAppStore((state) =>
    taskId ? state.taskPlans.loadingByTaskId[taskId] ?? false : false
  );
  const isLoaded = useAppStore((state) =>
    taskId ? state.taskPlans.loadedByTaskId[taskId] ?? false : false
  );
  const isSaving = useAppStore((state) =>
    taskId ? state.taskPlans.savingByTaskId[taskId] ?? false : false
  );
  const setTaskPlan = useAppStore((state) => state.setTaskPlan);
  const setTaskPlanLoading = useAppStore((state) => state.setTaskPlanLoading);
  const setTaskPlanSaving = useAppStore((state) => state.setTaskPlanSaving);
  const connectionStatus = useAppStore((state) => state.connection.status);

  const [error, setError] = useState<string | null>(null);

  const fetchPlan = useCallback(async () => {
    if (!taskId) return;

    setTaskPlanLoading(taskId, true);
    setError(null);
    try {
      const fetchedPlan = await getTaskPlan(taskId);
      setTaskPlan(taskId, fetchedPlan);
    } catch (err) {
      console.error('Failed to fetch task plan:', err);
      setError(err instanceof Error ? err.message : 'Failed to fetch plan');
    } finally {
      setTaskPlanLoading(taskId, false);
    }
  }, [taskId, setTaskPlan, setTaskPlanLoading]);

  // Fetch plan on mount or when taskId changes
  useEffect(() => {
    if (connectionStatus !== 'connected') return;
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
    if (wasHidden && isNowVisible && connectionStatus === 'connected' && taskId) {
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
        console.error('Failed to save task plan:', err);
        setError(err instanceof Error ? err.message : 'Failed to save plan');
        return null;
      } finally {
        setTaskPlanSaving(taskId, false);
      }
    },
    [taskId, plan, setTaskPlan, setTaskPlanSaving]
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
      console.error('Failed to delete task plan:', err);
      setError(err instanceof Error ? err.message : 'Failed to delete plan');
      return false;
    } finally {
      setTaskPlanSaving(taskId, false);
    }
  }, [taskId, setTaskPlan, setTaskPlanSaving]);

  return {
    plan: plan ?? null,
    isLoading,
    isSaving,
    error,
    savePlan,
    deletePlan: removePlan,
    refetch: fetchPlan,
  };
}

