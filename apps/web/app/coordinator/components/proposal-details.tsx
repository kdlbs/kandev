import { useTranslation } from "react-i18next";
import { CardDescription } from "@kandev/ui/card";
import type { AttentionProposal } from "@/lib/coordinator/attention";

export type ProposalDetailsProps = {
  proposal: AttentionProposal;
  workflowNameById: Map<string, string>;
  stepNameByWorkflowStep: Map<string, string>;
  coordinatorName: string;
};

/**
 * A proposal item's title, description, target workflow/step, attribution
 * and propose-only policy line (AC-COORDINATOR-NEEDS-YOU-002.8). No decision
 * actions render here; task 08 adds Approve/Edit/Reject.
 */
export function ProposalDetails({
  proposal,
  workflowNameById,
  stepNameByWorkflowStep,
  coordinatorName,
}: ProposalDetailsProps) {
  const { t } = useTranslation();
  const workflowName = workflowNameById.get(proposal.spec.workflow_id) ?? proposal.spec.workflow_id;
  const stepName =
    stepNameByWorkflowStep.get(`${proposal.spec.workflow_id}:${proposal.spec.step_id}`) ??
    proposal.spec.step_id;

  return (
    <div className="space-y-1">
      <p className="text-sm font-medium">{proposal.spec.title}</p>
      <CardDescription>{proposal.spec.description}</CardDescription>
      <CardDescription>
        {workflowName} · {stepName}
      </CardDescription>
      <CardDescription>{t("coordinator:proposedBy", { name: coordinatorName })}</CardDescription>
      <CardDescription>{t("coordinator:policyProposeOnly")}</CardDescription>
    </div>
  );
}
