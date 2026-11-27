"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { moveTask } from "@/lib/api";
import { ApiError } from "@/lib/api/client";
import type { WorkflowChangePayload } from "@/lib/api/domains/kanban-api";
import type { Task, Workflow } from "@/lib/types/http";
import { useToast } from "@/components/toast-provider";
import { useTranslation } from "react-i18next";
import { taskMatchesWorkflowChange } from "./use-change-workflow-utils";

type WorkflowChangeError = { code?: string; source_profile_id?: string };
type WorkflowChangeRequestSnapshot = {
  workflowId: string;
  stepId: string;
  overrides: Record<string, string>;
};

function apiErrorDetails(error: unknown): WorkflowChangeError {
  if (!(error instanceof ApiError) || !error.body || typeof error.body !== "object") return {};
  const body = error.body as WorkflowChangeError;
  return {
    code: typeof body.code === "string" ? body.code : undefined,
    source_profile_id:
      typeof body.source_profile_id === "string" ? body.source_profile_id : undefined,
  };
}

type SubmitArgs = {
  open: boolean;
  canSubmit: boolean;
  task: Task | null;
  selectedWorkflow: Workflow | null;
  selectedStepId: string;
  workflowChange: WorkflowChangePayload | undefined;
  refreshTask: () => Promise<Task | null>;
  onOpenChange: (open: boolean) => void;
  onSuccess?: () => void;
};

export function useChangeWorkflowSubmit({
  open,
  canSubmit,
  task,
  selectedWorkflow,
  selectedStepId,
  workflowChange,
  refreshTask,
  onOpenChange,
  onSuccess,
}: SubmitArgs) {
  const { t } = useTranslation();
  const { toast } = useToast();
  const pendingRef = useRef(false);
  const lastRequestRef = useRef<WorkflowChangeRequestSnapshot | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [submitError, setSubmitError] = useState<WorkflowChangeError | null>(null);
  const [uncertainResult, setUncertainResult] = useState(false);

  useEffect(() => resetOnOpen(open, lastRequestRef, setSubmitError, setUncertainResult), [open]);

  const submit = useCallback(async () => {
    if (!canSubmit || !task || !selectedWorkflow || !selectedStepId || !workflowChange)
      return false;
    if (pendingRef.current) return false;
    pendingRef.current = true;
    setIsSubmitting(true);
    setSubmitError(null);
    setUncertainResult(false);
    const targetWorkflowId = selectedWorkflow.id;
    const requestOverrides = workflowChange.agent_overrides;
    lastRequestRef.current = {
      workflowId: targetWorkflowId,
      stepId: selectedStepId,
      overrides: { ...requestOverrides },
    };
    try {
      await moveTask(task.id, {
        workflow_id: targetWorkflowId,
        workflow_step_id: selectedStepId,
        workflow_change: workflowChange,
      });
      toast({ title: t("task:changeWorkflowSuccess"), variant: "success" });
      onSuccess?.();
      onOpenChange(false);
      return true;
    } catch (error) {
      return handleSubmitError({
        error,
        workflowId: targetWorkflowId,
        stepId: selectedStepId,
        overrides: requestOverrides,
        refreshTask,
        setSubmitError,
        setUncertainResult,
        toast,
        t,
        onSuccess,
        onOpenChange,
      });
    } finally {
      pendingRef.current = false;
      setIsSubmitting(false);
    }
  }, [
    canSubmit,
    onOpenChange,
    onSuccess,
    refreshTask,
    selectedStepId,
    selectedWorkflow,
    t,
    task,
    toast,
    workflowChange,
  ]);

  const retryAfterRefresh = useCallback(
    () =>
      retryWorkflowChangeAfterRefresh({
        lastRequest: lastRequestRef.current,
        refreshTask,
        setSubmitError,
        setUncertainResult,
        toast,
        t,
        onSuccess,
        onOpenChange,
      }),
    [onOpenChange, onSuccess, refreshTask, t, toast],
  );

  const clearSubmitError = useCallback(() => setSubmitError(null), []);

  return {
    isSubmitting,
    submitError,
    sourceProfileErrorId: submitError?.source_profile_id,
    uncertainResult,
    submit,
    retryAfterRefresh,
    clearSubmitError,
  };
}

async function retryWorkflowChangeAfterRefresh({
  lastRequest,
  refreshTask,
  setSubmitError,
  setUncertainResult,
  toast,
  t,
  onSuccess,
  onOpenChange,
}: {
  lastRequest: WorkflowChangeRequestSnapshot | null;
  refreshTask: () => Promise<Task | null>;
  setSubmitError: (error: WorkflowChangeError | null) => void;
  setUncertainResult: (value: boolean) => void;
  toast: ReturnType<typeof useToast>["toast"];
  t: ReturnType<typeof useTranslation>["t"];
  onSuccess?: () => void;
  onOpenChange: (open: boolean) => void;
}) {
  const refreshed = await refreshTask();
  if (!refreshed) return;
  const matchesLastRequest =
    lastRequest &&
    taskMatchesWorkflowChange(
      refreshed,
      lastRequest.workflowId,
      lastRequest.stepId,
      lastRequest.overrides,
    );
  setUncertainResult(false);
  setSubmitError(null);
  if (!matchesLastRequest) return;
  toast({ title: t("task:changeWorkflowObservedSuccess"), variant: "success" });
  onSuccess?.();
  onOpenChange(false);
}

function resetOnOpen(
  open: boolean,
  lastRequestRef: { current: WorkflowChangeRequestSnapshot | null },
  setSubmitError: (error: WorkflowChangeError | null) => void,
  setUncertainResult: (value: boolean) => void,
) {
  if (!open) return;
  lastRequestRef.current = null;
  setSubmitError(null);
  setUncertainResult(false);
}

async function handleSubmitError({
  error,
  workflowId,
  stepId,
  overrides,
  refreshTask,
  setSubmitError,
  setUncertainResult,
  toast,
  t,
  onSuccess,
  onOpenChange,
}: {
  error: unknown;
  workflowId: string;
  stepId: string;
  overrides: Record<string, string>;
  refreshTask: () => Promise<Task | null>;
  setSubmitError: (error: WorkflowChangeError | null) => void;
  setUncertainResult: (value: boolean) => void;
  toast: ReturnType<typeof useToast>["toast"];
  t: ReturnType<typeof useTranslation>["t"];
  onSuccess?: () => void;
  onOpenChange: (open: boolean) => void;
}): Promise<boolean> {
  const apiError = apiErrorDetails(error);
  if (apiError.code === "workflow_change_conflict") {
    await refreshTask();
    setSubmitError({ code: apiError.code });
    return false;
  }
  if (!(error instanceof ApiError) || error.status >= 500) {
    const latest = await refreshTask();
    if (latest && taskMatchesWorkflowChange(latest, workflowId, stepId, overrides)) {
      toast({ title: t("task:changeWorkflowObservedSuccess"), variant: "success" });
      onSuccess?.();
      onOpenChange(false);
      return true;
    }
    setUncertainResult(true);
    setSubmitError({ code: "workflow_change_uncertain" });
    return false;
  }
  setSubmitError(apiError.code ? apiError : { code: "invalid_workflow_change" });
  return false;
}
