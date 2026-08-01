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
  const [options, setOptions] = useState<TaskMRAutomationOptions | null>(null);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const refreshRequestRef = useRef<Record<string, number>>({});
  const updateRequestRef = useRef<Record<string, number>>({});
  const updateSettleCounterRef = useRef<Record<string, number>>({});
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
    const settleCounterAtStart = updateSettleCounterRef.current[taskId] ?? 0;
    setLoading(true);
    setError(null);
    try {
      const response = await getTaskMRAutomation(taskId, { cache: "no-store" });
      const noNewerUpdateSettled =
        (updateSettleCounterRef.current[taskId] ?? 0) === settleCounterAtStart;
      if (isCurrent(taskId, refreshRequestRef.current, requestId) && noNewerUpdateSettled) {
        setOptions(response);
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
        updateSettleCounterRef.current[taskId] = (updateSettleCounterRef.current[taskId] ?? 0) + 1;
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
      return;
    }
    // Reset to a clean slate for the new task immediately — the previous
    // task's options/saving/error state must not remain visible (or
    // actionable) while this task's options are (re-)loading, regardless of
    // how a still-in-flight request for the previous task eventually settles
    // (isCurrent above ensures it can no longer write to state). Always
    // re-fetches, including when returning to a task visited earlier in this
    // hook's lifetime — options was just cleared above, so skipping the
    // fetch here would leave it permanently null.
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
