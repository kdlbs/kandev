"use client";

import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@kandev/ui/tooltip";
import { cn } from "@/lib/utils";

export type WorkflowMessageMetadata = {
  workflow_message?: boolean;
  workflow_auto_start?: boolean;
  workflow_step_id?: string;
  workflow_step_name?: string;
  workflow_step_color?: string;
};

export type WorkflowStepMessageInfo = {
  stepId?: string;
  stepName?: string;
  stepColor?: string;
};

const FALLBACK_STEP_COLOR = "bg-neutral-400";
const WORKFLOW_STEP_COLOR_CLASSES = new Set([
  "bg-slate-500",
  "bg-red-500",
  "bg-orange-500",
  "bg-yellow-500",
  "bg-green-500",
  "bg-emerald-500",
  "bg-cyan-500",
  "bg-blue-500",
  "bg-indigo-500",
  "bg-purple-500",
  FALLBACK_STEP_COLOR,
]);

function safeStepColor(stepColor?: string): string {
  return stepColor && WORKFLOW_STEP_COLOR_CLASSES.has(stepColor) ? stepColor : FALLBACK_STEP_COLOR;
}

export function workflowMessageInfoFromMetadata(
  metadata: WorkflowMessageMetadata | null | undefined,
): WorkflowStepMessageInfo | null {
  if (!metadata?.workflow_message && !metadata?.workflow_auto_start) return null;
  return {
    stepId: metadata.workflow_step_id,
    stepName: metadata.workflow_step_name,
    stepColor: metadata.workflow_step_color,
  };
}

type WorkflowStepMessageBadgeProps = {
  workflow: WorkflowStepMessageInfo;
  size?: "xs" | "sm";
};

export function WorkflowStepMessageBadge({ workflow, size = "sm" }: WorkflowStepMessageBadgeProps) {
  const label = workflow.stepName || "workflow step";
  const sizeClass =
    size === "xs" ? "gap-1 px-1.5 py-0.5 text-[10px]" : "gap-1.5 px-2.5 py-1 text-xs font-medium";
  const dotSize = size === "xs" ? "h-1.5 w-1.5" : "h-2 w-2";
  const stepColor = safeStepColor(workflow.stepColor);
  const badge = (
    <span
      className={cn(
        "inline-flex max-w-full items-center rounded-full bg-muted/60 text-muted-foreground",
        sizeClass,
      )}
      data-testid="workflow-message-badge"
      data-workflow-step-id={workflow.stepId}
    >
      <span
        className={cn("shrink-0 rounded-full", dotSize, stepColor)}
        data-testid="workflow-message-dot"
      />
      <span className="truncate">Workflow: {label}</span>
    </span>
  );

  return (
    <TooltipProvider delayDuration={300}>
      <Tooltip>
        <TooltipTrigger asChild>{badge}</TooltipTrigger>
        <TooltipContent>Workflow step message from {label}</TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}
