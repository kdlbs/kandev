"use client";

import { useMemo } from "react";
import { useShallow } from "zustand/react/shallow";
import { useAppStore } from "@/components/state-provider";
import type { WorkflowStep } from "@/components/kanban-card";
import type { TaskMoveStep, TaskMoveWorkflow } from "@/components/task/task-move-context-menu";
import { sortWorkflowStepsByPosition } from "@/lib/kanban/workflow-step-order";
import type { WorkflowSnapshotData } from "@/lib/state/slices/kanban/types";

export type KanbanCardMoveTargets = {
  currentWorkflowId: string | null;
  workflowItems: TaskMoveWorkflow[];
  stepsByWorkflowId: Record<string, TaskMoveStep[]>;
};

/**
 * The card's hot render path: a caller with column context already passes a
 * `steps` list pre-filtered for this workflow, and its `currentWorkflowId`
 * always resolves from `kanbanMulti.snapshots`. Kept to exactly these two
 * subscriptions (system design: no `kanban.tasks`/`hiddenWorkflowStepIds`
 * reads here) so unrelated writes to either slice never re-render every
 * visible card. Callers without column context (preview panel, detail top
 * bar) use `useTaskActionsMenuMoveTargets` instead, which layers the
 * flat-list and hidden-step fallbacks this hook intentionally omits.
 */
export function useKanbanCardMoveTargets(
  taskId: string,
  steps?: WorkflowStep[],
): KanbanCardMoveTargets {
  const workflows = useAppStore((state) => state.workflows.items);
  const currentWorkflowId = useAppStore((state) => {
    for (const [workflowId, snapshot] of Object.entries(state.kanbanMulti.snapshots)) {
      if (snapshot.tasks.some((task) => task.id === taskId)) return workflowId;
    }
    return null;
  });
  const snapshotStepsByWorkflowId = useAppStore(
    useShallow((state): Record<string, WorkflowSnapshotData["steps"]> => {
      const result: Record<string, WorkflowSnapshotData["steps"]> = {};
      for (const [workflowId, snapshot] of Object.entries(state.kanbanMulti.snapshots)) {
        result[workflowId] = snapshot.steps;
      }
      return result;
    }),
  );

  const workflowItems = useMemo<TaskMoveWorkflow[]>(() => {
    const current = workflows.find((workflow) => workflow.id === currentWorkflowId);
    return workflows
      .filter((workflow) => workflow.workspaceId === current?.workspaceId && !workflow.hidden)
      .map((workflow) => ({ id: workflow.id, name: workflow.name, hidden: workflow.hidden }));
  }, [workflows, currentWorkflowId]);

  const stepsByWorkflowId = useMemo<Record<string, TaskMoveStep[]>>(() => {
    const result: Record<string, TaskMoveStep[]> = {};
    for (const [workflowId, snapshotSteps] of Object.entries(snapshotStepsByWorkflowId)) {
      result[workflowId] = sortWorkflowStepsByPosition(snapshotSteps).map((step) => ({
        id: step.id,
        title: step.title,
        color: step.color,
        events: step.events,
      }));
    }
    if (currentWorkflowId && steps) {
      result[currentWorkflowId] = steps.map((step) => ({
        id: step.id,
        title: step.title,
        color: step.color,
        events: step.events,
      }));
    }
    return result;
  }, [snapshotStepsByWorkflowId, currentWorkflowId, steps]);

  return { currentWorkflowId, workflowItems, stepsByWorkflowId };
}
