import { useCallback, useMemo, useState } from "react";
import type { AttentionTask, NeedsYouItem } from "@/lib/coordinator/attention";
import { useNeedsYouFocusAfterDecision } from "../use-needs-you-focus";
import { useNeedsYouFormNavigation } from "../use-needs-you-navigation";
import { EmptyNeedsYouState } from "./empty-needs-you-state";
import { AutonomyItemCard } from "./autonomy-item-card";
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

type HeldItem = { item: NeedsYouItem; index: number };

/**
 * Items the manager has expanded stay in the list, at their position, after
 * the task stops needing the manager, until they collapse
 * (docs/specs/coordinator/system-design/relay.md "Question card").
 */
function useHeldItems(items: NeedsYouItem[]) {
  const [held, setHeld] = useState<Record<string, HeldItem>>({});
  const onExpandedChange = useCallback(
    (item: NeedsYouItem, expanded: boolean) => {
      setHeld((current) => {
        if (!expanded) {
          if (!(item.id in current)) return current;
          const { [item.id]: _released, ...rest } = current;
          return rest;
        }
        const live = items.findIndex((candidate) => candidate.id === item.id);
        const index = live >= 0 ? live : (current[item.id]?.index ?? items.length);
        const existing = current[item.id];
        if (existing && existing.item === item && existing.index === index) return current;
        return { ...current, [item.id]: { item, index } };
      });
    },
    [items],
  );
  const displayItems = useMemo(() => {
    const live = new Set(items.map((item) => item.id));
    const merged = [...items];
    Object.values(held)
      .filter((entry) => !live.has(entry.item.id))
      .sort((a, b) => a.index - b.index)
      .forEach((entry) => merged.splice(Math.min(entry.index, merged.length), 0, entry.item));
    return merged;
  }, [items, held]);
  return { displayItems, onExpandedChange };
}

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
  const { displayItems, onExpandedChange } = useHeldItems(items);

  if (displayItems.length === 0) {
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
      {displayItems.map((item) =>
        item.kind === "autonomy" ? (
          <AutonomyItemCard
            key={item.id}
            item={item}
            workspaceId={workspaceId}
            coordinatorId={coordinatorId}
            canManage={canManage}
          />
        ) : (
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
            onExpandedChange={onExpandedChange}
            autoOpenForm={item.id === autoOpenProposalId ? autoOpenForm : null}
            onAutoFormOpened={item.id === autoOpenProposalId ? onAutoFormOpened : undefined}
          />
        ),
      )}
    </div>
  );
}
