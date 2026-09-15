import { useEffect, useCallback, useState, useRef } from "react";
import { useTranslation } from "react-i18next";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import {
  getTaskPlan,
  createTaskPlan,
  updateTaskPlan,
  deleteTaskPlan,
  listPlanRevisions,
  getPlanRevision,
  revertPlanRevision,
} from "@/lib/api/domains/plan-api";
import { WebSocketRequestError } from "@/lib/ws/client";
import type { TaskPlan, TaskPlanRevision } from "@/lib/types/http";

const EMPTY_REVISIONS: readonly TaskPlanRevision[] = Object.freeze([]);

/** A save-scoped rejection, distinct from the shared `error` slot (see savePlan). */
export type PlanSaveError =
  | { kind: "content-too-large"; limit: number; submitted: number }
  | { kind: "generic"; message: string };

/**
 * Classifies a savePlan failure from the WebSocket rejection's structured
 * `details`, never from re-measuring the draft: the draft is not what was
 * rejected, and by the time the rejection lands it may not be what was
 * submitted either. `reason` is a stable wire token, not user copy.
 */
function classifySaveError(err: unknown, fallbackMessage: string): PlanSaveError {
  if (err instanceof WebSocketRequestError) {
    const details = err.details;
    const limit = details?.limit;
    const submitted = details?.submitted;
    if (
      // i18n-exempt: wire discriminator compared with ===, never localized or rendered.
      details?.reason === "plan_content_too_large" &&
      typeof limit === "number" &&
      Number.isFinite(limit) &&
      typeof submitted === "number" &&
      Number.isFinite(submitted)
    ) {
      return { kind: "content-too-large", limit, submitted };
    }
  }
  return { kind: "generic", message: err instanceof Error ? err.message : fallbackMessage };
}

/** What a save attempt is still allowed to write once it resolves. */
type SaveAttemptGuard = {
  /** True while this attempt is still the latest started FOR ITS OWN TASK,
   * regardless of which task the panel currently points at. Gates the store
   * write (`setTaskPlan`): a save that genuinely succeeded should not be
   * discarded just because the user switched away, but an earlier-started
   * attempt for the SAME task must not clobber a later-started one's result
   * (including a later one that was rejected) with stale content. */
  isLatestForTask: () => boolean;
  /** True while, in addition, the panel is still showing this attempt's
   * task. Gates the displayed `saveError`, which must never be shown for a
   * task the user has since switched away from (AC-003.8). */
  isCurrentAttempt: () => boolean;
};

/**
 * Tracks which savePlan attempt is allowed to write saveError and the plan
 * store, so a stale continuation (task switched underneath it, or a later
 * attempt already resolved) drops its result instead of overwriting a newer
 * one. `beginAttempt` returns the guard the caller checks before writing
 * from either branch.
 */
function useSaveErrorScope(taskId: string | null) {
  const [saveError, setSaveError] = useState<PlanSaveError | null>(null);
  // Assigned in the hook body on every render, not an effect: an effect
  // commits after the render, so a save that resolves in between would
  // compare against the previous task's id in exactly the window this guards.
  const currentTaskIdRef = useRef(taskId);
  // Bumped every time the panel's task identity actually changes, including
  // a return to a task it already showed (A->B->A). isLatestForTask alone
  // survives that round trip unchanged — nothing re-attempted a save on A in
  // between — so without this, an A attempt still in flight when the panel
  // left A can resolve after the panel returns to A and pass isCurrentAttempt,
  // displaying a rejection for a write the user never saw fail. AC-003.8: "a
  // write for the previous task that fails after the change shall not be
  // displayed at all" — not just "not displayed while away from it".
  const taskViewRef = useRef(0);
  if (currentTaskIdRef.current !== taskId) {
    taskViewRef.current += 1;
  }
  currentTaskIdRef.current = taskId;
  // Monotonically increasing; incremented before it is read, so the first
  // attempt is 1 and 0 is never a live attempt. Not reset on task change —
  // it only needs to stay monotonic, and restarting it could let a task-A
  // attempt's number collide with a task-B attempt's.
  const saveAttemptSeqRef = useRef(0);
  // Latest issued sequence PER TASK, not global: a save for task B must not
  // make an in-flight task-A attempt look stale, and switching away from
  // task A and back must not either.
  const latestIssuedSeqByTaskRef = useRef(new Map<string, number>());

  // Reset on task change: a rejection displayed for the previous task must
  // stop being displayed once the panel points at a different task.
  useEffect(() => {
    setSaveError(null);
  }, [taskId]);

  const beginAttempt = useCallback((attemptTaskId: string): SaveAttemptGuard => {
    saveAttemptSeqRef.current += 1;
    const attemptSeq = saveAttemptSeqRef.current;
    const attemptView = taskViewRef.current;
    latestIssuedSeqByTaskRef.current.set(attemptTaskId, attemptSeq);
    // A displayed rejection disappears when the next attempt begins, not
    // when that attempt completes.
    setSaveError(null);
    const isLatestForTask = () =>
      latestIssuedSeqByTaskRef.current.get(attemptTaskId) === attemptSeq;
    return {
      isLatestForTask,
      isCurrentAttempt: () =>
        currentTaskIdRef.current === attemptTaskId &&
        taskViewRef.current === attemptView &&
        isLatestForTask(),
    };
  }, []);

  return { saveError, setSaveError, beginAttempt };
}

