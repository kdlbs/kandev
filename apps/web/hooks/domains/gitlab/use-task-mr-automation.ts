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
 * consumer of this data yet, and per-taskId request-generation refs already
 * guard against stale responses the same way the store-backed hook does.
 */
export function useTaskMRAutomationOptions(taskId: string | null) {
  const [options, setOptions] = useState<TaskMRAutomationOptions | null>(null);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const requestRef = useRef(0);
  const loadedForTaskRef = useRef<string | null>(null);

  const refresh = useCallback(async (): Promise<TaskMRAutomationOptions | null> => {
    if (!taskId) return null;
    const requestId = ++requestRef.current;
    setLoading(true);
    setError(null);
    try {
      const response = await getTaskMRAutomation(taskId, { cache: "no-store" });
      if (requestRef.current === requestId) {
        setOptions(response);
        loadedForTaskRef.current = taskId;
      }
      return response;
    } catch (err) {
      if (requestRef.current === requestId) {
        setError(errorMessage(err));
      }
      throw err;
    } finally {
      if (requestRef.current === requestId) {
        setLoading(false);
      }
    }
  }, [taskId]);

  const update = useCallback(
    async (patch: TaskMRAutomationPatch): Promise<TaskMRAutomationOptions | null> => {
      if (!taskId) return null;
      const requestId = ++requestRef.current;
      const previous = options;
      // Optimistic update: apply immediately, revert on failure (AC27).
      setOptions((current) => (current ? { ...current, ...patch } : current));
      setSaving(true);
      setError(null);
      try {
        const response = await updateTaskMRAutomation(taskId, patch, { cache: "no-store" });
        if (requestRef.current === requestId) {
          setOptions(response);
        }
        return response;
      } catch (err) {
        if (requestRef.current === requestId) {
          setOptions(previous);
          setError(errorMessage(err));
        }
        throw err;
      } finally {
        if (requestRef.current === requestId) {
          setSaving(false);
        }
      }
    },
    [options, taskId],
  );

  useEffect(() => {
    if (!taskId) {
      setOptions(null);
      loadedForTaskRef.current = null;
      return;
    }
    if (loadedForTaskRef.current === taskId) return;
    void refresh().catch(() => {
      // Error state is stored for the UI; callers can retry via refresh.
    });
    // refresh is stable per taskId (useCallback dep is only taskId), so
    // omitting it from deps avoids re-running on every render.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [taskId]);

  return { options, loading, saving, error, refresh, update };
}
