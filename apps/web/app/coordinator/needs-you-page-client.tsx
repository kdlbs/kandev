"use client";

import type { CoordinatorInputStatus } from "./use-coordinator-attention";
import { CoordinatorRouteContent } from "./coordinator-route-content";
import { NeedsYouItemsPanel } from "./components/needs-you-items-panel";

export type NeedsYouPageClientProps = {
  workspaceId: string;
  coordinatorId: string | null;
};

/** Loaded or errored for every input: `proposal-cards.md#cards "Forms and navigation"` waits for this before acting on a deep link. */
function allInputsLoaded(inputs: CoordinatorInputStatus[]): boolean {
  return inputs.every((input) => input.loadedAt !== undefined || input.error);
}

/**
 * The Needs you screen: one item card per entry the coordinator classifies
 * as needing a human decision, or the empty state when there is none
 * (docs/specs/coordinator/requirements/needs-you.md REQ-COORDINATOR-NEEDS-YOU-001..003).
 */
export function NeedsYouPageClient({ workspaceId, coordinatorId }: NeedsYouPageClientProps) {
  return (
    <CoordinatorRouteContent
      workspaceId={workspaceId}
      coordinatorId={coordinatorId}
      view="needs-you"
    >
      {({ coordinator, attention, canManage }) => (
        <NeedsYouItemsPanel
          items={attention.classification.needsYou}
          workingCount={attention.classification.queue.working.length}
          inputsLoaded={allInputsLoaded(attention.inputs)}
          workspaceId={workspaceId}
          coordinatorId={coordinator.id}
          coordinatorName={coordinator.name}
          canManage={canManage}
          attentionMaps={{
            stepNameByTaskId: attention.stepNameByTaskId,
            workflowNameById: attention.workflowNameById,
            stepNameByWorkflowStep: attention.stepNameByWorkflowStep,
            openTasksById: attention.openTasksById,
          }}
          computeNeedsYouCount={attention.computeNeedsYouCount}
        />
      )}
    </CoordinatorRouteContent>
  );
}
