"use client";

import { IconArrowsMaximize, IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useAppStore } from "@/components/state-provider";
import type { UseEnsureTaskSessionResult } from "@/hooks/domains/session/use-ensure-task-session";
import type { Task } from "./kanban-card";
import { PreviewSessionTabs } from "./task/preview-session-tabs";
import { TaskMoveErrorBanner } from "./task/task-move-error-banner";
import { MinimalWorkflowStepper, type WorkflowStepperStep } from "./task/workflow-step-disclosure";
import { useTranslation } from "react-i18next";

interface TaskPreviewPanelProps {
  task: Task | null;
  sessionId?: string | null;
  ensureSession?: UseEnsureTaskSessionResult;
  onClose: () => void;
  onMaximize?: (task: Task) => void;
  onSessionChange?: (sessionId: string | null) => void;
  workflowSteps?: WorkflowStepperStep[];
  currentStepId?: string | null;
  taskWorkflowId?: string | null;
  movingToStepId?: string | null;
  onMoveStep?: (stepId: string) => Promise<boolean>;
  onDisclosureOpenChange?: (open: boolean) => void;
  moveError?: unknown;
}

interface PreviewPanelHeaderProps {
  task: Task | null;
  onClose: () => void;
  onMaximize?: (task: Task) => void;
  workflowSteps: WorkflowStepperStep[];
  currentStepId: string | null;
  taskWorkflowId: string | null;
  movingToStepId: string | null;
  onMoveStep?: (stepId: string) => Promise<boolean>;
  onDisclosureOpenChange?: (open: boolean) => void;
}

function PreviewPanelHeader({
  task,
  onClose,
  onMaximize,
  workflowSteps,
  currentStepId,
  taskWorkflowId,
  movingToStepId,
  onMoveStep,
  onDisclosureOpenChange,
}: PreviewPanelHeaderProps) {
  const { t } = useTranslation();
  const currentIndex = workflowSteps.findIndex((step) => step.id === currentStepId);
  const handleMoveStep = task && workflowSteps.length > 0 ? onMoveStep : undefined;
  return (
    <div className="flex items-center justify-between border-b px-4 py-3">
      <div className="flex min-w-0 flex-1 items-center">
        <h2 className="min-w-[88px] flex-1 truncate text-sm font-semibold">
          {task?.title ?? t("task:taskChat")}
        </h2>
        {handleMoveStep && (
          <div
            // `w-full` on the trigger button counteracts a browser quirk:
            // a <button> normally sizes to its content regardless of `display`,
            // so without an explicit width it overflows this shrink-capped
            // wrapper instead of shrinking to it (defeating truncation).
            className="min-w-0 max-w-[50%] shrink [&>button]:w-full"
          >
            <MinimalWorkflowStepper
              sortedSteps={workflowSteps}
              currentIndex={currentIndex}
              taskId={task?.id ?? null}
              workflowId={taskWorkflowId}
              movingToStepId={movingToStepId}
              onMove={handleMoveStep}
              onDisclosureOpenChange={onDisclosureOpenChange}
            />
          </div>
        )}
      </div>
      <div className="flex items-center gap-1">
        {onMaximize && task && (
          <Button
            variant="ghost"
            size="icon"
            className="h-8 w-8 cursor-pointer"
            onClick={() => onMaximize(task)}
            title={t("common:openFullPage")}
          >
            <IconArrowsMaximize className="h-4 w-4" />
            <span className="sr-only">{t("common:openFullPage")}</span>
          </Button>
        )}
        <Button variant="ghost" size="icon" className="h-8 w-8 cursor-pointer" onClick={onClose}>
          <IconX className="h-4 w-4" />
          <span className="sr-only">{t("task:closePreview")}</span>
        </Button>
      </div>
    </div>
  );
}

export function TaskPreviewPanel({
  task,
  sessionId = null,
  ensureSession,
  onClose,
  onMaximize,
  onSessionChange,
  workflowSteps = [],
  currentStepId = null,
  taskWorkflowId = null,
  movingToStepId = null,
  onMoveStep,
  onDisclosureOpenChange,
  moveError = null,
}: TaskPreviewPanelProps) {
  const { t } = useTranslation();
  const activeWorkspaceId = useAppStore((s) => s.workspaces.activeId);
  return (
    <div
      data-testid="task-preview-panel"
      className="flex h-full w-full flex-col border-l bg-background"
    >
      <PreviewPanelHeader
        task={task}
        onClose={onClose}
        onMaximize={onMaximize}
        workflowSteps={workflowSteps}
        currentStepId={currentStepId}
        taskWorkflowId={taskWorkflowId}
        movingToStepId={movingToStepId}
        onMoveStep={onMoveStep}
        onDisclosureOpenChange={onDisclosureOpenChange}
      />

      {moveError !== null && <TaskMoveErrorBanner error={moveError} />}

      {/* Content */}
      <div className="flex-1 min-h-0 flex flex-col">
        {task ? (
          <PreviewSessionTabs
            taskId={task.id}
            sessionId={sessionId}
            ensureSession={ensureSession}
            workspaceId={activeWorkspaceId ?? null}
            onSessionChange={onSessionChange}
          />
        ) : (
          <div className="flex flex-1 items-center justify-center text-sm text-muted-foreground">
            {t("task:selectATaskToStartChatting")}
          </div>
        )}
      </div>
    </div>
  );
}
