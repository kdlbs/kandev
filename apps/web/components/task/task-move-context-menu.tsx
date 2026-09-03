"use client";

import { useRef, useState } from "react";
import { IconAdjustments, IconArrowRight, IconLogicBuffer } from "@tabler/icons-react";
import {
  ContextMenuItem,
  ContextMenuSeparator,
  ContextMenuSub,
  ContextMenuSubContent,
  ContextMenuSubTrigger,
} from "@kandev/ui/context-menu";
import { cn } from "@/lib/utils";
import { useTranslation } from "react-i18next";
import { useWorkflowMove } from "@/hooks/domains/kanban/use-workflow-move";
import { useToast } from "@/components/toast-provider";
import {
  WorkflowMoveDialog,
  WorkflowMoveOptions,
  WorkflowMoveOptionsForm,
  type WorkflowMoveOptionsSubmit,
} from "./workflow-move-options";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import type { WorkflowMoveEntryOptions } from "@/lib/api/domains/kanban-api";

export type TaskMoveStep = {
  id: string;
  title: string;
  task_id?: string | null;
  color?: string | null;
  workflow_id?: string | null;
  agent_profile_id?: string | null;
  events?: { on_enter?: Array<{ type: string; config?: Record<string, unknown> }> };
};

export type TaskMoveWorkflow = {
  id: string;
  name: string;
  hidden?: boolean;
};

export function useTaskMoveOptions({
  taskId,
  workflowId,
  steps,
  closeMenu,
}: {
  taskId: string;
  workflowId?: string | null;
  steps?: TaskMoveStep[];
  closeMenu?: () => void;
}) {
  const [moveOptionsStep, setMoveOptionsStep] = useState<TaskMoveStep | null>(null);
  const { move, isMoving } = useWorkflowMove();
  const { toast } = useToast();
  const { t } = useTranslation();

  const openMoveOptionsStep = (targetStep: TaskMoveStep) => {
    setMoveOptionsStep({ ...targetStep, task_id: targetStep.task_id ?? taskId });
  };

  const openMoveOptions = (targetStepId: string) => {
    const targetStep = steps?.find((step) => step.id === targetStepId);
    if (!targetStep) return;
    openMoveOptionsStep({ ...targetStep, workflow_id: workflowId ?? undefined });
  };

  const runMove = async (
    targetStepId: string,
    entryOptions: WorkflowMoveEntryOptions | undefined,
  ) => {
    if (!workflowId) return false;
    const result = await move(taskId, {
      workflow_id: workflowId,
      workflow_step_id: targetStepId,
      position: 0,
      entry_options: entryOptions,
    });
    if (result.disposition === "failed") {
      const error = result.error;
      toast({
        title: t("task:failedToMoveTask"),
        description: error instanceof Error ? error.message : t("task:failedToMoveTask"),
        variant: "error",
      });
      return false;
    }
    closeMenu?.();
    return true;
  };

  const submitMoveOptions = async (entryOptions: WorkflowMoveEntryOptions | undefined) => {
    if (!moveOptionsStep) return false;
    const ok = await runMove(moveOptionsStep.id, entryOptions);
    if (ok) setMoveOptionsStep(null);
    return ok;
  };

  // Move a specific step directly, used by the fine-pointer inline options form
  // rendered in the "Move to" submenu (no intermediate dialog/drawer surface).
  const submitMoveOptionsForStep = (
    targetStepId: string,
    entryOptions: WorkflowMoveEntryOptions | undefined,
  ) => runMove(targetStepId, entryOptions);

  return {
    moveOptionsStep,
    isMoving,
    openMoveOptions,
    openMoveOptionsStep,
    submitMoveOptions,
    submitMoveOptionsForStep,
    closeMoveOptions: () => {
      setMoveOptionsStep(null);
    },
  };
}

export function TaskMoveOptionsSurface({
  step,
  isMoving,
  onClose,
  onSubmit,
}: {
  step: TaskMoveStep | null;
  isMoving: boolean;
  onClose: () => void;
  onSubmit: WorkflowMoveOptionsSubmit;
}) {
  const usesTouchDrawer = useTouchDrawer();
  if (!step) return null;
  const optionsProps = {
    open: true,
    onOpenChange: (open: boolean) => {
      if (!open) onClose();
    },
    targetStepName: step.title,
    isMoving,
    onSubmit,
  };
  const Options = usesTouchDrawer ? WorkflowMoveOptions : WorkflowMoveDialog;
  return <Options {...optionsProps} />;
}

