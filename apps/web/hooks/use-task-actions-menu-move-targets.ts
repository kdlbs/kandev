"use client";

import { useMemo } from "react";
import { useAppStore } from "@/components/state-provider";
import type { WorkflowStep } from "@/components/kanban-card";
import type { KanbanCardMoveTargets } from "@/components/kanban-card-menu-items";
import type { TaskMoveStep, TaskMoveWorkflow } from "@/components/task/task-move-context-menu";
import { sortWorkflowStepsByPosition } from "@/lib/kanban/auto-hide-empty-columns";

/**
 * Move-target resolution for callers with no board-column context (the
 * preview panel, the detail top bar). Layers two fallbacks the card's hot
 * render path (`useKanbanCardMoveTargets`) deliberately omits (system
 * design: the card hook keeps a fixed, minimal subscription set so
 * unrelated writes to these slices never re-render every visible card):
 *  - resolves `currentWorkflowId` from the flat `kanban.tasks` list when a
 *    WS-arrived task hasn't yet been merged into its workflow's snapshot
 *    (the same race `findTaskInSnapshots` falls back for).
 *  - filters the current workflow's steps by the user's hidden-step
 *    preference when the caller has no `steps` list of its own to pass
 *    (AC-TASKS-TASK-ACTIONS-MENU-002.3b: Move to must match the card).
 */
export function useTaskActionsMenuMoveTargets(
  taskId: string,
  steps?: WorkflowStep[],
): KanbanCardMoveTargets {
  const workflows = useAppStore((state) => state.workflows.items);
  const snapshots = useAppStore((state) => state.kanbanMulti.snapshots);
  const kanbanTasks = useAppStore((state) => state.kanban.tasks);
  const hiddenWorkflowStepIds = useAppStore((state) => state.userSettings.hiddenWorkflowStepIds);

  const currentWorkflowId = useMemo(() => {
    for (const [workflowId, snapshot] of Object.entries(snapshots)) {
      if (snapshot.tasks.some((task) => task.id === taskId)) return workflowId;
    }
    return kanbanTasks.find((task) => task.id === taskId)?.workflowId ?? null;
  }, [snapshots, kanbanTasks, taskId]);

  const workflowItems = useMemo<TaskMoveWorkflow[]>(() => {
    const current = workflows.find((workflow) => workflow.id === currentWorkflowId);
    return workflows
      .filter((workflow) => workflow.workspaceId === current?.workspaceId && !workflow.hidden)
      .map((workflow) => ({ id: workflow.id, name: workflow.name, hidden: workflow.hidden }));
  }, [workflows, currentWorkflowId]);

  const fallbackCurrentWorkflowSteps = useMemo<WorkflowStep[] | undefined>(() => {
    if (!currentWorkflowId || steps) return undefined;
    const snapshotSteps = snapshots[currentWorkflowId]?.steps;
    if (!snapshotSteps) return undefined;
    const hiddenIds = hiddenWorkflowStepIds[currentWorkflowId];
    const hiddenSet = hiddenIds && hiddenIds.length > 0 ? new Set(hiddenIds) : null;
    const sorted = sortWorkflowStepsByPosition(snapshotSteps);
    return hiddenSet ? sorted.filter((step) => !hiddenSet.has(step.id)) : sorted;
  }, [currentWorkflowId, steps, snapshots, hiddenWorkflowStepIds]);

  const stepsByWorkflowId = useMemo<Record<string, TaskMoveStep[]>>(() => {
    const result: Record<string, TaskMoveStep[]> = {};
    for (const [workflowId, snapshot] of Object.entries(snapshots)) {
      result[workflowId] = sortWorkflowStepsByPosition(snapshot.steps).map((step) => ({
        id: step.id,
        title: step.title,
        color: step.color,
        events: step.events,
      }));
    }
    const effectiveCurrentWorkflowSteps = steps ?? fallbackCurrentWorkflowSteps;
    if (currentWorkflowId && effectiveCurrentWorkflowSteps) {
      result[currentWorkflowId] = effectiveCurrentWorkflowSteps.map((step) => ({
        id: step.id,
        title: step.title,
        color: step.color,
        events: step.events,
      }));
    }
    return result;
  }, [snapshots, currentWorkflowId, steps, fallbackCurrentWorkflowSteps]);

  return { currentWorkflowId, workflowItems, stepsByWorkflowId };
}
