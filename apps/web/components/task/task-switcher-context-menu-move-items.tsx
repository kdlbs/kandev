"use client";

import { TaskMoveContextMenuItems } from "@/components/task/task-move-context-menu";
import type { TaskContextMenuItemsProps } from "./task-switcher-context-menu";
import { useWorkflowStepProgress } from "@/hooks/domains/kanban/use-workflow-step-progress";

export function TaskMoveItems({
  task,
  workflows,
  stepsByWorkflowId,
  steps,
  isDeleting,
  onMoveToStep,
  onMoveToStepWithOptions,
  onSubmitWithOptions,
  moveOptionsBusy,
  actingIds,
  actingOnSelection,
  onBulkMove,
  isMixedWorkflowSelection,
  closeMenu,
  moveTasks,
  onChangeWorkflow,
}: Omit<TaskContextMenuItemsProps, "onRenameTask" | "onArchiveTask" | "onDeleteTask"> & {
  actingIds: string[];
  actingOnSelection: boolean;
}) {
  const workflowId = task.workflowId;
  const { progressByStepId, agentLabelsByProfileId } = useTaskMoveProgress(task, actingOnSelection);
  if (!workflowId) return null;
  const runSelectionMove = (
    targetWorkflowId: string,
    stepId: string,
    destination: "step" | "workflow",
  ) => {
    closeMenu();
    if (onBulkMove) {
      onBulkMove(actingIds, targetWorkflowId, stepId);
      return;
    }
    void moveTasks(actingIds, targetWorkflowId, stepId, destination).catch(() => {
      // useTaskWorkflowMove already shows the failure toast.
    });
  };

  let moveToStep: ((stepId: string) => void) | undefined;
  if (actingOnSelection) {
    moveToStep = isMixedWorkflowSelection
      ? undefined
      : (stepId) => runSelectionMove(workflowId, stepId, "step");
  } else {
    moveToStep = (stepId) => {
      closeMenu();
      if (onMoveToStep) {
        onMoveToStep(task.id, workflowId, stepId);
        return;
      }
      void moveTasks([task.id], workflowId, stepId, "step").catch(() => {
        // useTaskWorkflowMove already shows the failure toast.
      });
    };
  }

  return (
    <TaskMoveContextMenuItems
      currentWorkflowId={workflowId}
      currentStepId={actingOnSelection ? undefined : task.workflowStepId}
      workflows={workflows ?? []}
      stepsByWorkflowId={stepsByWorkflowId ?? (steps ? { [workflowId]: steps } : {})}
      disabled={isDeleting || task.isArchived}
      showSeparator={false}
      onMoveToStep={moveToStep}
      onMoveToStepWithOptions={
        actingIds.length === 1 && onMoveToStepWithOptions ? onMoveToStepWithOptions : undefined
      }
      onSubmitWithOptions={
        actingIds.length === 1 && onSubmitWithOptions ? onSubmitWithOptions : undefined
      }
      isMoving={moveOptionsBusy}
      progressByStepId={progressByStepId}
      agentLabelsByProfileId={agentLabelsByProfileId}
      isBulkSelection={actingIds.length > 1}
      onChangeWorkflow={actingIds.length === 1 ? onChangeWorkflow : undefined}
      onSendToWorkflow={
        actingIds.length > 1
          ? (targetWorkflowId, stepId) => {
              if (actingOnSelection) {
                runSelectionMove(targetWorkflowId, stepId, "workflow");
                return;
              }
              closeMenu();
              void moveTasks([task.id], targetWorkflowId, stepId, "workflow").catch(() => {
                // useTaskWorkflowMove already shows the failure toast.
              });
            }
          : undefined
      }
    />
  );
}

function useTaskMoveProgress(task: TaskContextMenuItemsProps["task"], actingOnSelection: boolean) {
  const taskProjection = actingOnSelection
    ? null
    : {
        id: task.id,
        state: task.state,
        primarySessionId: task.primarySessionId,
        primarySessionState: task.sessionState,
      };
  return useWorkflowStepProgress({
    taskId: actingOnSelection ? null : task.id,
    currentStepId: actingOnSelection ? null : task.workflowStepId,
    taskProjection,
    // Context menus render the bounded task-row projection. Session caches
    // belong to the task detail surface and may still describe an older step.
    preferTaskProjection: true,
    useSessionProjection: false,
  });
}
