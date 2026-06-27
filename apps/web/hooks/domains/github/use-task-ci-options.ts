"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  getTaskCIAutomationOptions,
  updateTaskCIAutomationOptions,
} from "@/lib/api/domains/github-api";
import { qk } from "@/lib/query/keys";
import { taskCiOptionsQueryOptions } from "@/lib/query/query-options/github";
import type { TaskCIAutomationPatch, TaskCIAutomationOptions } from "@/lib/types/github";

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : "Failed to load CI automation options.";
}

export function useTaskCIAutomationOptions(taskId: string | null) {
  const queryClient = useQueryClient();
  const query = useQuery({
    ...taskCiOptionsQueryOptions(taskId ?? ""),
    enabled: Boolean(taskId),
  });
  const refreshRequestRef = useRef<Record<string, number>>({});
  const updateRequestRef = useRef<Record<string, number>>({});
  const options = query.data ?? null;
  const [manualLoading, setManualLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async (): Promise<TaskCIAutomationOptions | null> => {
    if (!taskId) return null;
    const requestId = (refreshRequestRef.current[taskId] ?? 0) + 1;
    refreshRequestRef.current[taskId] = requestId;
    await queryClient.cancelQueries({ queryKey: qk.integrations.github.taskCiOptions(taskId) });
    setManualLoading(true);
    setError(null);
    try {
      const response = await getTaskCIAutomationOptions(taskId, { cache: "no-store" });
      if (refreshRequestRef.current[taskId] === requestId) {
        queryClient.setQueryData(qk.integrations.github.taskCiOptions(taskId), response);
      }
      return response;
    } catch (err) {
      if (refreshRequestRef.current[taskId] === requestId) {
        setError(errorMessage(err));
      }
      throw err;
    } finally {
      if (refreshRequestRef.current[taskId] === requestId) {
        setManualLoading(false);
      }
    }
  }, [queryClient, taskId]);

  const update = useCallback(
    async (patch: TaskCIAutomationPatch): Promise<TaskCIAutomationOptions | null> => {
      if (!taskId) return null;
      const requestId = (updateRequestRef.current[taskId] ?? 0) + 1;
      updateRequestRef.current[taskId] = requestId;
      setSaving(true);
      setError(null);
      try {
        const response = await updateTaskCIAutomationOptions(taskId, patch, { cache: "no-store" });
        if (updateRequestRef.current[taskId] === requestId) {
          queryClient.setQueryData(qk.integrations.github.taskCiOptions(taskId), response);
        }
        return response;
      } catch (err) {
        if (updateRequestRef.current[taskId] === requestId) {
          setError(errorMessage(err));
        }
        throw err;
      } finally {
        if (updateRequestRef.current[taskId] === requestId) {
          setSaving(false);
        }
      }
    },
    [queryClient, taskId],
  );

  const resetPrompt = useCallback(() => update({ auto_fix_prompt_override: null }), [update]);

  useEffect(() => {
    setError(null);
    setManualLoading(false);
    setSaving(false);
  }, [taskId]);

  useMirrorTaskCIQuery({
    error,
    loading: manualLoading,
    options,
    query,
    refresh,
    setError,
    taskId,
  });

  return {
    options,
    loading: manualLoading || (query.isFetching && !query.isSuccess),
    saving,
    error,
    refresh,
    update,
    resetPrompt,
  };
}

function useMirrorTaskCIQuery({
  error,
  loading,
  options,
  query,
  refresh,
  setError,
  taskId,
}: {
  error: string | null;
  loading: boolean;
  options: TaskCIAutomationOptions | null;
  query: {
    data: TaskCIAutomationOptions | undefined;
    error: Error | null;
    isFetching: boolean;
    isSuccess: boolean;
  };
  refresh: () => Promise<TaskCIAutomationOptions | null>;
  setError: (error: string | null) => void;
  taskId: string | null;
}) {
  useEffect(() => {
    if (!taskId || !query.error) return;
    setError(errorMessage(query.error));
  }, [query.error, setError, taskId]);

  useEffect(() => {
    if (!taskId || options || loading || error || query.error || query.isFetching) return;
    void refresh().catch(() => {
      // Error state is stored for the UI; callers can retry via refresh.
    });
  }, [error, loading, options, query.isFetching, refresh, taskId]);
}
