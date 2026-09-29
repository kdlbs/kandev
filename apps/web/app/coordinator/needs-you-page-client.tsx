"use client";

import type { CoordinatorInputStatus } from "./use-coordinator-attention";
import { CoordinatorRouteContent } from "./coordinator-route-content";
import { NeedsYouItemsPanel } from "./components/needs-you-items-panel";
import { WatchesNoneNotice } from "./components/watches-none-notice";
import { GoalNote } from "./components/goal-note";
import { useFeature } from "@/hooks/domains/features/use-feature";
import type { NeedsYouItem } from "@/lib/coordinator/attention";
import { Phase2PageProvider } from "./phase2-page-provider";

export type NeedsYouPageClientProps = {
  workspaceId: string;
  coordinatorId: string | null;
};

function proposalsKey(items: NeedsYouItem[]): string {
  return items
    .flatMap((item) =>
      item.kind === "proposal" ? [`${item.proposal.id}:${item.proposal.status}`] : [],
    )
    .join(",");
}

function stallTaskIds(items: NeedsYouItem[]): string[] {
  return items.flatMap((item) => (item.kind === "stall" ? [item.task.id] : []));
}

function MaybePhase2({
  enabled,
  workspaceId,
  coordinatorId,
  items,
  children,
}: {
  enabled: boolean;
  workspaceId: string;
  coordinatorId: string;
  items: NeedsYouItem[];
  children: React.ReactNode;
}) {
  if (!enabled) return <>{children}</>;
  return (
    <Phase2PageProvider
      workspaceId={workspaceId}
      coordinatorId={coordinatorId}
      proposalsKey={proposalsKey(items)}
      liveStallTaskIds={stallTaskIds(items)}
    >
      {children}
    </Phase2PageProvider>
  );
}

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
  const phase2 = useFeature("coordinatorPhase2");
  return (
    <CoordinatorRouteContent
      workspaceId={workspaceId}
      coordinatorId={coordinatorId}
      view="needs-you"
    >
      {({ coordinator, attention, canManage }) => (
        <MaybePhase2
          enabled={phase2}
          workspaceId={workspaceId}
          coordinatorId={coordinator.id}
          items={attention.classification.needsYou}
        >
          {phase2 && (
            <GoalNote
              workspaceId={workspaceId}
              coordinatorId={coordinator.id}
              canManage={canManage}
            />
          )}
          {phase2 && (
            <WatchesNoneNotice
              workspaceId={workspaceId}
              coordinatorId={coordinator.id}
              watchSet={attention.watchSet}
              chooseBoards={canManage}
            />
          )}
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
        </MaybePhase2>
      )}
    </CoordinatorRouteContent>
  );
}