type TaskMoveContextMenuItemsProps = {
  currentWorkflowId?: string | null;
  currentStepId?: string | null;
  workflows: TaskMoveWorkflow[];
  stepsByWorkflowId: Record<string, TaskMoveStep[]>;
  disabled?: boolean;
  showSeparator?: boolean;
  onMoveToStep?: (stepId: string) => void;
  onMoveToStepWithOptions?: (stepId: string) => void;
  onSubmitWithOptions?: (
    stepId: string,
    entryOptions: WorkflowMoveEntryOptions | undefined,
  ) => Promise<boolean>;
  isMoving?: boolean;
  onSendToWorkflow?: (workflowId: string, stepId: string) => void;
};

export function stepHasAutoStart(step: TaskMoveStep) {
  return step.events?.on_enter?.some((action) => action.type === "auto_start_agent") ?? false;
}

function StepMenuLabel({
  step,
  isCurrent,
  testIdPrefix,
}: {
  step: TaskMoveStep;
  isCurrent: boolean;
  testIdPrefix: string;
}) {
  const { t } = useTranslation();
  const hasAutoStart = stepHasAutoStart(step);
  return (
    <>
      <span className={cn("block h-2 w-2 rounded-full shrink-0", step.color ?? "")} />
      <span className="flex-1 truncate">{step.title}</span>
      {(isCurrent || hasAutoStart) && (
        <span className="ml-auto flex items-center gap-1 text-[10px] text-muted-foreground">
          {isCurrent && (
            <span data-testid={`${testIdPrefix}-current-${step.id}`}>{t("task:current2")}</span>
          )}
          {hasAutoStart && (
            <span data-testid={`${testIdPrefix}-autostart-${step.id}`}>{t("task:autoStart")}</span>
          )}
        </span>
      )}
    </>
  );
}

function StepMenuItem({
  step,
  currentStepId,
  onSelect,
  onSelectWithOptions,
  onSubmitWithOptions,
  isMoving,
  testIdPrefix = "task-context-step",
}: {
  step: TaskMoveStep;
  currentStepId?: string | null;
  onSelect: (stepId: string) => void;
  onSelectWithOptions?: (stepId: string) => void;
  onSubmitWithOptions?: (
    stepId: string,
    entryOptions: WorkflowMoveEntryOptions | undefined,
  ) => Promise<boolean>;
  isMoving?: boolean;
  testIdPrefix?: string;
}) {
  const { t } = useTranslation();
  const pointerTypeRef = useRef<string | null>(null);
  const isCurrent = step.id === currentStepId;
  const content = <StepMenuLabel step={step} isCurrent={isCurrent} testIdPrefix={testIdPrefix} />;

  if (!onSelectWithOptions && !onSubmitWithOptions) {
    return (
      <ContextMenuItem
        className="[@media(pointer:coarse)]:min-h-11"
        data-testid={`${testIdPrefix}-${step.id}`}
        disabled={isCurrent}
        onSelect={(event) => {
          event.preventDefault();
          if (!isCurrent) onSelect(step.id);
        }}
      >
        {content}
      </ContextMenuItem>
    );
  }

  return (
    <ContextMenuSub>
      <ContextMenuSubTrigger
        className="[@media(pointer:coarse)]:min-h-11"
        data-testid={`${testIdPrefix}-${step.id}`}
        disabled={isCurrent}
        onPointerDown={(event) => {
          pointerTypeRef.current = event.pointerType;
        }}
        onPointerCancel={() => {
          pointerTypeRef.current = null;
        }}
        onClick={(event) => {
          const pointerType = pointerTypeRef.current;
          pointerTypeRef.current = null;
          const rect = event.currentTarget.getBoundingClientRect();
          const tappedChevron =
            (pointerType === "touch" || pointerType === "pen") &&
            rect.width > 0 &&
            event.clientX >= rect.right - 32;
          if (!tappedChevron) {
            event.preventDefault();
            if (!isCurrent) onSelect(step.id);
          }
        }}
        onKeyDown={(event) => {
          if (!isCurrent && (event.key === "Enter" || event.key === " ")) {
            event.preventDefault();
            onSelect(step.id);
          }
        }}
      >
        {content}
      </ContextMenuSubTrigger>
      <ContextMenuSubContent className={onSubmitWithOptions ? "w-72" : "w-48"}>
        {onSubmitWithOptions ? (
          // Interactive fields live inside a Radix menu popup. Stop key events at
          // this wrapper (bubble phase, after the field handled them) so the
          // menu's typeahead never yanks focus off the instructions field or the
          // agent-profile combobox search while the user is typing.
          <div className="p-1" onKeyDown={(event) => event.stopPropagation()}>
            <WorkflowMoveOptionsForm
              isMoving={isMoving ?? false}
              isTouchSurface={false}
              instructionsRows={3}
              onSubmit={(entryOptions) => onSubmitWithOptions(step.id, entryOptions)}
            />
          </div>
        ) : (
          <ContextMenuItem
            className="[@media(pointer:coarse)]:min-h-11"
            data-testid={`task-context-step-options-${step.id}`}
            onSelect={(event) => {
              event.preventDefault();
              onSelectWithOptions?.(step.id);
            }}
          >
            <IconAdjustments className="mr-2 h-4 w-4" />
            {t("task:moveWithOptions")}
          </ContextMenuItem>
        )}
      </ContextMenuSubContent>
    </ContextMenuSub>
  );
}

