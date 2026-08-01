"use client";

import { useCallback, useEffect, useRef } from "react";
import { getTaskMRAutomation, updateTaskMRAutomation } from "@/lib/api/domains/gitlab-api";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import type { TaskMRAutomationOptions, TaskMRAutomationPatch } from "@/lib/types/gitlab";

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : "Failed to load MR automation options.";
}

/**
 * Fetch + optimistic-patch a task's GitLab MR lifecycle notification
 * switches. State lives in the GitLab store slice (taskMRAutomation),
 * keyed by taskId, so a WS gitlab.task_mr_options.updated push — from an
 * MCP tool call, another browser tab, or the orchestrator's own recovery
 * publish — reaches every mounted instance immediately (mirrors
 * useTaskCIAutomationOptions, GitHub's equivalent).
 *
 * Each task's state lives in its own store slot, so switching taskId
 * cannot leak a stale response into a different task's display the way
 * shared local state could — no reset-on-switch or "is this still the
 * active task" gating is needed. An already-populated slot (from a WS
 * push or an earlier mount) is trusted rather than re-fetched.
 *
 * refresh and update still track independent per-taskId generation
 * counters so a refresh started while an update's PATCH is still in
 * flight cannot suppress the update's response, or vice versa, for the
 * same task.
 *
 * refresh's own response is further gated on updateSettleCounterRef: a GET
 * can be in flight when a PATCH starts and resolve afterward with pre-patch
 * data (the server hadn't processed the PATCH yet when the GET ran). Since
 * that GET's own generation check alone can't detect this — it's still
 * "current" — refresh additionally requires that no update has settled since
 * it started; update bumps the counter in its `finally` regardless of
 * success/failure so a stale GET can never flip a just-saved switch back.
 */
export function useTaskMRAutomationOptions(taskId: string | null) {
  const refreshRequestRef = useRef<Record<string, number>>({});
  const updateRequestRef = useRef<Record<string, number>>({});
  const updateSettleCounterRef = useRef<Record<string, number>>({});
  const storeApi = useAppStoreApi();

  const options = useAppStore((state) =>
    taskId ? (state.taskMRAutomation.byTaskId[taskId] ?? null) : null,
  );
  const loading = useAppStore((state) =>
    taskId ? Boolean(state.taskMRAutomation.loading[taskId]) : false,
  );
  const saving = useAppStore((state) =>
    taskId ? Boolean(state.taskMRAutomation.saving[taskId]) : false,
  );
  const error = useAppStore((state) =>
    taskId ? (state.taskMRAutomation.errors[taskId] ?? null) : null,
  );
  const setOptions = useAppStore((state) => state.setTaskMRAutomationOptions);
  const setLoading = useAppStore((state) => state.setTaskMRAutomationLoading);
  const setSaving = useAppStore((state) => state.setTaskMRAutomationSaving);
  const setError = useAppStore((state) => state.setTaskMRAutomationError);

  const refresh = useCallback(async (): Promise<TaskMRAutomationOptions | null> => {
    if (!taskId) return null;
    const requestId = (refreshRequestRef.current[taskId] ?? 0) + 1;
    refreshRequestRef.current[taskId] = requestId;
    const settleCounterAtStart = updateSettleCounterRef.current[taskId] ?? 0;
    setLoading(taskId, true);
    setError(taskId, null);
    try {
      const response = await getTaskMRAutomation(taskId, { cache: "no-store" });
      const noNewerUpdateSettled =
        (updateSettleCounterRef.current[taskId] ?? 0) === settleCounterAtStart;
      if (refreshRequestRef.current[taskId] === requestId && noNewerUpdateSettled) {
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
    async (patch: TaskMRAutomationPatch): Promise<TaskMRAutomationOptions | null> => {
      if (!taskId) return null;
      const requestId = (updateRequestRef.current[taskId] ?? 0) + 1;
      updateRequestRef.current[taskId] = requestId;
      const previous = storeApi.getState().taskMRAutomation.byTaskId[taskId] ?? null;
      // Optimistic update: apply immediately, revert on failure (AC27).
      if (previous) {
        setOptions(taskId, { ...previous, ...patch });
      }
      setSaving(taskId, true);
      setError(taskId, null);
      try {
        const response = await updateTaskMRAutomation(taskId, patch, { cache: "no-store" });
        if (updateRequestRef.current[taskId] === requestId) {
          setOptions(taskId, response);
        }
        return response;
      } catch (err) {
        if (updateRequestRef.current[taskId] === requestId) {
          if (previous) setOptions(taskId, previous);
          setError(taskId, errorMessage(err));
        }
        throw err;
      } finally {
        updateSettleCounterRef.current[taskId] = (updateSettleCounterRef.current[taskId] ?? 0) + 1;
        if (updateRequestRef.current[taskId] === requestId) {
          setSaving(taskId, false);
        }
      }
    },
    [setError, setOptions, setSaving, storeApi, taskId],
  );

  useEffect(() => {
    if (!taskId || options || loading || error) return;
    void refresh().catch(() => {
      // Error state is stored for the UI; callers can retry via refresh.
    });
  }, [error, loading, options, refresh, taskId]);

  return { options, loading, saving, error, refresh, update };
}
