"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import {
  getTaskCompletionGate,
  listTaskCompletionGateHistory,
  type TaskCompletionGateHistory,
  type TaskCompletionGateSnapshot,
} from "@/lib/api/domains/task-completion-gates-api";
import {
  usePresentationToken,
  useWorkflowStepMove,
} from "@/hooks/domains/kanban/use-workflow-step-move";
import type { WorkflowStepperStep } from "@/components/task/workflow-step-disclosure";
import { TaskCompletionGateSurface } from "./task-completion-gate-surface";

type TaskCompletionGateRowProps = {
  taskId: string;
  taskUpdatedAt: string;
  workflowId: string;
  workflowStepId: string;
  taskState: string;
  workflowSteps: WorkflowStepperStep[];
  isMobile: boolean;
};

export function TaskCompletionGateRow({
  taskId,
  taskUpdatedAt,
  workflowId,
  workflowStepId,
  taskState,
  workflowSteps,
  isMobile,
}: TaskCompletionGateRowProps) {
  const { t } = useTranslation();
  const [snapshot, setSnapshot] = useState<TaskCompletionGateSnapshot | null>(null);
  const [history, setHistory] = useState<TaskCompletionGateHistory[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [open, setOpen] = useState(false);
  const completionSteps = useMemo(
    () => workflowSteps.filter((step) => step.complete_task_on_enter),
    [workflowSteps],
  );
  const presentationToken = usePresentationToken(taskId);
  const { handleMove, movingToStepId } = useWorkflowStepMove({
    taskId,
    workflowId,
    currentStepId: workflowStepId,
    taskState,
    presentationToken,
  });

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const [nextSnapshot, nextHistory] = await Promise.all([
        getTaskCompletionGate(taskId),
        listTaskCompletionGateHistory(taskId),
      ]);
      setSnapshot(nextSnapshot);
      setHistory(nextHistory);
      setError(null);
    } catch {
      setError(t("task:completionGateLoadError"));
    } finally {
      setLoading(false);
    }
  }, [taskId, taskUpdatedAt, t]);

  useEffect(() => {
    setSnapshot(null);
    setHistory([]);
    void load();
  }, [load]);

  const submitOverride = useCallback(
    async (stepId: string, reason: string, revision: number) => {
      setError(null);
      const moved = await handleMove(stepId, undefined, { expected_revision: revision, reason });
      if (!moved) {
        setError(t("task:completionGateMoveError"));
        return false;
      }
      setOpen(false);
      await load();
      return true;
    },
    [handleMove, load, t],
  );

  return (
    <section
      className="flex shrink-0 items-center justify-between gap-3 border-b border-border px-4 py-2"
      data-testid="task-completion-gate-row"
    >
      <TaskCompletionGateStatus loading={loading} snapshot={snapshot} error={error} />
      <Button
        type="button"
        variant="outline"
        className="min-h-11 shrink-0 cursor-pointer"
        disabled={loading || movingToStepId !== null}
        onClick={() => setOpen(true)}
        data-testid="task-completion-gate-open"
      >
        {t("task:completionGateInspect")}
      </Button>
      <TaskCompletionGateSurface
        taskId={taskId}
        taskUpdatedAt={taskUpdatedAt}
        isMobile={isMobile}
        open={open}
        snapshot={snapshot}
        history={history}
        completionSteps={completionSteps}
        error={error}
        onOpenChange={setOpen}
        onRefresh={load}
        onOverride={submitOverride}
      />
    </section>
  );
}

function TaskCompletionGateStatus({
  loading,
  snapshot,
  error,
}: {
  loading: boolean;
  snapshot: TaskCompletionGateSnapshot | null;
  error: string | null;
}) {
  const { t } = useTranslation();
  const criterionCount = snapshot?.criteria.length ?? 0;
  const verifiedCount =
    snapshot?.criteria.filter(
      (criterion) =>
        criterion.verified_revision === criterion.criterion_revision && criterion.evidence != null,
    ).length ?? 0;
  let status = t("task:completionGateClear");
  if (loading) status = t("task:completionGateLoading");
  else if (snapshot?.blocked) {
    status = t("task:completionGateBlockedSummary", { count: snapshot.blockers?.length ?? 0 });
  }

  return (
    <div className="flex min-w-0 flex-col">
      <span className="text-xs text-muted-foreground">{t("task:completionGateLabel")}</span>
      <span className="truncate text-sm font-medium" data-testid="task-completion-gate-status">
        {status}
        {!loading &&
          snapshot &&
          ` · ${t("task:completionGateProgress", { verified: verifiedCount, total: criterionCount })}`}
      </span>
      {error && (
        <span
          className="text-xs text-destructive"
          role="alert"
          data-testid="task-completion-gate-row-error"
        >
          {error}
        </span>
      )}
    </div>
  );
}
