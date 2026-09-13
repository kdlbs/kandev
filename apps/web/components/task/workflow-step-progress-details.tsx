"use client";

import { useTranslation } from "react-i18next";
import type {
  WorkflowStepProgress,
  WorkflowStepProgressStatus,
} from "@/hooks/domains/kanban/use-workflow-step-progress";

const EMPTY_AGENT_LABELS_BY_PROFILE_ID: Readonly<Record<string, string>> = {};

export function StepProgressDetails({
  progress,
  agentProfileId,
  agentLabelsByProfileId = EMPTY_AGENT_LABELS_BY_PROFILE_ID,
  testId,
}: {
  progress: WorkflowStepProgress;
  agentProfileId?: string;
  agentLabelsByProfileId?: Readonly<Record<string, string>>;
  testId?: string;
}) {
  const { t } = useTranslation();
  if (progress.status === "idle") return null;

  const agentLabel =
    progress.agentLabel ?? (agentProfileId ? agentLabelsByProfileId[agentProfileId] : undefined);

  return (
    <div
      data-testid={testId}
      role="status"
      aria-live="polite"
      className="flex min-w-0 items-center gap-1.5 pl-4 text-[11px] text-muted-foreground"
    >
      <span className="truncate">{t(workflowStepProgressTranslationKey(progress.status))}</span>
      {agentLabel && <span className="truncate text-foreground/75">{agentLabel}</span>}
    </div>
  );
}

export function workflowStepProgressTranslationKey(status: WorkflowStepProgressStatus) {
  switch (status) {
    case "moving":
      return "task:workflowStepProgressMoving";
    case "preparing":
      return "task:workflowStepProgressPreparing";
    case "starting":
      return "task:workflowStepProgressStarting";
    case "cancelling":
      return "task:workflowStepProgressCancelling";
    case "running":
      return "task:workflowStepProgressRunning";
    case "waiting":
      return "task:workflowStepProgressWaiting";
    case "idle_agent":
      return "task:workflowStepProgressIdle";
    case "not_started":
      return "task:workflowStepProgressNotStarted";
    case "completed":
      return "task:workflowStepProgressCompleted";
    case "failed":
      return "task:workflowStepProgressFailed";
    case "cancelled":
      return "task:workflowStepProgressCancelled";
    case "idle":
      return "task:workflowStepProgressNotStarted";
  }
}