type PlanFetchOptions = {
  taskId: string | null;
  visible: boolean;
  connectionStatus: string;
  isLoaded: boolean;
  isLoading: boolean;
  setError: (err: string | null) => void;
};

/**
 * Fetches the plan on load and on becoming visible again (e.g., tab switch).
 * Split out of useTaskPlan purely to keep that hook under the line-count limit.
 */
function usePlanFetch({
  taskId,
  visible,
  connectionStatus,
  isLoaded,
  isLoading,
  setError,
}: PlanFetchOptions) {
  const { t } = useTranslation("task");
  const prevVisibleRef = useRef(visible);
  const setTaskPlan = useAppStore((state) => state.setTaskPlan);
  const setTaskPlanLoading = useAppStore((state) => state.setTaskPlanLoading);
  const markTaskPlanSeen = useAppStore((state) => state.markTaskPlanSeen);

  const fetchPlan = useCallback(async () => {
    if (!taskId) return;

    setTaskPlanLoading(taskId, true);
    setError(null);
    try {
      const fetchedPlan = await getTaskPlan(taskId);
      setTaskPlan(taskId, fetchedPlan);
      // Initial fetch is not a notification — mark as seen so no indicator flashes.
      markTaskPlanSeen(taskId);
    } catch (err) {
      console.error("Failed to fetch task plan:", err);
      setError(err instanceof Error ? err.message : t("task:failedToFetchPlan"));
    } finally {
      setTaskPlanLoading(taskId, false);
    }
  }, [taskId, setTaskPlan, setTaskPlanLoading, markTaskPlanSeen, setError, t]);

  useEffect(() => {
    if (connectionStatus !== "connected") return;
    if (taskId && !isLoaded && !isLoading) {
      fetchPlan();
    }
  }, [taskId, isLoaded, isLoading, fetchPlan, connectionStatus]);

  // Refetch when becoming visible (e.g., tab switch)
  useEffect(() => {
    const wasHidden = !prevVisibleRef.current;
    prevVisibleRef.current = visible;

    // Only refetch when transitioning from hidden to visible
    if (wasHidden && visible && connectionStatus === "connected" && taskId) {
      fetchPlan();
    }
  }, [visible, connectionStatus, taskId, fetchPlan]);

  return fetchPlan;
}

/**
 * Hook to fetch and manage the plan for a task.
 * Plans are task-scoped (one plan per task, shared across all sessions).
 * @param taskId - The task ID to fetch the plan for
 * @param options.visible - When true, refetches the plan (use for tab visibility)
 */
