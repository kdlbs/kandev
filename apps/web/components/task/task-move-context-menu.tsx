"use client";

import { IconArrowRight, IconLogicBuffer } from "@tabler/icons-react";
import {
  ContextMenuItem,
  ContextMenuSeparator,
  ContextMenuSub,
  ContextMenuSubContent,
  ContextMenuSubTrigger,
} from "@kandev/ui/context-menu";
import { cn } from "@/lib/utils";

export type TaskMoveStep = {
  id: string;
  title: string;
  color?: string | null;
  events?: { on_enter?: Array<{ type: string; config?: Record<string, unknown> }> };
};

export type TaskMoveWorkflow = {
  id: string;
  name: string;
  hidden?: boolean;
};

type TaskMoveContextMenuItemsProps = {
  currentWorkflowId?: string | null;
  currentStepId?: string | null;
  workflows: TaskMoveWorkflow[];
  stepsByWorkflowId: Record<string, TaskMoveStep[]>;
  disabled?: boolean;
  showSeparator?: boolean;
  onMoveToStep?: (stepId: string) => void;
  onSendToWorkflow?: (workflowId: string, stepId: string) => void;
};

export function stepHasAutoStart(step: TaskMoveStep) {
  return step.events?.on_enter?.some((action) => action.type === "auto_start_agent") ?? false;
}

function StepMenuItem({
  step,
  currentStepId,
  onSelect,
}: {
  step: TaskMoveStep;
  currentStepId?: string | null;
  onSelect: (stepId: string) => void;
}) {
  const isCurrent = step.id === currentStepId;
  const hasAutoStart = stepHasAutoStart(step);
  return (
    <ContextMenuItem
      key={step.id}
      data-testid={`task-context-step-${step.id}`}
      disabled={isCurrent}
      onSelect={(event) => {
        event.preventDefault();
        if (!isCurrent) onSelect(step.id);
      }}
    >
      <span className={cn("block h-2 w-2 rounded-full shrink-0", step.color ?? "")} />
      <span className="flex-1 truncate">{step.title}</span>
      {isCurrent && (
        <span
          data-testid={`task-context-step-current-${step.id}`}
          className="ml-auto text-[10px] text-muted-foreground"
        >
          Current
        </span>
      )}
      {hasAutoStart && (
        <span
          data-testid={`task-context-step-autostart-${step.id}`}
          className="ml-auto text-[10px] text-muted-foreground"
        >
          Auto-start
        </span>
      )}
    </ContextMenuItem>
  );
}

function MoveToCurrentWorkflowSubmenu({
  steps,
  currentStepId,
  disabled,
  onMoveToStep,
}: {
  steps: TaskMoveStep[];
  currentStepId?: string | null;
  disabled?: boolean;
  onMoveToStep?: (stepId: string) => void;
}) {
  if (!onMoveToStep || steps.length <= 1) return null;
  return (
    <ContextMenuSub>
      <ContextMenuSubTrigger data-testid="task-context-move-to" disabled={disabled}>
        <IconArrowRight className="mr-2 h-4 w-4" />
        Move to
      </ContextMenuSubTrigger>
      <ContextMenuSubContent className="w-48">
        {steps.map((step) => (
          <StepMenuItem
            key={step.id}
            step={step}
            currentStepId={currentStepId}
            onSelect={onMoveToStep}
          />
        ))}
      </ContextMenuSubContent>
    </ContextMenuSub>
  );
}

function WorkflowTargetItem({
  workflow,
  steps,
  disabled,
  onSendToWorkflow,
}: {
  workflow: TaskMoveWorkflow;
  steps: TaskMoveStep[];
  disabled?: boolean;
  onSendToWorkflow?: (workflowId: string, stepId: string) => void;
}) {
  if (steps.length === 0 || !onSendToWorkflow) {
    return (
      <ContextMenuItem
        data-testid={`task-context-workflow-${workflow.id}`}
        disabled
        aria-disabled="true"
      >
        <span className="flex-1 truncate">{workflow.name}</span>
        <span data-testid="task-context-disabled-reason" className="ml-2 text-[10px]">
          No steps
        </span>
      </ContextMenuItem>
    );
  }

  return (
    <ContextMenuSub>
      <ContextMenuSubTrigger
        data-testid={`task-context-workflow-${workflow.id}`}
        disabled={disabled}
      >
        <span className="truncate">{workflow.name}</span>
      </ContextMenuSubTrigger>
      <ContextMenuSubContent className="w-48">
        {steps.map((step) => (
          <StepMenuItem
            key={step.id}
            step={step}
            onSelect={(stepId) => onSendToWorkflow(workflow.id, stepId)}
          />
        ))}
      </ContextMenuSubContent>
    </ContextMenuSub>
  );
}

function SendToWorkflowSubmenu({
  currentWorkflowId,
  workflows,
  stepsByWorkflowId,
  disabled,
  onSendToWorkflow,
}: {
  currentWorkflowId?: string | null;
  workflows: TaskMoveWorkflow[];
  stepsByWorkflowId: Record<string, TaskMoveStep[]>;
  disabled?: boolean;
  onSendToWorkflow?: (workflowId: string, stepId: string) => void;
}) {
  const targets = workflows.filter((workflow) => workflow.id !== currentWorkflowId);
  if (!onSendToWorkflow || !currentWorkflowId || targets.length === 0) return null;
  return (
    <ContextMenuSub>
      <ContextMenuSubTrigger data-testid="task-context-send-to-workflow" disabled={disabled}>
        <IconLogicBuffer className="mr-2 h-4 w-4" />
        Send to workflow
      </ContextMenuSubTrigger>
      <ContextMenuSubContent className="w-56">
        {targets.map((workflow) => (
          <WorkflowTargetItem
            key={workflow.id}
            workflow={workflow}
            steps={stepsByWorkflowId[workflow.id] ?? []}
            disabled={disabled}
            onSendToWorkflow={onSendToWorkflow}
          />
        ))}
      </ContextMenuSubContent>
    </ContextMenuSub>
  );
}

export function TaskMoveContextMenuItems({
  currentWorkflowId,
  currentStepId,
  workflows,
  stepsByWorkflowId,
  disabled,
  showSeparator = true,
  onMoveToStep,
  onSendToWorkflow,
}: TaskMoveContextMenuItemsProps) {
  const visibleWorkflows = workflows.filter((workflow) => !workflow.hidden);
  const currentSteps = currentWorkflowId ? (stepsByWorkflowId[currentWorkflowId] ?? []) : [];
  const hasSameWorkflowMove = Boolean(onMoveToStep && currentSteps.length > 1);
  const hasCrossWorkflowMove = Boolean(
    onSendToWorkflow &&
    currentWorkflowId &&
    visibleWorkflows.some((workflow) => workflow.id !== currentWorkflowId),
  );

  if (!hasSameWorkflowMove && !hasCrossWorkflowMove) return null;

  return (
    <>
      {showSeparator && <ContextMenuSeparator />}
      <MoveToCurrentWorkflowSubmenu
        steps={currentSteps}
        currentStepId={currentStepId}
        disabled={disabled}
        onMoveToStep={onMoveToStep}
      />
      <SendToWorkflowSubmenu
        currentWorkflowId={currentWorkflowId}
        workflows={visibleWorkflows}
        stepsByWorkflowId={stepsByWorkflowId}
        disabled={disabled}
        onSendToWorkflow={onSendToWorkflow}
      />
    </>
  );
}
