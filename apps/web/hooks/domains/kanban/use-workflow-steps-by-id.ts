"use client";

import { useWorkflowStepMetadata } from "./use-workflow-step-metadata";
import { useMemo } from "react";
import { useAppStore } from "@/components/state-provider";
import type { KanbanState } from "@/lib/state/slices";
import { sortWorkflowStepsByPosition } from "@/lib/kanban/workflow-step-order";
import type { WorkflowStepperStep } from "@/components/task/workflow-step-disclosure";

type StoreStep = KanbanState["steps"][number];

function mapStep(step: StoreStep): WorkflowStepperStep {
  return {
    id: step.id,
    name: step.title,
    color: step.color,
    position: step.position,
    events: step.events,
    allow_manual_move: step.allow_manual_move,
    prompt: step.prompt,
    is_start_step: step.is_start_step,
    agent_profile_id: step.agent_profile_id,
    complete_task_on_enter: step.complete_task_on_enter,
  };
}

/** Resolves loaded board steps or narrow metadata for a task's own workflow. */
export function useWorkflowStepsById(workflowId: string | null | undefined): WorkflowStepperStep[] {
  const activeWorkflowId = useAppStore((state) => state.kanban.workflowId);
  const activeSteps = useAppStore((state) => state.kanban.steps);
  const snapshotSteps = useAppStore((state) =>
    workflowId ? state.kanbanMulti.snapshots[workflowId]?.steps : undefined,
  );

  const placeholder = useAppStore((state) =>
    workflowId ? state.kanbanMulti.snapshots[workflowId]?.isPlaceholder === true : false,
  );
  const cachedSteps = workflowId === activeWorkflowId ? activeSteps : snapshotSteps;
  const needsMetadata = cachedSteps === undefined || (placeholder && cachedSteps.length === 0);
  const metadataSteps = useWorkflowStepMetadata(workflowId && needsMetadata ? workflowId : null);
  return useMemo(() => {
    if (!workflowId) return [];
    const rawSteps = needsMetadata ? metadataSteps : cachedSteps;
    if (!rawSteps || rawSteps.length === 0) return [];
    return sortWorkflowStepsByPosition(rawSteps.map(mapStep));
  }, [workflowId, cachedSteps, metadataSteps, needsMetadata]);
}
