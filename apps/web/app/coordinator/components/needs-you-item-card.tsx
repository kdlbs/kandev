import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Badge } from "@kandev/ui/badge";
import { Card, CardAction, CardContent, CardFooter, CardHeader, CardTitle } from "@kandev/ui/card";
import { cn } from "@/lib/utils";
import type { AttentionTask, NeedsYouAskableItem, NeedsYouItem } from "@/lib/coordinator/attention";
import { useProposalRow } from "@/hooks/domains/coordinator/use-proposals";
import { useRelayItem } from "@/hooks/domains/coordinator/use-relay-item";
import { useCoordinatorPhase3Effective } from "@/hooks/domains/settings/use-coordinator-phase3-effective";
import { deriveCopilotItemId, deriveCopilotItemRef } from "@/lib/coordinator/copilot-id";
import { formatAge } from "@/lib/coordinator/format";
import { resolveProposalSourceTask, severityFor, whyClearsText } from "@/lib/coordinator/item-text";
import { ProposalCard, type ProposalCardForm } from "../proposal-card/proposal-card";
import { AskAboutThisButton, NeedsYouItemPrimaryActions } from "./needs-you-item-actions";
import { RelayAnswer } from "./relay-answer";

export type NeedsYouItemCardProps = {
  item: NeedsYouAskableItem;
  workspaceId: string;
  stepNameByTaskId: Map<string, string>;
  workflowNameById: Map<string, string>;
  stepNameByWorkflowStep: Map<string, string>;
  openTasksById: Map<string, AttentionTask>;
  coordinatorName: string;
  coordinatorId: string;
  canManage: boolean;
  /** Opens the proposal's form immediately, from the chat card's "Forms and navigation" deep link. */
  autoOpenForm?: ProposalCardForm | null;
  onAutoFormOpened?: () => void;
  /** "full" variant's "Next" toast line (proposal-cards.md#cards "Toast counts"). */
  computeNeedsYouCount?: () => number;
  /** Reports whether the item is expanded (held) so the list keeps rendering it. */
  onExpandedChange?: (item: NeedsYouItem, expanded: boolean) => void;
};

/** `needs-you-item-heading-<id>`, the focusable heading id `proposal-cards.md#cards "Focus after a decision"` moves focus to. */
export function needsYouItemHeadingId(itemId: string): string {
  return `needs-you-item-heading-${itemId}`;
}

function useItemRelay(
  item: NeedsYouAskableItem,
  workspaceId: string,
  coordinatorId: string,
  canManage: boolean,
) {
  const phase3Effective = useCoordinatorPhase3Effective();
  return useRelayItem({
    workspaceId,
    coordinatorId,
    taskId: item.kind === "question" ? item.task.id : null,
    pendingAction: item.kind === "question" ? item.pendingAction : undefined,
    enabled: item.kind === "question" && phase3Effective && canManage,
    refreshKey: relayRefreshKey(item),
  });
}

/** Changes whenever the task's status summary changes, which re-reads the relay. */
function relayRefreshKey(item: NeedsYouAskableItem): string {
  if (item.kind !== "question") return "";
  const summary = item.task.statusSummary;
  return [summary?.pending_action, summary?.last_activity_at, item.task.updatedAt].join("|");
}

function headFor(
  item: NeedsYouAskableItem,
  stepNameByTaskId: Map<string, string>,
  openTasksById: Map<string, AttentionTask>,
  newTaskLabel: string,
): { identifier: string; stepName: string | undefined } {
  if (item.kind === "proposal") {
    const sourceTask = resolveProposalSourceTask(item, openTasksById);
    if (!sourceTask) return { identifier: newTaskLabel, stepName: undefined };
    return {
      identifier: sourceTask.identifier ?? sourceTask.title,
      stepName: stepNameByTaskId.get(sourceTask.id),
    };
  }
  return {
    identifier: item.task.identifier ?? item.task.title,
    stepName: stepNameByTaskId.get(item.task.id),
  };
}

/**
 * One Needs you item, rendering the common head (identifier/step, severity
 * pill, age, Ask about this), the why/clears texts, kind-specific details for
 * a proposal, and the kind-specific actions
 * (docs/specs/coordinator/requirements/needs-you.md REQ-COORDINATOR-NEEDS-YOU-002).
 */
