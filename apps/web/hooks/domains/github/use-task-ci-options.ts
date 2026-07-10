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
  const refreshRequestRef = useRef<Record<string, number>>({});
  const updateRequestRef = useRef<Record<string, number>>({});
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
    const requestId = (refreshRequestRef.current[taskId] ?? 0) + 1;
    refreshRequestRef.current[taskId] = requestId;
    setLoading(taskId, true);
    setError(taskId, null);
    try {
      const response = await getTaskCIAutomationOptions(taskId, { cache: "no-store" });
      if (refreshRequestRef.current[taskId] === requestId) {
        setOptions(taskId, response);
      }
      return response;
    } catch (err) {
      if (refreshRequestRef.current[taskId] === requestId) {
        setError(taskId, errorMessage(err));
      }
      throw err;
    } finally {
      if (refreshRequestRef.current[taskId] === requestId) {
        setLoading(taskId, false);
      }
    }
  }, [setError, setLoading, setOptions, taskId]);

  const update = useCallback(
    async (patch: TaskCIAutomationPatch): Promise<TaskCIAutomationOptions | null> => {
      if (!taskId) return null;
      const requestId = (updateRequestRef.current[taskId] ?? 0) + 1;
      updateRequestRef.current[taskId] = requestId;
      setSaving(taskId, true);
      setError(taskId, null);
      try {
        const response = await updateTaskCIAutomationOptions(taskId, patch, { cache: "no-store" });
        if (updateRequestRef.current[taskId] === requestId) {
          setOptions(taskId, response);
        }
        return response;
      } catch (err) {
        if (updateRequestRef.current[taskId] === requestId) {
          setError(taskId, errorMessage(err));
        }
        throw err;
      } finally {
        if (updateRequestRef.current[taskId] === requestId) {
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
