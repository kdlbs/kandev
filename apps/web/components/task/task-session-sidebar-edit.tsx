"use client";

import { useCallback, useState } from "react";
import { useAppStoreApi } from "@/components/state-provider";
import { TaskCreateDialog } from "@/components/task-create-dialog";
import type { KanbanState } from "@/lib/state/slices";
import type { Task } from "@/lib/types/http";
import { findTaskInSnapshots } from "@/lib/kanban/find-task";
import type { StepDef } from "./task-switcher-context-menu";
import type { TaskSwitcherItem } from "./task-switcher";

export type SidebarTaskEditTarget = {
  id: string;
  title: string;
  description?: string;
  workflowId: string;
  workflowStepId: string;
  state?: Task["state"];
  repositoryId?: string;
};

export function buildSidebarTaskEditTarget(
  item: Pick<TaskSwitcherItem, "id" | "workflowId" | "workflowStepId">,
  sourceTask: KanbanState["tasks"][number] | null,
): SidebarTaskEditTarget | null {
  if (!sourceTask || !item.workflowId) return null;
  const workflowStepId = sourceTask.workflowStepId ?? item.workflowStepId;
  if (!workflowStepId) return null;

  return {
    id: sourceTask.id,
    title: sourceTask.title,
    description: sourceTask.description,
    workflowId: item.workflowId,
    workflowStepId,
    state: sourceTask.state,
    repositoryId: sourceTask.repositoryId ?? undefined,
  };
}

export function useSidebarTaskEdit() {
  const store = useAppStoreApi();
  const [editingTask, setEditingTask] = useState<SidebarTaskEditTarget | null>(null);

  const handleEditTask = useCallback(
    (item: TaskSwitcherItem) => {
      const state = store.getState();
      const sourceTask = findTaskInSnapshots(
        item.id,
        state.kanbanMulti.snapshots,
        state.kanban.tasks,
      );
      const target = buildSidebarTaskEditTarget(item, sourceTask);
      if (target) setEditingTask(target);
    },
    [store],
  );

  return { editingTask, setEditingTask, handleEditTask };
}

export function SidebarTaskEditDialog({
  target,
  onTargetChange,
  workspaceId,
  stepsByWorkflowId,
}: {
  target: SidebarTaskEditTarget | null;
  onTargetChange: (target: SidebarTaskEditTarget | null) => void;
  workspaceId: string | null;
  stepsByWorkflowId: Record<string, StepDef[]>;
}) {
  const steps = target ? (stepsByWorkflowId[target.workflowId] ?? []) : [];
  return (
    <TaskCreateDialog
      open={target !== null}
      onOpenChange={(open) => {
        if (!open) onTargetChange(null);
      }}
      mode="edit"
      workspaceId={workspaceId}
      workflowId={target?.workflowId ?? null}
      lockedFields={{ workflow: true }}
      defaultStepId={target?.workflowStepId ?? null}
      steps={steps}
      editingTask={
        target
          ? {
              id: target.id,
              title: target.title,
              description: target.description,
              workflowStepId: target.workflowStepId,
              state: target.state,
              repositoryId: target.repositoryId,
            }
          : null
      }
      initialValues={
        target
          ? {
              title: target.title,
              description: target.description,
              state: target.state,
              repositoryId: target.repositoryId,
            }
          : undefined
      }
    />
  );
}
