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

/** The client-side stale threshold for an `approving` claim, matching the backend's two-minute window (proposals.md#recovery, approval_sweep.go). */
export const PROPOSAL_APPROVAL_STALE_MS = 2 * 60 * 1000;

/**
 * Whether an `approving` claim is stale, per the browser's clock with no
 * server time offset (`AC-COORDINATOR-PROPOSALS-005.10`).
 */
export function isApprovalClaimStale(claimedAt: string | null, nowMs: number): boolean {
  if (!claimedAt) return false;
  return nowMs >= new Date(claimedAt).getTime() + PROPOSAL_APPROVAL_STALE_MS;
}

/**
 * The status line for `pending`, `approving`, `failed` and `rejected`
 * (proposal-cards.md#cards "Content by state"). `approved` is not handled
 * here: it needs `<card>`, resolved asynchronously by the caller, via
 * {@link approvedStatusLine}.
 */
export function proposalStatusLine(
  proposal: Pick<Proposal, "status" | "error" | "reject_reason" | "claimed_at" | "reply_text">,
  t: TFunction,
  nowMs: number,
): string {
  switch (proposal.status) {
    case "pending":
      return t("coordinator:proposalStatusPending");
    case "approving":
      return isApprovalClaimStale(proposal.claimed_at, nowMs)
        ? t("coordinator:proposalStatusApprovingStale")
        : t("coordinator:proposalStatusApproving");
    case "failed":
      return proposal.error
        ? t("coordinator:proposalStatusFailedWithError", { error: proposal.error })
        : t("coordinator:proposalStatusFailed");
    case "rejected":
      return proposal.reject_reason
        ? t("coordinator:proposalStatusRejectedWithReason", { reason: proposal.reject_reason })
        : t("coordinator:proposalStatusRejected");
    case "returned":
      return t("coordinator:proposalStatusReturned", { text: proposal.reply_text ?? "" });
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
