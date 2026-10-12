import type { AttentionTask, NeedsYouItem } from "@/lib/coordinator/attention";
import { useNeedsYouFocusAfterDecision } from "../use-needs-you-focus";
import { useNeedsYouFormNavigation } from "../use-needs-you-navigation";
import { EmptyNeedsYouState } from "./empty-needs-you-state";
import { NeedsYouItemCard } from "./needs-you-item-card";

export type NeedsYouItemsPanelProps = {
  items: NeedsYouItem[];
  workingCount: number;
  inputsLoaded: boolean;
  workspaceId: string;
  coordinatorId: string;
  coordinatorName: string;
  canManage: boolean;
  attentionMaps: {
    stepNameByTaskId: Map<string, string>;
    workflowNameById: Map<string, string>;
    stepNameByWorkflowStep: Map<string, string>;
    openTasksById: Map<string, AttentionTask>;
  };
  computeNeedsYouCount: () => number;
};

/**
 * The Needs you list (or its empty state), plus the two cross-item
 * behaviours that only make sense at the list's level: the chat card's
 * `?proposal=<id>&form=edit|reject` deep link
 * (proposal-cards.md#cards "Forms and navigation") and focus-after-decision
 * (proposal-cards.md#cards "Focus after a decision").
 */
export function NeedsYouItemsPanel({
  items,
  workingCount,
  inputsLoaded,
  workspaceId,
  coordinatorId,
  coordinatorName,
  canManage,
  attentionMaps,
  computeNeedsYouCount,
}: NeedsYouItemsPanelProps) {
  const { autoOpenProposalId, autoOpenForm, onAutoFormOpened } = useNeedsYouFormNavigation(
    items,
    inputsLoaded,
  );
  useNeedsYouFocusAfterDecision(items);

  if (items.length === 0) {
    return (
      <EmptyNeedsYouState
        workingCount={workingCount}
        workspaceId={workspaceId}
        coordinatorId={coordinatorId}
      />
    );
  }

  return (
    <div className="space-y-3" data-testid="needs-you-item-list">
      {items.map((item) => (
        <NeedsYouItemCard
          key={item.id}
          item={item}
          workspaceId={workspaceId}
          stepNameByTaskId={attentionMaps.stepNameByTaskId}
          workflowNameById={attentionMaps.workflowNameById}
          stepNameByWorkflowStep={attentionMaps.stepNameByWorkflowStep}
          openTasksById={attentionMaps.openTasksById}
          coordinatorName={coordinatorName}
          coordinatorId={coordinatorId}
          canManage={canManage}
          computeNeedsYouCount={computeNeedsYouCount}
          autoOpenForm={item.id === autoOpenProposalId ? autoOpenForm : null}
          onAutoFormOpened={item.id === autoOpenProposalId ? onAutoFormOpened : undefined}
        />
      ))}
    </div>
  );
}
