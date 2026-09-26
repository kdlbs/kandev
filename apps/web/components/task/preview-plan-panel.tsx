"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { Button } from "@kandev/ui/button";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { PanelLoadingState } from "@/components/panel-loading-state";
import { getTaskPlan } from "@/lib/api/domains/plan-api";
import { PlanReadOnlyMarkdown } from "@/components/editors/tiptap/tiptap-plan-readonly";
import type { TaskPlan } from "@/lib/types/http-agents";
import { useTranslation } from "react-i18next";

/**
 * Fetches and reads the task plan for the kanban preview panel without
 * marking it seen. Mirrors `LazyPlanPreview`'s lazy-load path rather than
 * `useTaskPlan`, whose initial fetch marks the plan seen by design — that
 * would clear the Plan tab's unseen indicator before the user ever looks.
 *
 * `failed` is local component state (not the shared store) so a rejected
 * fetch stops the effect instead of retrying every render. A reconnect or
 * explicit retry clears the error and lets the effect fetch again. Call this
 * once per preview panel —
 * `usePreviewPlanTab` owns it and passes the result down to
 * `PreviewPlanPanel` — a second mount would duplicate the fetch.
 */
export function usePreviewPlanSummary(taskId: string) {
  const plan = useAppStore((state) => state.taskPlans.byTaskId[taskId] ?? null);
  const loaded = useAppStore((state) => state.taskPlans.loadedByTaskId[taskId] ?? false);
  const loading = useAppStore((state) => state.taskPlans.loadingByTaskId[taskId] ?? false);
  const connectionStatus = useAppStore((state) => state.connection.status);
  const storeApi = useAppStoreApi();
  const [failedTaskId, setFailedTaskId] = useState<string | null>(null);
  const failed = failedTaskId === taskId;
  const retry = useCallback(() => {
    setFailedTaskId((current) => (current === taskId ? null : current));
  }, [taskId]);
  const previousConnectionStatus = useRef(connectionStatus);

  useEffect(() => {
    const reconnected =
      previousConnectionStatus.current !== "connected" && connectionStatus === "connected";
    previousConnectionStatus.current = connectionStatus;
    if (reconnected) retry();
  }, [connectionStatus, retry]);

  useEffect(() => {
    if (loaded && failedTaskId === taskId) setFailedTaskId(null);
  }, [failedTaskId, loaded, taskId]);

  useEffect(() => {
    if (connectionStatus !== "connected" || failed) return;
    const currentPlanState = storeApi.getState().taskPlans;
    if (currentPlanState.loadedByTaskId[taskId] || currentPlanState.loadingByTaskId[taskId]) return;
    const { setTaskPlanLoading } = storeApi.getState();
    setTaskPlanLoading(taskId, true);
    getTaskPlan(taskId)
      .then((result) => {
        // Race guard: a WS `task.plan.created`/`task.plan.updated`/
        // `task.plan.deleted` push can land in the store while this WebSocket
        // request is in flight. `loadedByTaskId` (not `byTaskId` truthiness)
        // tells us whether the store already holds an authoritative entry —
        // a deleted plan is stored as `null` on purpose, and that tombstone
        // is a real entry, not "nothing arrived yet". Once an entry exists,
        // only ever move forward in time: never resurrect a tombstone, and
        // never replace a real plan with `null` or an older version.
        const state = storeApi.getState();
        const hasLiveEntry = state.taskPlans.loadedByTaskId[taskId] ?? false;
        const live = state.taskPlans.byTaskId[taskId] ?? null;
        if (hasLiveEntry) {
          if (live === null) return;
          if (result === null) return;
          if (Date.parse(result.updated_at) < Date.parse(live.updated_at)) return;
        }
        storeApi.getState().setTaskPlan(taskId, result);
      })
      .catch(() => {
        setFailedTaskId(taskId);
      })
      .finally(() => {
        storeApi.getState().setTaskPlanLoading(taskId, false);
      });
  }, [connectionStatus, failed, storeApi, taskId]);

  return { plan, loaded, loading, failed: failed && !loaded, retry };
}

type PreviewPlanPanelProps = {
  plan: TaskPlan | null;
  loaded: boolean;
  loading: boolean;
  failed: boolean;
  onRetry: () => void;
};

/** Read-only plan render for the kanban preview panel's Plan tab. */
export function PreviewPlanPanel({
  plan,
  loaded,
  loading,
  failed,
  onRetry,
}: PreviewPlanPanelProps) {
  const { t } = useTranslation();

  if (failed) {
    return (
      <div
        className="flex h-full flex-col items-center justify-center gap-3 text-sm text-muted-foreground"
        data-testid="preview-plan-error-state"
      >
        <span>{t("task:planLoadError")}</span>
        <Button
          type="button"
          variant="outline"
          className="min-h-11 min-w-11 cursor-pointer [@media(pointer:fine)]:min-h-0 [@media(pointer:fine)]:min-w-0"
          onClick={onRetry}
          data-testid="preview-plan-retry"
        >
          {t("task:retry")}
        </Button>
      </div>
    );
  }

  if (loading || !loaded) {
    return <PanelLoadingState testId="preview-plan-loading-state" label={t("task:loadingPlan")} />;
  }

  if (!plan?.content) {
    return (
      <div
        className="flex h-full items-center justify-center text-sm text-muted-foreground"
        data-testid="preview-plan-empty-state"
      >
        {t("task:planIsEmpty")}
      </div>
    );
  }

  return (
    <div className="h-full min-h-0 overflow-y-auto p-4" data-testid="preview-plan-panel">
      <PlanReadOnlyMarkdown content={plan.content} testId="preview-plan-content" />
    </div>
  );
}
