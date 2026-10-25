"use client";

import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import type { Task } from "@/components/kanban-card";
import type { WorkflowStep } from "@/components/kanban-column";

export const ORPHAN_STEP_ID = "__kandev_orphan__";

export const ORPHAN_STEP: WorkflowStep = {
  id: ORPHAN_STEP_ID,
  title: "",
  color: "#f59e0b",
};

export function isOrphanMoveTarget(targetStepId: string): boolean {
  return targetStepId === ORPHAN_STEP_ID;
}

export function remapOrphanTasks(
  tasks: Task[],
  stepIds: Set<string>,
  orphanStepId: string,
): { tasks: Task[]; hasOrphans: boolean } {
  let hasOrphans = false;
  const remapped = tasks.map((task) => {
    if (!task.workflowStepId || stepIds.has(task.workflowStepId)) return task;
    hasOrphans = true;
    return { ...task, workflowStepId: orphanStepId };
  });
  return { tasks: remapped, hasOrphans };
}

export function useOrphanDisplay(
  tasks: Task[],
  steps: WorkflowStep[],
): { displayTasks: Task[]; displaySteps: WorkflowStep[] } {
  const { t } = useTranslation("kanban");
  return useMemo(() => {
    const stepIds = new Set(steps.map((step) => step.id));
    const { tasks: displayTasks, hasOrphans } = remapOrphanTasks(tasks, stepIds, ORPHAN_STEP_ID);
    const displaySteps = hasOrphans
      ? [...steps, { ...ORPHAN_STEP, title: t("kanban:needsReassignment") }]
      : steps;
    return { displayTasks, displaySteps };
  }, [tasks, steps, t]);
}
