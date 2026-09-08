"use client";

import { useCallback, useEffect, type RefObject } from "react";
import { getTaskMRAutomation, updateTaskMRAutomation } from "@/lib/api/domains/gitlab-api";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import type { AppState } from "@/lib/state/store";
import type {
  TaskMRAutomationOptions,
  TaskMRAutomationOptionsForMR,
  TaskMRAutomationPatch,
} from "@/lib/types/gitlab";
import { t } from "@/lib/i18n";

type AppStoreApi = ReturnType<typeof useAppStoreApi>;

type MRAutomationRequestRefs = Pick<
  MRAutomationRequestContext,
  "refreshRequestRef" | "updateRequestRef" | "updateSettleCounterRef"
>;

// One task can mount a control for every linked MR. Those controls share a
// store, so they must also share request ordering: an older control's full
// response must not overwrite a newer control's response for another MR.
const requestRefsByStore = new WeakMap<AppStoreApi, MRAutomationRequestRefs>();

function requestRefsForStore(storeApi: AppStoreApi): MRAutomationRequestRefs {
  const existing = requestRefsByStore.get(storeApi);
  if (existing) return existing;
  const refs: MRAutomationRequestRefs = {
    refreshRequestRef: { current: {} },
    updateRequestRef: { current: {} },
    updateSettleCounterRef: { current: {} },
  };
  requestRefsByStore.set(storeApi, refs);
  return refs;
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : t("gitlab:failedToLoadMrAutomationOptions");
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
  // A task with several linked MRs mounts one MRAutomationControls per MR,
  // and every instance's mount effect sees the same pre-fetch render snapshot
  // (options null, loading false) — so without this guard, opening the
  // dropdown would fire one identical GET per linked MR. The in-flight
  // request commits to the shared store slot that all of them read.
  if (storeApi.getState().taskMRAutomation.loading[taskId]) {
    return storeApi.getState().taskMRAutomation.byTaskId[taskId] ?? null;
  }
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

/**
 * Applies a patch to the cached options the way the backend will. The five
 * switches live per-MR in `mr_options`, so a naive `{...previous, ...patch}`
 * would write them onto the task-level *aggregate* instead — showing the
 * switch as on for every linked MR until the response landed. Switch fields
 * are therefore merged into the targeted MR's entry (or every entry, when the
 * patch names no MR, which is the fan-out the backend performs); the
 * remaining task-level fields merge at the top level as before.
 */
function applyMRAutomationPatchOptimistically(
  previous: TaskMRAutomationOptions,
  patch: TaskMRAutomationPatch,
): TaskMRAutomationOptions {
  const {
    repository_id: repositoryID,
    project_path: projectPath,
    mr_iid: mrIID,
    ...fields
  } = patch;
  const switchKeys = [
    "auto_fix_enabled",
    "auto_merge_enabled",
    "prompt_on_review_requested",
    "prompt_on_merged",
    "prompt_on_closed",
  ] as const;
  const switchPatch: Partial<TaskMRAutomationOptionsForMR> = {};
  const taskLevel: Partial<TaskMRAutomationOptions> = {};
  for (const [key, value] of Object.entries(fields)) {
    if ((switchKeys as readonly string[]).includes(key)) {
      Object.assign(switchPatch, { [key]: value });
    } else {
      Object.assign(taskLevel, { [key]: value });
    }
  }
  if (Object.keys(switchPatch).length === 0) {
    return { ...previous, ...taskLevel };
  }
  const targetsOneMR =
    repositoryID !== undefined && projectPath !== undefined && mrIID !== undefined;
  const mrOptions = (previous.mr_options ?? []).map((option) => {
    const isTarget =
      !targetsOneMR ||
      (option.repository_id === repositoryID &&
        option.project_path === projectPath &&
        option.mr_iid === mrIID);
    return isTarget ? { ...option, ...switchPatch } : option;
  });
  return { ...previous, ...taskLevel, mr_options: mrOptions };
}

type UpdateResponseCommitContext = {
  automation: AppState["taskMRAutomation"];
  updateRequestRef: Record<string, number>;
  taskId: string;
  requestId: number;
  externalGenAtStart: number;
  response: TaskMRAutomationOptions;
};

function shouldCommitUpdateResponse({
  automation,
  updateRequestRef,
  taskId,
  requestId,
  externalGenAtStart,
  response,
}: UpdateResponseCommitContext): boolean {
  const currentRevision = automation.byTaskId[taskId]?.automation_revision ?? 0;
  const responseRevision = response.automation_revision ?? 0;
  const isLatestRequest = updateRequestRef[taskId] === requestId;
  const isNewerRevision = responseRevision > currentRevision;
  return (
    (automation.externalGeneration[taskId] ?? 0) === externalGenAtStart &&
    (isLatestRequest || isNewerRevision)
  );
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
    setOptions(taskId, applyMRAutomationPatchOptimistically(previous, patch));
  }
  setSaving(taskId, true);
  setError(taskId, null);
  try {
    const response = await updateTaskMRAutomation(taskId, patch, { cache: "no-store" });
    const automation = storeApi.getState().taskMRAutomation;
    // Request-start order is not server revision order. The helper allows an
    // older request through only when its server revision is strictly newer;
    // the store then applies its monotonic revision guard.
    if (
      shouldCommitUpdateResponse({
        automation,
        updateRequestRef: updateRequestRef.current,
        taskId,
        requestId,
        externalGenAtStart,
        response,
      })
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
  const storeApi = useAppStoreApi();

  const { refreshRequestRef, updateRequestRef, updateSettleCounterRef } =
    requestRefsForStore(storeApi);
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

  // Clears the per-task auto-fix prompt override, restoring the default
  // (editable) template — an explicit empty string, not undefined, since
  // the backend treats undefined as "field not sent" and empty string as
  // "restore default" (AC5).
  const resetPrompt = useCallback(
    async (): Promise<TaskMRAutomationOptions | null> => update({ auto_fix_prompt_override: "" }),
    [update],
  );

  return { options, loading, saving, error, refresh, update, resetPrompt };
}
