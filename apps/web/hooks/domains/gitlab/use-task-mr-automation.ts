"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { getTaskMRAutomation, updateTaskMRAutomation } from "@/lib/api/domains/gitlab-api";
import type { TaskMRAutomationOptions, TaskMRAutomationPatch } from "@/lib/types/gitlab";

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : "Failed to load MR automation options.";
}

/**
 * Fetch + optimistic-patch a task's GitLab MR lifecycle notification
 * switches. Mirrors useTaskCIAutomationOptions (GitHub) but keeps state
 * local to the hook rather than the global store — there is no other
 * consumer of this data yet.
 *
 * refresh and update track independent per-taskId generation counters (like
 * the store-backed GitHub hook), so a refresh started while an update's PATCH
 * is still in flight cannot suppress the update's response, or vice versa.
 * Every applied response is additionally gated on the hook still being
 * "current" for that taskId (activeTaskIdRef), so a response for a task the
 * caller has since switched away from can never leak into the displayed
 * state.
 */
export function useTaskMRAutomationOptions(taskId: string | null) {
  const [options, setOptions] = useState<TaskMRAutomationOptions | null>(null);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const refreshRequestRef = useRef<Record<string, number>>({});
  const updateRequestRef = useRef<Record<string, number>>({});
  const loadedForTaskRef = useRef<string | null>(null);
  const activeTaskIdRef = useRef<string | null>(taskId);
  activeTaskIdRef.current = taskId;

  const isCurrent = useCallback(
    (calledForTaskId: string, requestMap: Record<string, number>, requestId: number) =>
      activeTaskIdRef.current === calledForTaskId && requestMap[calledForTaskId] === requestId,
    [],
  );

  const refresh = useCallback(async (): Promise<TaskMRAutomationOptions | null> => {
    if (!taskId) return null;
    const requestId = (refreshRequestRef.current[taskId] ?? 0) + 1;
    refreshRequestRef.current[taskId] = requestId;
    setLoading(true);
    setError(null);
    try {
      const response = await getTaskMRAutomation(taskId, { cache: "no-store" });
      if (isCurrent(taskId, refreshRequestRef.current, requestId)) {
        setOptions(response);
        loadedForTaskRef.current = taskId;
      }
      return response;
    } catch (err) {
      if (isCurrent(taskId, refreshRequestRef.current, requestId)) {
        setError(errorMessage(err));
      }
      throw err;
    } finally {
      if (isCurrent(taskId, refreshRequestRef.current, requestId)) {
        setLoading(false);
      }
    }
  }, [isCurrent, taskId]);

  const update = useCallback(
    async (patch: TaskMRAutomationPatch): Promise<TaskMRAutomationOptions | null> => {
      if (!taskId) return null;
      const requestId = (updateRequestRef.current[taskId] ?? 0) + 1;
      updateRequestRef.current[taskId] = requestId;
      const previous = options;
      // Optimistic update: apply immediately, revert on failure (AC27).
      setOptions((current) => (current ? { ...current, ...patch } : current));
      setSaving(true);
      setError(null);
      try {
        const response = await updateTaskMRAutomation(taskId, patch, { cache: "no-store" });
        if (isCurrent(taskId, updateRequestRef.current, requestId)) {
          setOptions(response);
        }
        return response;
      } catch (err) {
        if (isCurrent(taskId, updateRequestRef.current, requestId)) {
          setOptions(previous);
          setError(errorMessage(err));
        }
        throw err;
      } finally {
        if (isCurrent(taskId, updateRequestRef.current, requestId)) {
          setSaving(false);
        }
      }
    },
    [isCurrent, options, taskId],
  );

  useEffect(() => {
    if (!taskId) {
      setOptions(null);
      setError(null);
      setSaving(false);
      setLoading(false);
      loadedForTaskRef.current = null;
      return;
    }
    if (loadedForTaskRef.current === taskId) return;
    // Reset to a clean slate for the new task immediately — the previous
    // task's options/saving/error state must not remain visible (or
    // actionable) while the new task's options are still loading, regardless
    // of how a still-in-flight request for the previous task eventually
    // settles (isCurrent above ensures it can no longer write to state).
    setOptions(null);
    setError(null);
    setSaving(false);
    void refresh().catch(() => {
      // Error state is stored for the UI; callers can retry via refresh.
    });
    // refresh is stable per taskId (useCallback dep is only taskId), so
    // omitting it from deps avoids re-running on every render.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [taskId]);

  return { options, loading, saving, error, refresh, update };
}