export function useTaskPlan(taskId: string | null, options?: { visible?: boolean }) {
  const { t } = useTranslation("task");
  const { visible = true } = options ?? {};
  const plan = useAppStore((state) => (taskId ? state.taskPlans.byTaskId[taskId] : undefined));
  const isLoading = useAppStore((state) =>
    taskId ? (state.taskPlans.loadingByTaskId[taskId] ?? false) : false,
  );
  const isLoaded = useAppStore((state) =>
    taskId ? (state.taskPlans.loadedByTaskId[taskId] ?? false) : false,
  );
  const isSaving = useAppStore((state) =>
    taskId ? (state.taskPlans.savingByTaskId[taskId] ?? false) : false,
  );
  const setTaskPlan = useAppStore((state) => state.setTaskPlan);
  const setTaskPlanSaving = useAppStore((state) => state.setTaskPlanSaving);
  const connectionStatus = useAppStore((state) => state.connection.status);

  const [error, setError] = useState<string | null>(null);
  const { saveError, setSaveError, beginAttempt } = useSaveErrorScope(taskId);
  const fetchPlan = usePlanFetch({
    taskId,
    visible,
    connectionStatus,
    isLoaded,
    isLoading,
    setError,
  });

  const savePlan = useCallback(
    async (content: string, title?: string): Promise<TaskPlan | null> => {
      if (!taskId) return null;
      const attempt = beginAttempt(taskId);

      setTaskPlanSaving(taskId, true);
      setError(null);
      try {
        let savedPlan: TaskPlan;
        if (plan) {
          // Update existing plan
          savedPlan = await updateTaskPlan(taskId, content, title);
        } else {
          // Create new plan
          savedPlan = await createTaskPlan(taskId, content, title);
        }
        // An earlier-started attempt for this task resolving after a
        // later-started one must not clobber the newer attempt's outcome —
        // including a later attempt's rejection banner and retained draft —
        // with this stale success (AC-003.2).
        if (attempt.isLatestForTask()) {
          setTaskPlan(taskId, savedPlan);
        }
        return savedPlan;
      } catch (err) {
        console.error("Failed to save task plan:", err);
        setError(err instanceof Error ? err.message : t("task:failedToSavePlan"));
        if (attempt.isCurrentAttempt()) {
          setSaveError(classifySaveError(err, t("task:failedToSavePlan")));
        }
        return null;
      } finally {
        setTaskPlanSaving(taskId, false);
      }
    },
    [taskId, plan, setTaskPlan, setTaskPlanSaving, setSaveError, beginAttempt, t],
  );

  const removePlan = useCallback(async (): Promise<boolean> => {
    if (!taskId) return false;

    setTaskPlanSaving(taskId, true);
    setError(null);
    try {
      await deleteTaskPlan(taskId);
      setTaskPlan(taskId, null);
      return true;
    } catch (err) {
      console.error("Failed to delete task plan:", err);
      setError(err instanceof Error ? err.message : t("task:failedToDeletePlan"));
      return false;
    } finally {
      setTaskPlanSaving(taskId, false);
    }
  }, [taskId, setTaskPlan, setTaskPlanSaving, t]);

  const revisionsBundle = useTaskPlanRevisions(taskId, setTaskPlanSaving, setError);

  return {
    plan: plan ?? null,
    isLoading,
    isSaving,
    error,
    saveError,
    savePlan,
    deletePlan: removePlan,
    refetch: fetchPlan,
    ...revisionsBundle,
  };
}

const EMPTY_PAIR: readonly [string | null, string | null] = Object.freeze([
  null,
  null,
]) as readonly [string | null, string | null];