// eslint-disable-next-line max-lines-per-function -- the card composes the head, kind-specific bodies and the held answer
export function NeedsYouItemCard({
  item,
  workspaceId,
  stepNameByTaskId,
  workflowNameById,
  stepNameByWorkflowStep,
  openTasksById,
  coordinatorName,
  coordinatorId,
  canManage,
  autoOpenForm,
  onAutoFormOpened,
  computeNeedsYouCount,
  onExpandedChange,
}: NeedsYouItemCardProps) {
  const { t } = useTranslation();
  const head = headFor(item, stepNameByTaskId, openTasksById, t("coordinator:newTask"));
  const severity = severityFor(item);
  const { why, clears } = whyClearsText(item, t);
  const copilotId = deriveCopilotItemId(item, openTasksById);
  const copilotRef = deriveCopilotItemRef(item);
  const headingRef = useRef<HTMLDivElement>(null);
  const proposalId = item.kind === "proposal" ? item.proposal.id : null;
  const proposalRow = useProposalRow(coordinatorId, proposalId);
  const relay = useItemRelay(item, workspaceId, coordinatorId, canManage);
  const cardRef = useRef<HTMLDivElement>(null);
  // A started reply holds the item until the card remounts (navigation).
  const [replyHeld, setReplyHeld] = useState(false);
  const held = relay.expanded || replyHeld;
  useEffect(() => {
    onExpandedChange?.(item, held);
  }, [item, held, onExpandedChange]);
  const answerable = relay.offered || relay.expanded;

  return (
    // The severity is the stripe down the left edge, so the list can be
    // scanned for what decides now without reading every badge (mockup v2.1
    // `.item.hot` / `.item.cool`).
    <Card
      ref={cardRef}
      className={cn(
        "border-l-[3px]",
        severity === "decide-now" ? "border-l-destructive" : "border-l-primary",
      )}
      data-testid={`needs-you-item-${item.id}`}
    >
      <CardHeader>
        {/* pr-2: the age ends this cell and Ask about this begins the next, so
            without it the two sit 4px apart (mockup v2.1 `.itemhead` gap: 8px). */}
        <CardTitle
          ref={headingRef}
          id={needsYouItemHeadingId(item.id)}
          tabIndex={-1}
          className="flex flex-wrap items-center gap-2 pr-2"
        >
          <span>{head.identifier}</span>
          {head.stepName && (
            <span className="text-muted-foreground font-normal">{head.stepName}</span>
          )}
          <Badge variant={severity === "decide-now" ? "destructive" : "secondary"}>
            {severity === "decide-now"
              ? t("coordinator:severityDecideNow")
              : t("coordinator:severityReview")}
          </Badge>
          <span className="text-muted-foreground ml-auto font-normal">{formatAge(item.ageMs)}</span>
        </CardTitle>
        <CardAction>
          <AskAboutThisButton
            coordinatorId={coordinatorId}
            id={copilotId}
            itemRef={copilotRef}
            canManage={canManage}
          />
        </CardAction>
      </CardHeader>
      <CardContent className="space-y-2">
        {item.kind === "proposal" && proposalRow && (
          <ProposalCard
            variant="full"
            proposal={proposalRow}
            canManage={canManage}
            workspaceId={workspaceId}
            coordinatorId={coordinatorId}
            workflowNameById={workflowNameById}
            stepNameByWorkflowStep={stepNameByWorkflowStep}
            openTasksById={openTasksById}
            coordinatorName={coordinatorName}
            autoOpenForm={autoOpenForm}
            onAutoFormOpened={onAutoFormOpened}
            onFormForceClosed={() => headingRef.current?.focus()}
            computeNeedsYouCount={computeNeedsYouCount}
            onReplyStarted={() => setReplyHeld(true)}
          />
        )}
        {/* Label beside its text, not above it: two stacked pairs turned a
            four-line card into eight (mockup v2.1 `.why`). */}
        <dl className="grid grid-cols-[minmax(82px,max-content)_minmax(0,1fr)] gap-x-2.5 gap-y-0.5">
          <dt className="text-muted-foreground">{t("coordinator:whyItIsHere")}</dt>
          <dd className="m-0 min-w-0 [overflow-wrap:anywhere]">{why}</dd>
          {!answerable && (
            <>
              <dt className="text-muted-foreground">{t("coordinator:whatClearsIt")}</dt>
              <dd className="m-0 min-w-0 [overflow-wrap:anywhere]">{clears}</dd>
            </>
          )}
        </dl>
        <RelayAnswer relay={relay} itemRef={cardRef} />
      </CardContent>
      {item.kind !== "proposal" && (
        <CardFooter>
          <NeedsYouItemPrimaryActions
            item={item}
            canManage={canManage}
            answerHere={
              answerable ? { expanded: relay.expanded, onToggle: relay.toggle } : undefined
            }
          />
        </CardFooter>
      )}
    </Card>
  );
}
