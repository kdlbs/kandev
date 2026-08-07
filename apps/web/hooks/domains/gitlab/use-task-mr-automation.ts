"use client";

import { useCallback, useEffect, useRef, type RefObject } from "react";
import { getTaskMRAutomation, updateTaskMRAutomation } from "@/lib/api/domains/gitlab-api";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import type { AppState } from "@/lib/state/store";
import type { TaskMRAutomationOptions, TaskMRAutomationPatch } from "@/lib/types/gitlab";

type AppStoreApi = ReturnType<typeof useAppStoreApi>;

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : "Failed to load MR automation options.";
}

// True when `requestId` is still the latest request this hook issued for
// `taskId` on `requestRef`, and no externally-sourced write (a WS push) has
// landed since `externalGenAtStart` was captured. Shared by refresh()'s
// commit check and update()'s commit/rollback checks.
function isCurrentAndUnchangedExternally(
  automation: AppState["taskMRAutomation"],
  requestRef: Record<string, number>,
  taskId: string,
  requestId: number,
  externalGenAtStart: number,
): boolean {
  return (
    requestRef[taskId] === requestId &&
    (automation.externalGeneration[taskId] ?? 0) === externalGenAtStart
  );
}

type MRAutomationRequestContext = {
  taskId: string;
  storeApi: AppStoreApi;
  refreshRequestRef: RefObject<Record<string, number>>;
  updateRequestRef: RefObject<Record<string, number>>;
  updateSettleCounterRef: RefObject<Record<string, number>>;
  setOptions: AppState["setTaskMRAutomationOptions"];
  setLoading: AppState["setTaskMRAutomationLoading"];
  setSaving: AppState["setTaskMRAutomationSaving"];
  setError: AppState["setTaskMRAutomationError"];
};

async function performRefresh(
  ctx: MRAutomationRequestContext,
): Promise<TaskMRAutomationOptions | null> {
  const {
    taskId,
    storeApi,
    refreshRequestRef,
    updateSettleCounterRef,
    setOptions,
    setLoading,
    setError,
  } = ctx;
  const requestId = (refreshRequestRef.current[taskId] ?? 0) + 1;
  refreshRequestRef.current[taskId] = requestId;
  const settleCounterAtStart = updateSettleCounterRef.current[taskId] ?? 0;
  const externalGenAtStart = storeApi.getState().taskMRAutomation.externalGeneration[taskId] ?? 0;
  setLoading(taskId, true);
  setError(taskId, null);
  try {
    const response = await getTaskMRAutomation(taskId, { cache: "no-store" });
    const automation = storeApi.getState().taskMRAutomation;
    const safeToCommit =
      isCurrentAndUnchangedExternally(
        automation,
        refreshRequestRef.current,
        taskId,
        requestId,
        externalGenAtStart,
      ) &&
      !automation.saving[taskId] &&
      (updateSettleCounterRef.current[taskId] ?? 0) === settleCounterAtStart;
    if (safeToCommit) {
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
}

async function performUpdate(
  ctx: MRAutomationRequestContext,
  patch: TaskMRAutomationPatch,
): Promise<TaskMRAutomationOptions | null> {
  const {
    taskId,
    storeApi,
    updateRequestRef,
    updateSettleCounterRef,
    setOptions,
    setSaving,
    setError,
  } = ctx;
  const requestId = (updateRequestRef.current[taskId] ?? 0) + 1;
  updateRequestRef.current[taskId] = requestId;
  const externalGenAtStart = storeApi.getState().taskMRAutomation.externalGeneration[taskId] ?? 0;
  const previous = storeApi.getState().taskMRAutomation.byTaskId[taskId] ?? null;
  // Optimistic update: apply immediately, revert on failure (AC27).
  if (previous) {
    setOptions(taskId, { ...previous, ...patch });
  }
  setSaving(taskId, true);
  setError(taskId, null);
  try {
    const response = await updateTaskMRAutomation(taskId, patch, { cache: "no-store" });
    const automation = storeApi.getState().taskMRAutomation;
    if (
      isCurrentAndUnchangedExternally(
        automation,
        updateRequestRef.current,
        taskId,
        requestId,
        externalGenAtStart,
      )
    ) {
      setOptions(taskId, response);
    }
    return response;
  } catch (err) {
    if (updateRequestRef.current[taskId] === requestId) {
      const automation = storeApi.getState().taskMRAutomation;
      // Only revert to the pre-optimistic snapshot if nothing fresher has
      // landed externally since — otherwise a failed PATCH would erase a
      // WS-pushed update that arrived while it was in flight.
      const unchangedExternally =
        (automation.externalGeneration[taskId] ?? 0) === externalGenAtStart;
      if (previous && unchangedExternally) {
        setOptions(taskId, previous);
      }
      setError(taskId, errorMessage(err));
    }
    throw err;
  } finally {
    updateSettleCounterRef.current[taskId] = (updateSettleCounterRef.current[taskId] ?? 0) + 1;
    if (updateRequestRef.current[taskId] === requestId) {
      setSaving(taskId, false);
    }
  }
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
 * refresh and update (performRefresh/performUpdate above) track independent
 * per-taskId generation counters so a refresh started while an update's
 * PATCH is still in flight cannot suppress the update's response, or vice
 * versa, for the same task.
 *
 * Three additional races are guarded explicitly:
 *  - A WS push can land while a local refresh()/update() is still in
 *    flight. Both capture taskMRAutomation.externalGeneration[taskId] at
 *    the start and re-check it before committing their own result (a
 *    fresh fetch, or an optimistic-update revert) — a push bumps the
 *    generation, so a slower local response that would otherwise
 *    clobber the pushed value is dropped instead.
 *  - refresh()'s own response is gated on updateSettleCounterRef: a GET
 *    can be in flight when a PATCH starts and resolve afterward with
 *    pre-patch data (the server hadn't processed the PATCH yet when the
 *    GET ran). refresh requires that no update has settled since it
 *    started; update bumps the counter in its `finally` regardless of
 *    success/failure so a stale GET can never flip a just-saved switch
 *    back.
 *  - refresh() also skips committing while an update for the same task
 *    is still saving (not yet settled) — otherwise its pre-patch
 *    response can land mid-PATCH and visibly flip an optimistically-set
 *    switch back off for a moment before the PATCH's own response
 *    corrects it again.
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
    return performRefresh({
      taskId,
      storeApi,
      refreshRequestRef,
      updateRequestRef,
      updateSettleCounterRef,
      setOptions,
      setLoading,
      setSaving,
      setError,
    });
  }, [setError, setLoading, setOptions, setSaving, storeApi, taskId]);

  const update = useCallback(
    async (patch: TaskMRAutomationPatch): Promise<TaskMRAutomationOptions | null> => {
      if (!taskId) return null;
      return performUpdate(
        {
          taskId,
          storeApi,
          refreshRequestRef,
          updateRequestRef,
          updateSettleCounterRef,
          setOptions,
          setLoading,
          setSaving,
          setError,
        },
        patch,
      );
    },
    [setError, setLoading, setOptions, setSaving, storeApi, taskId],
  );

  useEffect(() => {
    if (!taskId || options || loading || error) return;
    void refresh().catch(() => {
      // Error state is stored for the UI; callers can retry via refresh.
    });
  }, [error, loading, options, refresh, taskId]);

  return { options, loading, saving, error, refresh, update };
}
