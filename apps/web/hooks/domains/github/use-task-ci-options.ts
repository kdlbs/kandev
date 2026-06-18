"use client";

import { useCallback, useEffect, useRef } from "react";
import {
  getTaskCIAutomationOptions,
  updateTaskCIAutomationOptions,
} from "@/lib/api/domains/github-api";
import { useAppStore } from "@/components/state-provider";
import type { TaskCIAutomationPatch, TaskCIAutomationOptions } from "@/lib/types/github";

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : "Failed to load CI automation options.";
}

export function useTaskCIAutomationOptions(taskId: string | null) {
  const refreshRequestRef = useRef(0);
  const updateRequestRef = useRef(0);
  const options = useAppStore((state) =>
    taskId ? (state.taskCIAutomation.byTaskId[taskId] ?? null) : null,
  );
  const loading = useAppStore((state) =>
    taskId ? Boolean(state.taskCIAutomation.loading[taskId]) : false,
  );
  const saving = useAppStore((state) =>
    taskId ? Boolean(state.taskCIAutomation.saving[taskId]) : false,
  );
  const error = useAppStore((state) =>
    taskId ? (state.taskCIAutomation.errors[taskId] ?? null) : null,
  );
  const setOptions = useAppStore((state) => state.setTaskCIAutomationOptions);
  const setLoading = useAppStore((state) => state.setTaskCIAutomationLoading);
  const setSaving = useAppStore((state) => state.setTaskCIAutomationSaving);
  const setError = useAppStore((state) => state.setTaskCIAutomationError);

  const refresh = useCallback(async (): Promise<TaskCIAutomationOptions | null> => {
    if (!taskId) return null;
    const requestId = refreshRequestRef.current + 1;
    refreshRequestRef.current = requestId;
    setLoading(taskId, true);
    setError(taskId, null);
    try {
      const response = await getTaskCIAutomationOptions(taskId, { cache: "no-store" });
      if (refreshRequestRef.current === requestId) {
        setOptions(taskId, response);
      }
      return response;
    } catch (err) {
      if (refreshRequestRef.current === requestId) {
        setError(taskId, errorMessage(err));
      }
      throw err;
    } finally {
      if (refreshRequestRef.current === requestId) {
        setLoading(taskId, false);
      }
    }
  }, [setError, setLoading, setOptions, taskId]);

  const update = useCallback(
    async (patch: TaskCIAutomationPatch): Promise<TaskCIAutomationOptions | null> => {
      if (!taskId) return null;
      const requestId = updateRequestRef.current + 1;
      updateRequestRef.current = requestId;
      setSaving(taskId, true);
      setError(taskId, null);
      try {
        const response = await updateTaskCIAutomationOptions(taskId, patch, { cache: "no-store" });
        if (updateRequestRef.current === requestId) {
          setOptions(taskId, response);
        }
        return response;
      } catch (err) {
        if (updateRequestRef.current === requestId) {
          setError(taskId, errorMessage(err));
        }
        throw err;
      } finally {
        if (updateRequestRef.current === requestId) {
          setSaving(taskId, false);
        }
      }
    },
    [setError, setOptions, setSaving, taskId],
  );

  const resetPrompt = useCallback(() => update({ auto_fix_prompt_override: null }), [update]);

  useEffect(() => {
    if (!taskId || options || loading || error) return;
    void refresh().catch(() => {
      // Error state is stored for the UI; callers can retry via refresh.
    });
  }, [error, loading, options, refresh, taskId]);

  return { options, loading, saving, error, refresh, update, resetPrompt };
}