function useTaskPlanRevisions(
  taskId: string | null,
  setTaskPlanSaving: (taskId: string, saving: boolean) => void,
  setError: (err: string | null) => void,
) {
  const { t } = useTranslation("task");
  const revisions = useAppStore((state) =>
    taskId ? (state.taskPlans.revisionsByTaskId[taskId] ?? EMPTY_REVISIONS) : EMPTY_REVISIONS,
  ) as TaskPlanRevision[];
  const isLoadingRevisions = useAppStore((state) =>
    taskId ? (state.taskPlans.revisionsLoadingByTaskId[taskId] ?? false) : false,
  );
  const isRevisionsLoaded = useAppStore((state) =>
    taskId ? (state.taskPlans.revisionsLoadedByTaskId[taskId] ?? false) : false,
  );
  const connectionStatus = useAppStore((state) => state.connection.status);
  const storeApi = useAppStoreApi();
  const setPlanRevisions = useAppStore((state) => state.setPlanRevisions);
  const setPlanRevisionsLoading = useAppStore((state) => state.setPlanRevisionsLoading);
  const cachePlanRevisionContent = useAppStore((state) => state.cachePlanRevisionContent);

  const loadRevisions = useCallback(async () => {
    if (!taskId) return;
    setPlanRevisionsLoading(taskId, true);
    try {
      const list = await listPlanRevisions(taskId);
      setPlanRevisions(taskId, list);
    } catch (err) {
      console.error("Failed to load plan revisions:", err);
      setError(err instanceof Error ? err.message : t("task:failedToLoadPlanRevisions"));
    } finally {
      setPlanRevisionsLoading(taskId, false);
    }
  }, [taskId, setPlanRevisions, setPlanRevisionsLoading, setError, t]);

  // Load revisions once on mount — events may have fired before the WS connected.
  useEffect(() => {
    if (connectionStatus !== "connected") return;
    if (!taskId || isRevisionsLoaded || isLoadingRevisions) return;
    loadRevisions();
  }, [taskId, connectionStatus, isRevisionsLoaded, isLoadingRevisions, loadRevisions]);

  const loadRevisionContent = useCallback(
    async (revisionId: string): Promise<string> => {
      // Read the cache lazily via the store API inside the callback so this
      // function's identity stays stable across cache updates. Selecting the
      // cache object as a hook input would re-create the callback whenever
      // any task's content was cached, which retriggers the dialogs'
      // content-fetch effects (cache short-circuits, but the work is wasted).
      const cached = storeApi.getState().taskPlans.revisionContentCache[revisionId];
      if (cached !== undefined) return cached;
      // Pass taskId so the backend can enforce revision-belongs-to-task.
      const rev = await getPlanRevision(revisionId, taskId ?? undefined);
      const content = rev.content ?? "";
      cachePlanRevisionContent(revisionId, content);
      return content;
    },
    [taskId, storeApi, cachePlanRevisionContent],
  );

  const revertTo = useCallback(
    async (revisionId: string, authorName?: string): Promise<TaskPlanRevision | null> => {
      if (!taskId) return null;
      setTaskPlanSaving(taskId, true);
      setError(null);
      try {
        return await revertPlanRevision(taskId, revisionId, authorName);
      } catch (err) {
        console.error("Failed to revert plan:", err);
        setError(err instanceof Error ? err.message : t("task:failedToRevertPlan"));
        return null;
      } finally {
        setTaskPlanSaving(taskId, false);
      }
    },
    [taskId, setTaskPlanSaving, setError, t],
  );

  return {
    revisions,
    isLoadingRevisions,
    loadRevisions,
    loadRevisionContent,
    revertTo,
    ...usePreviewCompareState(taskId),
  };
}

/** Phase 6: preview + compare selectors and actions, scoped to the active task. */
function usePreviewCompareState(taskId: string | null) {
  const previewRevisionId = useAppStore((state) =>
    taskId ? (state.taskPlans.previewRevisionIdByTaskId[taskId] ?? null) : null,
  );
  const comparePair = useAppStore((state) =>
    taskId ? (state.taskPlans.comparePairByTaskId[taskId] ?? EMPTY_PAIR) : EMPTY_PAIR,
  ) as [string | null, string | null];
  const setPreviewRevisionStore = useAppStore((state) => state.setPreviewRevision);
  const toggleComparePairStore = useAppStore((state) => state.toggleComparePair);
  const clearComparePairStore = useAppStore((state) => state.clearComparePair);

  const setPreviewRevision = useCallback(
    (revisionId: string | null) => {
      if (!taskId) return;
      setPreviewRevisionStore(taskId, revisionId);
    },
    [taskId, setPreviewRevisionStore],
  );
  const toggleCompareSelection = useCallback(
    (revisionId: string) => {
      if (!taskId) return;
      toggleComparePairStore(taskId, revisionId);
    },
    [taskId, toggleComparePairStore],
  );
  const clearComparePair = useCallback(() => {
    if (!taskId) return;
    clearComparePairStore(taskId);
  }, [taskId, clearComparePairStore]);

  return {
    previewRevisionId,
    setPreviewRevision,
    comparePair,
    toggleCompareSelection,
    clearComparePair,
  };
}
