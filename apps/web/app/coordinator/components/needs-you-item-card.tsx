import { useRef } from "react";
import { useTranslation } from "react-i18next";
import { Badge } from "@kandev/ui/badge";
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@kandev/ui/card";
import type { AttentionTask, NeedsYouItem } from "@/lib/coordinator/attention";
import { useProposalRow } from "@/hooks/domains/coordinator/use-proposals";
import { deriveCopilotItemId } from "@/lib/coordinator/copilot-id";
import { formatAge } from "@/lib/coordinator/format";
import { resolveProposalSourceTask, severityFor, whyClearsText } from "@/lib/coordinator/item-text";
import { ProposalCard, type ProposalCardForm } from "../proposal-card/proposal-card";
import { AskAboutThisButton, NeedsYouItemPrimaryActions } from "./needs-you-item-actions";

export type NeedsYouItemCardProps = {
  item: NeedsYouItem;
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
};

/** `needs-you-item-heading-<id>`, the focusable heading id `proposal-cards.md#cards "Focus after a decision"` moves focus to. */
export function needsYouItemHeadingId(itemId: string): string {
  return `needs-you-item-heading-${itemId}`;
}

function headFor(
  item: NeedsYouItem,
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
}: NeedsYouItemCardProps) {
  const { t } = useTranslation();
  const head = headFor(item, stepNameByTaskId, openTasksById, t("coordinator:newTask"));
  const severity = severityFor(item);
  const { why, clears } = whyClearsText(item, t);
  const copilotId = deriveCopilotItemId(item, openTasksById);
  const headingRef = useRef<HTMLDivElement>(null);
  const proposalId = item.kind === "proposal" ? item.proposal.id : null;
  const proposalRow = useProposalRow(coordinatorId, proposalId);

  return (
    <Card data-testid={`needs-you-item-${item.id}`}>
      <CardHeader>
        <CardTitle
          ref={headingRef}
          id={needsYouItemHeadingId(item.id)}
          tabIndex={-1}
          className="flex flex-wrap items-center gap-2"
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
          <span className="text-muted-foreground font-normal">{formatAge(item.ageMs)}</span>
        </CardTitle>
        <CardAction>
          <AskAboutThisButton coordinatorId={coordinatorId} id={copilotId} canManage={canManage} />
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
            coordinatorName={coordinatorName}
            autoOpenForm={autoOpenForm}
            onAutoFormOpened={onAutoFormOpened}
            onFormForceClosed={() => headingRef.current?.focus()}
            computeNeedsYouCount={computeNeedsYouCount}
          />
        )}
        <div>
          <p className="text-xs font-medium">{t("coordinator:whyItIsHere")}</p>
          <CardDescription>{why}</CardDescription>
        </div>
        <div>
          <p className="text-xs font-medium">{t("coordinator:whatClearsIt")}</p>
          <CardDescription>{clears}</CardDescription>
        </div>
      </CardContent>
      {item.kind !== "proposal" && (
        <CardFooter>
          <NeedsYouItemPrimaryActions item={item} />
        </CardFooter>
      )}
    </Card>
  );
}
