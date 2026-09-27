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
}: NeedsYouItemCardProps) {
  const { t } = useTranslation();
  const head = headFor(item, stepNameByTaskId, openTasksById, t("coordinator:newTask"));
  const severity = severityFor(item);
  const { why, clears } = whyClearsText(item, t);

  return (
    <Card data-testid={`needs-you-item-${item.id}`}>
      <CardHeader>
        <CardTitle className="flex flex-wrap items-center gap-2">
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
          <AskAboutThisButton />
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
