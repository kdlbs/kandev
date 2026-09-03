"use client";

import { useEffect, useState } from "react";
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
 * fetch stops the effect instead of retrying every render; it resets when
 * `taskId` changes, allowing one attempt per task. Call this once per
 * preview panel — `usePreviewPlanTab` owns it and passes the result down to
 * `PreviewPlanPanel` — a second mount would duplicate the fetch.
 */
export function usePreviewPlanSummary(taskId: string) {
  const plan = useAppStore((state) => state.taskPlans.byTaskId[taskId] ?? null);
  const loaded = useAppStore((state) => state.taskPlans.loadedByTaskId[taskId] ?? false);
  const loading = useAppStore((state) => state.taskPlans.loadingByTaskId[taskId] ?? false);
  const storeApi = useAppStoreApi();
  const [failedTaskId, setFailedTaskId] = useState<string | null>(null);
  const failed = failedTaskId === taskId;

  useEffect(() => {
    if (loaded || loading || failed) return;
    const { setTaskPlanLoading } = storeApi.getState();
    setTaskPlanLoading(taskId, true);
    getTaskPlan(taskId)
      .then((result) => {
        storeApi.getState().setTaskPlan(taskId, result);
      })
      .catch(() => {
        storeApi.getState().setTaskPlanLoading(taskId, false);
        setFailedTaskId(taskId);
      });
  }, [taskId, loaded, loading, failed, storeApi]);

  return { plan, loaded, loading, failed };
}

type PreviewPlanPanelProps = {
  plan: TaskPlan | null;
  loaded: boolean;
  loading: boolean;
  failed: boolean;
};

/** Read-only plan render for the kanban preview panel's Plan tab. */
export function PreviewPlanPanel({ plan, loaded, loading, failed }: PreviewPlanPanelProps) {
  const { t } = useTranslation();

  if (failed) {
    return (
      <div
        className="flex h-full items-center justify-center text-sm text-muted-foreground"
        data-testid="preview-plan-error-state"
      >
        {t("task:planLoadError")}
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
