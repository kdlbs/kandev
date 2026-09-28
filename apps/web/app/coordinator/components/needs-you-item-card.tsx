import { useTranslation } from "react-i18next";
import { Badge } from "@kandev/ui/badge";
import { Card, CardAction, CardContent, CardFooter, CardHeader, CardTitle } from "@kandev/ui/card";
import { cn } from "@/lib/utils";
import type { AttentionTask, NeedsYouItem } from "@/lib/coordinator/attention";
import { deriveCopilotItemId } from "@/lib/coordinator/copilot-id";
import { formatAge } from "@/lib/coordinator/format";
import { resolveProposalSourceTask, severityFor, whyClearsText } from "@/lib/coordinator/item-text";
import { AskAboutThisButton, NeedsYouItemPrimaryActions } from "./needs-you-item-actions";
import { ProposalDetails } from "./proposal-details";

export type NeedsYouItemCardProps = {
  item: NeedsYouItem;
  stepNameByTaskId: Map<string, string>;
  workflowNameById: Map<string, string>;
  stepNameByWorkflowStep: Map<string, string>;
  openTasksById: Map<string, AttentionTask>;
  coordinatorName: string;
  coordinatorId: string;
  canManage: boolean;
};

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
  stepNameByTaskId,
  workflowNameById,
  stepNameByWorkflowStep,
  openTasksById,
  coordinatorName,
  coordinatorId,
  canManage,
}: NeedsYouItemCardProps) {
  const { t } = useTranslation();
  const head = headFor(item, stepNameByTaskId, openTasksById, t("coordinator:newTask"));
  const severity = severityFor(item);
  const { why, clears } = whyClearsText(item, t);
  const copilotId = deriveCopilotItemId(item, openTasksById);

  return (
    // The severity is the stripe down the left edge, so the list can be
    // scanned for what decides now without reading every badge (mockup v2.1
    // `.item.hot` / `.item.cool`).
    <Card
      className={cn(
        "border-l-[3px]",
        severity === "decide-now" ? "border-l-destructive" : "border-l-primary",
      )}
      data-testid={`needs-you-item-${item.id}`}
    >
      <CardHeader>
        {/* pr-2: the age ends this cell and Ask about this begins the next, so
            without it the two sit 4px apart (mockup v2.1 `.itemhead` gap: 8px). */}
        <CardTitle className="flex flex-wrap items-center gap-2 pr-2">
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
          <AskAboutThisButton coordinatorId={coordinatorId} id={copilotId} canManage={canManage} />
        </CardAction>
      </CardHeader>
      <CardContent className="space-y-2">
        {item.kind === "proposal" && (
          <ProposalDetails
            proposal={item.proposal}
            workflowNameById={workflowNameById}
            stepNameByWorkflowStep={stepNameByWorkflowStep}
            coordinatorName={coordinatorName}
          />
        )}
        {/* Label beside its text, not above it: two stacked pairs turned a
            four-line card into eight (mockup v2.1 `.why`). */}
        <dl className="grid grid-cols-[minmax(82px,max-content)_minmax(0,1fr)] gap-x-2.5 gap-y-0.5">
          <dt className="text-muted-foreground">{t("coordinator:whyItIsHere")}</dt>
          <dd className="m-0 min-w-0 [overflow-wrap:anywhere]">{why}</dd>
          <dt className="text-muted-foreground">{t("coordinator:whatClearsIt")}</dt>
          <dd className="m-0 min-w-0 [overflow-wrap:anywhere]">{clears}</dd>
        </dl>
      </CardContent>
      {item.kind !== "proposal" && (
        <CardFooter>
          <NeedsYouItemPrimaryActions item={item} />
        </CardFooter>
      )}
    </Card>
  );
}
