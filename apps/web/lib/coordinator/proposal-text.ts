import type { TFunction } from "i18next";
import type { Proposal, ProposalSpec } from "@/lib/api/domains/coordinator-api";

/**
 * The spec a card should currently display: `final_spec` once a decision has
 * stamped it, else the original `spec` (docs/specs/coordinator/system-design/
 * proposal-cards.md#cards "Content by state").
 */
export function effectiveProposalSpec(
  proposal: Pick<Proposal, "spec" | "final_spec">,
): ProposalSpec {
  return proposal.final_spec ?? proposal.spec;
}

/** "`<workflow>` · `<step>`" of a spec, falling back to the raw id when a name is unknown. */
export function workflowStepLabel(
  spec: Pick<ProposalSpec, "workflow_id" | "step_id">,
  workflowNameById: Map<string, string>,
  stepNameByWorkflowStep: Map<string, string>,
): string {
  const workflowName = workflowNameById.get(spec.workflow_id) ?? spec.workflow_id;
  const stepName =
    stepNameByWorkflowStep.get(`${spec.workflow_id}:${spec.step_id}`) ?? spec.step_id;
  return `${workflowName} · ${stepName}`;
}

/** The name of `spec`'s step, falling back to the raw id (used by the approve toast's `<step>`). */
export function resolveStepName(
  spec: Pick<ProposalSpec, "workflow_id" | "step_id">,
  stepNameByWorkflowStep: Map<string, string>,
): string {
  return stepNameByWorkflowStep.get(`${spec.workflow_id}:${spec.step_id}`) ?? spec.step_id;
}

/**
 * The status line for `pending`, `approving`, `failed` and `rejected`
 * (proposal-cards.md#cards "Content by state"). `approved` is not handled
 * here: it needs `<card>`, resolved asynchronously by the caller, via
 * {@link approvedStatusLine}.
 */
export function proposalStatusLine(
  proposal: Pick<Proposal, "status" | "error" | "reject_reason">,
  t: TFunction,
): string {
  switch (proposal.status) {
    case "pending":
      return t("coordinator:proposalStatusPending");
    case "approving":
      return t("coordinator:proposalStatusApproving");
    case "failed":
      return proposal.error
        ? t("coordinator:proposalStatusFailedWithError", { error: proposal.error })
        : t("coordinator:proposalStatusFailed");
    case "rejected":
      return proposal.reject_reason
        ? t("coordinator:proposalStatusRejectedWithReason", { reason: proposal.reject_reason })
        : t("coordinator:proposalStatusRejected");
    default:
      return proposal.status;
  }
}

/** "Approved: `<card>`" (proposal-cards.md#cards "`<card>` and `<step>`"). */
export function approvedStatusLine(cardLabel: string, t: TFunction): string {
  return t("coordinator:proposalStatusApproved", { card: cardLabel });
}

/** The `<card>` fallback when the task's identifier is absent, unreadable or deleted: the current spec's title. */
export function approvedCardFallbackTitle(spec: Pick<ProposalSpec, "title">): string {
  return spec.title;
}
