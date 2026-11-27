"use client";

import { useCallback, useEffect, useRef } from "react";
import { retryTaskCIAutoMerge, updateTaskCIAutomationOptions } from "@/lib/api/domains/github-api";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import type { TaskCIAutomationPatch, TaskCIAutomationOptions } from "@/lib/types/github";
import { readJourneyCIOptions } from "@/hooks/journey-metadata-resources";
import { t } from "@/lib/i18n";

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : t("github:failedToLoadCiAutomationOptions");
}

function useTaskCIRead(taskId: string | null) {
  const store = useAppStoreApi();
  const controllers = useRef(new Map<string, Set<AbortController>>());
  const refreshRequestRef = useRef<Record<string, number>>({});
  const setOptions = useAppStore((state) => state.setTaskCIAutomationOptions);
  const setLoading = useAppStore((state) => state.setTaskCIAutomationLoading);
  const setError = useAppStore((state) => state.setTaskCIAutomationError);

  const refresh = useCallback(
    async (force = true): Promise<TaskCIAutomationOptions | null> => {
      if (!taskId) return null;
      const requestId = (refreshRequestRef.current[taskId] ?? 0) + 1;
      refreshRequestRef.current[taskId] = requestId;
      const controller = new AbortController();
      const active = controllers.current.get(taskId) ?? new Set<AbortController>();
      controllers.current.set(taskId, active);
      active.add(controller);
      setLoading(taskId, true);
      setError(taskId, null);
      try {
        const response = await readJourneyCIOptions(store, taskId, {
          signal: controller.signal,
          refresh: force,
        });
        if (refreshRequestRef.current[taskId] === requestId) {
          setOptions(taskId, response);
        }
        return response;
      } catch (err) {
        if (!controller.signal.aborted && refreshRequestRef.current[taskId] === requestId) {
          setError(taskId, errorMessage(err));
        }
        throw err;
      } finally {
        active.delete(controller);
        if (refreshRequestRef.current[taskId] === requestId) {
          setLoading(taskId, false);
        }
      }
    },
    [setError, setLoading, setOptions, store, taskId],
  );

  useEffect(() => {
    if (!taskId) return;
    const state = store.getState().taskCIAutomation;
    if (!state.byTaskId[taskId] && !state.errors[taskId]) {
      void refresh(false).catch(() => {
        // Error state is stored for the UI; callers can retry via refresh.
      });
    }
    const active = controllers.current;
    return () => {
      for (const controller of active.get(taskId) ?? []) controller.abort();
    };
  }, [refresh, store, taskId]);
  return refresh;
}

export function useTaskCIAutomationOptions(taskId: string | null) {
  const refresh = useTaskCIRead(taskId);
  const updateRequestRef = useRef<Record<string, number>>({});
  const retryRequestRef = useRef<Record<string, number>>({});
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
  const setSaving = useAppStore((state) => state.setTaskCIAutomationSaving);
  const setError = useAppStore((state) => state.setTaskCIAutomationError);

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

  const retryMerge = useCallback(
    async (repositoryId: string, prNumber: number): Promise<{ accepted: boolean } | null> => {
      if (!taskId) return null;
      const requestId = (retryRequestRef.current[taskId] ?? 0) + 1;
      retryRequestRef.current[taskId] = requestId;
      setSaving(taskId, true);
      setError(taskId, null);
      try {
        return await retryTaskCIAutoMerge(taskId, repositoryId, prNumber, { cache: "no-store" });
      } catch (err) {
        if (retryRequestRef.current[taskId] === requestId) {
          setError(taskId, errorMessage(err));
        }
        throw err;
      } finally {
        if (retryRequestRef.current[taskId] === requestId) {
          setSaving(taskId, false);
        }
      }
    },
    [setError, setSaving, taskId],
  );

  return { options, loading, saving, error, refresh, update, resetPrompt, retryMerge };
}
