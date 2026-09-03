"use client";

import { useEffect, useRef, useState } from "react";
import { IconArrowsMaximize, IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useAppStore } from "@/components/state-provider";
import type { UseEnsureTaskSessionResult } from "@/hooks/domains/session/use-ensure-task-session";
import type { Task } from "./kanban-card";
import { PreviewSessionTabs } from "./task/preview-session-tabs";
import { TaskMoveErrorBanner } from "./task/task-move-error-banner";
import { MinimalWorkflowStepper, type WorkflowStepperStep } from "./task/workflow-step-disclosure";
import { TaskActionsMenuTrigger } from "./task/task-actions-menu-trigger";
import { TaskActionsMenuDialogs } from "./task/task-actions-menu-dialogs";
import { useTaskActionsMenu, type TaskActionsMenuBoardRow } from "@/hooks/use-task-actions-menu";
import { useTaskCRUD } from "@/hooks/use-task-crud";
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
  isArchived?: boolean;
  movingToStepId?: string | null;
  onMoveStep?: (stepId: string) => Promise<boolean>;
  onDisclosureOpenChange?: (open: boolean) => void;
  moveError?: unknown;
  /** Lets the enclosing surface skip its own Escape-close while this menu is open. */
  onActionsMenuOpenChange?: (open: boolean) => void;
}

function buildBoardRow(task: Task | null): TaskActionsMenuBoardRow | null {
  if (!task) return null;
  return {
    id: task.id,
    title: task.title,
    description: task.description,
    workflowStepId: task.workflowStepId,
    state: task.state,
    repositoryId: task.repositoryId,
    repositories: task.repositories,
    parentTaskId: task.parentTaskId,
    primaryExecutorType: task.primaryExecutorType,
    workspaceMode: task.workspaceMode,
  };
}

/**
 * Menu open state lives here (not inside the Radix primitive alone) so a
 * subject-identity change can force it closed without retargeting
 * (AC-TASKS-TASK-ACTIONS-MENU-004.5a), and so the panel's own Escape handler
 * can tell whether an open menu already owns the keypress
 * (AC-TASKS-TASK-ACTIONS-MENU-001.11).
 */
function useActionsMenuOpenState(
  taskId: string | null,
  onActionsMenuOpenChange?: (open: boolean) => void,
) {
  const [open, setOpenState] = useState(false);
  const setOpen = (next: boolean) => {
    setOpenState(next);
    onActionsMenuOpenChange?.(next);
  };

  const prevTaskIdRef = useRef(taskId);
  useEffect(() => {
    if (prevTaskIdRef.current !== taskId) {
      prevTaskIdRef.current = taskId;
      setOpen(false);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [taskId]);

  return { open, setOpen };
}

interface PreviewPanelHeaderProps {
  task: Task | null;
  onClose: () => void;
  onMaximize?: (task: Task) => void;
  workflowSteps: WorkflowStepperStep[];
  currentStepId: string | null;
  taskWorkflowId: string | null;
  isArchived: boolean;
  movingToStepId: string | null;
  onMoveStep?: (stepId: string) => Promise<boolean>;
  onDisclosureOpenChange?: (open: boolean) => void;
  menuEntries: ReturnType<typeof useTaskActionsMenu>["entries"];
  triggerRef: ReturnType<typeof useTaskActionsMenu>["triggerRef"];
  actionsMenuOpen: boolean;
  setActionsMenuOpen: (open: boolean) => void;
}

function PreviewPanelHeader({
  task,
  onClose,
  onMaximize,
  workflowSteps,
  currentStepId,
  taskWorkflowId,
  isArchived,
  movingToStepId,
  onMoveStep,
  onDisclosureOpenChange,
  menuEntries,
  triggerRef,
  actionsMenuOpen,
  setActionsMenuOpen,
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
            // Keyed by task id so a task switch remounts this subtree instead of
            // reusing it: the disclosure's open state is local to it, and nothing
            // else ties that state (or a still-settling move promise's closure)
            // to which task is being previewed.
            key={task?.id ?? "none"}
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
              isArchived={isArchived}
              movingToStepId={movingToStepId}
              onMove={handleMoveStep}
              onDisclosureOpenChange={onDisclosureOpenChange}
            />
          </div>
        )}
      </div>
      <div className="flex items-center gap-1">
        {task && (
          <TaskActionsMenuTrigger
            entries={menuEntries}
            testId="task-preview-actions-menu"
            triggerRef={triggerRef}
            open={actionsMenuOpen}
            onOpenChange={setActionsMenuOpen}
          />
        )}
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
  isArchived = false,
  movingToStepId = null,
  onMoveStep,
  onDisclosureOpenChange,
  moveError = null,
  onActionsMenuOpenChange,
}: TaskPreviewPanelProps) {
  const { t } = useTranslation();
  const activeWorkspaceId = useAppStore((s) => s.workspaces.activeId);
  const workspaceId = activeWorkspaceId ?? null;
  const taskId = task?.id ?? null;
  const taskTitle = task?.title ?? "";
  const taskCRUD = useTaskCRUD();
  const { open: actionsMenuOpen, setOpen: setActionsMenuOpen } = useActionsMenuOpenState(
    taskId,
    onActionsMenuOpenChange,
  );
  const boardRow = buildBoardRow(task);
  const isArchiving = task ? taskCRUD.archivingTaskId === task.id : false;
  const isDeleting = task ? taskCRUD.deletingTaskId === task.id : false;

  const menu = useTaskActionsMenu({
    taskId,
    taskTitle,
    workspaceId,
    // The board excludes archived tasks, so the preview panel never holds one.
    isArchived: false,
    boardRow,
    isArchiving,
    isDeleting,
    onArchive: (opts) => (task ? taskCRUD.handleArchive(task, opts) : undefined),
    onDelete: (opts) => (task ? taskCRUD.handleDelete(task, opts) : undefined),
  });

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
        isArchived={isArchived}
        movingToStepId={movingToStepId}
        onMoveStep={onMoveStep}
        onDisclosureOpenChange={onDisclosureOpenChange}
        menuEntries={menu.entries}
        triggerRef={menu.triggerRef}
        actionsMenuOpen={actionsMenuOpen}
        setActionsMenuOpen={setActionsMenuOpen}
      />

      {moveError !== null && <TaskMoveErrorBanner error={moveError} />}

      {/* Content */}
      <div className="flex-1 min-h-0 flex flex-col">
        {task ? (
          <PreviewSessionTabs
            taskId={task.id}
            sessionId={sessionId}
            ensureSession={ensureSession}
            workspaceId={workspaceId}
            onSessionChange={onSessionChange}
          />
        ) : (
          <div className="flex flex-1 items-center justify-center text-sm text-muted-foreground">
            {t("task:selectATaskToStartChatting")}
          </div>
        )}
      </div>
      <TaskActionsMenuDialogs
        taskId={taskId}
        taskTitle={taskTitle}
        workspaceId={workspaceId}
        boardRow={boardRow}
        isArchiving={isArchiving}
        isDeleting={isDeleting}
        menu={menu}
      />
    </div>
  );
}
