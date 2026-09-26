"use client";

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
  };
}

/**
 * Resolves the ordered step list for a workflow that is not necessarily the
 * board's active workflow: the previewed task's own workflow, per
 * `plugin-context-api.ts`'s rule. `kanban.steps` covers the active workflow;
 * `kanbanMulti.snapshots` covers other workflows. The workspace-wide board
 * uses `useAllWorkflowSnapshots`, while task pages fetch only their own
 * workflow through `useWorkflowSnapshotById`.
 */
export function useWorkflowStepsById(workflowId: string | null | undefined): WorkflowStepperStep[] {
  const activeWorkflowId = useAppStore((state) => state.kanban.workflowId);
  const activeSteps = useAppStore((state) => state.kanban.steps);
  const snapshotSteps = useAppStore((state) =>
    workflowId ? state.kanbanMulti.snapshots[workflowId]?.steps : undefined,
  );

  return useMemo(() => {
    if (!workflowId) return [];
    const rawSteps = workflowId === activeWorkflowId ? activeSteps : snapshotSteps;
    if (!rawSteps || rawSteps.length === 0) return [];
    return sortWorkflowStepsByPosition(rawSteps.map(mapStep));
  }, [workflowId, activeWorkflowId, activeSteps, snapshotSteps]);
}
