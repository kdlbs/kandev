"use client";

import { TaskMoveContextMenuItems } from "@/components/task/task-move-context-menu";
import type { TaskContextMenuItemsProps } from "./task-switcher-context-menu";

export function TaskMoveItems({
  task,
  workflows,
  stepsByWorkflowId,
  steps,
  isDeleting,
  onMoveToStep,
  actingIds,
  actingOnSelection,
  onBulkMove,
  isMixedWorkflowSelection,
  closeMenu,
  moveTasks,
}: Omit<TaskContextMenuItemsProps, "onRenameTask" | "onArchiveTask" | "onDeleteTask"> & {
  actingIds: string[];
  actingOnSelection: boolean;
}) {
  if (!task.workflowId) return null;
  const workflowId = task.workflowId;
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
      onSendToWorkflow={(targetWorkflowId, stepId) => {
        if (actingOnSelection) {
          runSelectionMove(targetWorkflowId, stepId, "workflow");
          return;
        }
        closeMenu();
        void moveTasks([task.id], targetWorkflowId, stepId, "workflow").catch(() => {
          // useTaskWorkflowMove already shows the failure toast.
        });
      }}
    />
  );
}