function MoveToCurrentWorkflowSubmenu({
  steps,
  currentStepId,
  disabled,
  onMoveToStep,
  onMoveToStepWithOptions,
  onSubmitWithOptions,
  isMoving,
}: {
  steps: TaskMoveStep[];
  currentStepId?: string | null;
  disabled?: boolean;
  onMoveToStep?: (stepId: string) => void;
  onMoveToStepWithOptions?: (stepId: string) => void;
  onSubmitWithOptions?: (
    stepId: string,
    entryOptions: WorkflowMoveEntryOptions | undefined,
  ) => Promise<boolean>;
  isMoving?: boolean;
}) {
  const { t } = useTranslation();
  if (!onMoveToStep || steps.length <= 1) return null;
  return (
    <ContextMenuSub>
      <ContextMenuSubTrigger
        className="[@media(pointer:coarse)]:min-h-11"
        data-testid="task-context-move-to"
        disabled={disabled}
      >
        <IconArrowRight className="mr-2 h-4 w-4" />
        {t("task:moveTo")}
      </ContextMenuSubTrigger>
      <ContextMenuSubContent className="w-48">
        {steps.map((step) => (
          <StepMenuItem
            key={step.id}
            step={step}
            currentStepId={currentStepId}
            onSelect={onMoveToStep}
            onSelectWithOptions={onMoveToStepWithOptions}
            onSubmitWithOptions={onSubmitWithOptions}
            isMoving={isMoving}
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
  const { t } = useTranslation();
  if (steps.length === 0 || !onSendToWorkflow) {
    return (
      <ContextMenuItem
        data-testid={`task-context-workflow-${workflow.id}`}
        disabled
        aria-disabled="true"
      >
        <span className="flex-1 truncate">{workflow.name}</span>
        <span data-testid="task-context-disabled-reason" className="ml-2 text-[10px]">
          {t("task:noSteps")}
        </span>
      </ContextMenuItem>
    );
  }

  return (
    <ContextMenuSub>
      <ContextMenuSubTrigger
        className="[@media(pointer:coarse)]:min-h-11"
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
  const { t } = useTranslation();
  const targets = workflows.filter((workflow) => workflow.id !== currentWorkflowId);
  if (!onSendToWorkflow || !currentWorkflowId || targets.length === 0) return null;
  return (
    <ContextMenuSub>
      <ContextMenuSubTrigger
        className="[@media(pointer:coarse)]:min-h-11"
        data-testid="task-context-send-to-workflow"
        disabled={disabled}
      >
        <IconLogicBuffer className="mr-2 h-4 w-4" />
        {t("task:sendToWorkflow")}
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
  onMoveToStepWithOptions,
  onSubmitWithOptions,
  isMoving,
  onSendToWorkflow,
}: TaskMoveContextMenuItemsProps) {
  const visibleWorkflows = workflows.filter((workflow) => !workflow.hidden);
  const currentSteps = currentWorkflowId ? (stepsByWorkflowId[currentWorkflowId] ?? []) : [];
  const hasSameWorkflowMove = Boolean(
    (onMoveToStep || onMoveToStepWithOptions || onSubmitWithOptions) && currentSteps.length > 1,
  );
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
        onMoveToStepWithOptions={onMoveToStepWithOptions}
        onSubmitWithOptions={onSubmitWithOptions}
        isMoving={isMoving}
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
