import type { useTranslation } from "react-i18next";
import type { useToast } from "@/components/toast-provider";
import {
  isCreateTaskProposal,
  isImprovementProposal,
  type KindProposal,
  type StoredProposal,
} from "@/lib/api/domains/coordinator-api";
import type { ProposalDecisionOutcome } from "@/hooks/domains/coordinator/use-proposal-decision";
import { effectiveProposalSpec, resolveStepName } from "@/lib/coordinator/proposal-text";
import type { EditFormServerError } from "./edit-form";
import { kindApprovedToast, loweredToast } from "./outcome-copy";

export type ProposalCardVariant = "full" | "compact";
export type ProposalCardForm = "edit" | "reject";

export type TFn = ReturnType<typeof useTranslation>["t"];
type ToastFn = ReturnType<typeof useToast>["toast"];

function nextLine(t: TFn, computeNeedsYouCount?: () => number) {
  if (!computeNeedsYouCount) return undefined;
  const count = computeNeedsYouCount();
  return count === 0 ? t("coordinator:toastNextNone") : t("coordinator:toastNextCount", { count });
}

export type OutcomeContext = {
  t: TFn;
  toast: ToastFn;
  variant: ProposalCardVariant;
  stepNameByWorkflowStep: Map<string, string>;
  computeNeedsYouCount?: () => number;
  setServerError: (error: EditFormServerError | null) => void;
  setOpenForm: (form: ProposalCardForm | null) => void;
  resolveCardLabel: (proposal: StoredProposal) => Promise<string>;
  /** The target task label of a resume, message or move card. */
  targetLabel: string;
  setPolicyDenied: (denied: boolean) => void;
  offerReject?: (offer: { proposalId: string; reason: string }) => void;
};

async function toastApproved(proposal: StoredProposal, ctx: OutcomeContext) {
  let title: string;
  if (isImprovementProposal(proposal)) {
    title = ctx.t("coordinator:improvementApprovedPending");
  } else if (isCreateTaskProposal(proposal)) {
    const step = resolveStepName(effectiveProposalSpec(proposal), ctx.stepNameByWorkflowStep);
    const card = await ctx.resolveCardLabel(proposal);
    title = ctx.t("coordinator:toastApproved", { card, step });
  } else {
    title = kindApprovedToast(
      proposal,
      ctx.targetLabel,
      kindStepName(proposal, ctx.stepNameByWorkflowStep),
      ctx.t,
    );
  }
  ctx.toast({
    title,
    description: nextLine(ctx.t, ctx.computeNeedsYouCount),
    variant: "success",
  });
}

export function kindStepName(proposal: KindProposal, names: Map<string, string>): string {
  if (proposal.kind !== "move") return "";
  const spec = proposal.final_spec ?? proposal.spec;
  return names.get(`${spec.workflow_id}:${spec.to_step_id}`) ?? spec.to_step_id;
}

function toastRejected(proposalId: string, rejectReason: string | undefined, ctx: OutcomeContext) {
  const { t, toast, computeNeedsYouCount } = ctx;
  const reason = rejectReason?.trim();
  const offer = ctx.offerReject && reason ? { proposalId, reason } : null;
  toast({
    title: offer ? t("coordinator:toastRejectedOffer") : t("coordinator:toastRejected"),
    description: nextLine(t, computeNeedsYouCount),
    variant: "success",
    ...(offer && {
      duration: 10_000,
      action: {
        label: t("coordinator:toastRejectedOfferAction"),
        onClick: () => ctx.offerReject?.(offer),
      },
    }),
  });
}

/**
 * Applies one decision outcome per the decision-outcomes table
 * (docs/specs/coordinator/system-design/proposal-cards.md#cards "Decision
 * outcomes"): toast copy, form close/keep-open, and server-error display.
 * Extracted from ProposalCard to keep the component's own branching low.
 */
export function applyDecisionOutcome(
  outcome: ProposalDecisionOutcome,
  plainApprove: boolean,
  ctx: OutcomeContext,
  rejectReason?: string,
) {
  const { t, toast, variant, setServerError, setOpenForm } = ctx;
  switch (outcome.kind) {
    case "decided": {
      setServerError(null);
      setOpenForm(null);
      loweredToast(outcome.proposal, ctx);
      if (outcome.proposal.status === "failed") return;
      if (outcome.proposal.status === "approved") {
        void toastApproved(outcome.proposal, ctx);
      } else if (outcome.proposal.status === "rejected") {
        toastRejected(outcome.proposal.id, rejectReason, ctx);
      }
      return;
    }
    case "validation":
      setServerError({ message: outcome.message, field: outcome.field });
      // Only the full (Needs-you) card can show the edit form inline; the
      // compact chat card keeps its buttons visible and surfaces the error
      // via serverError-driven copy, since it has no inline form surface.
      if (plainApprove && variant === "full") setOpenForm("edit");
      return;
    case "policy_denied":
      setOpenForm(null);
      setServerError(null);
      ctx.setPolicyDenied(true);
      return;
    case "conflict":
      setOpenForm(null);
      setServerError(null);
      toast({ title: t("coordinator:toastConflict"), variant: "error" });
      return;
    case "forbidden":
      toast({ title: t("coordinator:toastForbidden"), variant: "error" });
      return;
    case "not_found":
      toast({ title: t("coordinator:toastNotFound"), variant: "error" });
      return;
    case "network":
      toast({ title: t("coordinator:toastNetworkError"), variant: "error" });
  }
}
