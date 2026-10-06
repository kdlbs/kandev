"use client";

import { useEffect, useRef, useState, type RefObject } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { CardDescription } from "@kandev/ui/card";
import { Spinner } from "@kandev/ui/spinner";
import type {
  ApproveProposalEdits,
  Proposal,
  ProposalSpec,
} from "@/lib/api/domains/coordinator-api";
import {
  useProposalDecision,
  type ProposalDecisionOutcome,
} from "@/hooks/domains/coordinator/use-proposal-decision";
import {
  approvedStatusLine,
  effectiveProposalSpec,
  isApprovalClaimStale,
  proposalStatusLine,
  resolveStepName,
  workflowStepLabel,
} from "@/lib/coordinator/proposal-text";
import { useToast } from "@/components/toast-provider";
import { useNowTick } from "../use-now-tick";
import { EditForm, type EditFormServerError } from "./edit-form";
import { RejectForm } from "./reject-form";
import { useApprovedCardLabel } from "./use-approved-card-label";

export type ProposalCardVariant = "full" | "compact";
export type ProposalCardForm = "edit" | "reject";

export type ProposalCardProps = {
  proposal: Proposal;
  /** "full" is the Needs-you card (description, attribution, policy line, inline forms); "compact" is the chat card. */
  variant: ProposalCardVariant;
  canManage: boolean;
  workspaceId: string;
  coordinatorId: string;
  workflowNameById: Map<string, string>;
  stepNameByWorkflowStep: Map<string, string>;
  /** Required for the "full" variant's "Proposed by" line. */
  coordinatorName?: string;
  /** Opens this form immediately (the Needs-you deep link from a chat card's Edit/Reject). */
  autoOpenForm?: ProposalCardForm | null;
  onAutoFormOpened?: () => void;
  /** "compact" variant: Edit/Reject navigate to Needs-you instead of opening in place. */
  onNavigateToForm?: (form: ProposalCardForm) => void;
  /** A remote change closed this card's open form (proposal-cards.md#cards "Remote changes while a form is open"). */
  onFormForceClosed?: () => void;
  /** "full" variant only: the Needs-you item count to show in the decision toast's "Next" line. */
  computeNeedsYouCount?: () => number;
};

type TFn = ReturnType<typeof useTranslation>["t"];
type ToastFn = ReturnType<typeof useToast>["toast"];

function nextLine(t: TFn, computeNeedsYouCount?: () => number) {
  if (!computeNeedsYouCount) return undefined;
  const count = computeNeedsYouCount();
  return count === 0 ? t("coordinator:toastNextNone") : t("coordinator:toastNextCount", { count });
}

type OutcomeContext = {
  t: TFn;
  toast: ToastFn;
  variant: ProposalCardVariant;
  stepNameByWorkflowStep: Map<string, string>;
  computeNeedsYouCount?: () => number;
  setServerError: (error: EditFormServerError | null) => void;
  setOpenForm: (form: ProposalCardForm | null) => void;
  resolveCardLabel: (proposal: Proposal) => Promise<string>;
};

async function toastApproved(proposal: Proposal, ctx: OutcomeContext) {
  const spec = effectiveProposalSpec(proposal);
  const step = resolveStepName(spec, ctx.stepNameByWorkflowStep);
  const card = await ctx.resolveCardLabel(proposal);
  ctx.toast({
    title: ctx.t("coordinator:toastApproved", { card, step }),
    description: nextLine(ctx.t, ctx.computeNeedsYouCount),
    variant: "success",
  });
}

/**
 * Applies one decision outcome per the decision-outcomes table
 * (docs/specs/coordinator/system-design/proposal-cards.md#cards "Decision
 * outcomes"): toast copy, form close/keep-open, and server-error display.
 * Extracted from ProposalCard to keep the component's own branching low.
 */
function applyDecisionOutcome(
  outcome: ProposalDecisionOutcome,
  plainApprove: boolean,
  ctx: OutcomeContext,
) {
  const { t, toast, variant, computeNeedsYouCount, setServerError, setOpenForm } = ctx;
  switch (outcome.kind) {
    case "decided": {
      setServerError(null);
      setOpenForm(null);
      if (outcome.proposal.status === "failed") return;
      if (outcome.proposal.status === "approved") {
        void toastApproved(outcome.proposal, ctx);
      } else if (outcome.proposal.status === "rejected") {
        toast({
          title: t("coordinator:toastRejected"),
          description: nextLine(t, computeNeedsYouCount),
          variant: "success",
        });
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

/** True only for a remote decision arriving while this card's own form is open and it isn't the one deciding it. */
function shouldForceCloseForm(
  prevStatus: Proposal["status"],
  status: Proposal["status"],
  hasOpenForm: boolean,
  busy: boolean,
): boolean {
  if (prevStatus === "approving" || status !== "approving") return false;
  return hasOpenForm && !busy;
}

type ProposalCardActionsProps = {
  showActions: boolean;
  openForm: ProposalCardForm | null;
  variant: ProposalCardVariant;
  busy: boolean;
  isStaleApproving: boolean;
  workspaceId: string;
  effSpec: ProposalSpec;
  serverError: EditFormServerError | null;
  approveButtonRef: RefObject<HTMLButtonElement | null>;
  editButtonRef: RefObject<HTMLButtonElement | null>;
  rejectButtonRef: RefObject<HTMLButtonElement | null>;
  onApproveClick: () => void;
  onEditClick: () => void;
  onRejectClick: () => void;
  onApproveWithEdits: (edits: ApproveProposalEdits) => void;
  onRejectConfirm: (reason: string | undefined) => void;
  onCancelEdit: () => void;
  onCancelReject: () => void;
};

/** Approve/Edit/Reject buttons, or whichever form is open. Split out to keep ProposalCard's own branching low. */
function ProposalCardActions(props: ProposalCardActionsProps) {
  const { t } = useTranslation();
  if (!props.showActions) return null;
  if (props.isStaleApproving) {
    return (
      <div className="flex flex-wrap gap-2">
        <Button
          ref={props.approveButtonRef}
          size="sm"
          disabled={props.busy}
          onClick={props.onApproveClick}
          className="min-h-11 sm:min-h-0"
        >
          {props.busy && <Spinner aria-hidden className="mr-1.5" />}
          {t("coordinator:retry")}
        </Button>
      </div>
    );
  }
  if (props.variant === "full" && props.openForm === "edit") {
    return (
      <EditForm
        workspaceId={props.workspaceId}
        spec={props.effSpec}
        busy={props.busy}
        serverError={props.serverError}
        onApprove={props.onApproveWithEdits}
        onCancel={props.onCancelEdit}
      />
    );
  }
  if (props.variant === "full" && props.openForm === "reject") {
    return (
      <RejectForm
        busy={props.busy}
        serverError={props.serverError}
        onConfirm={props.onRejectConfirm}
        onCancel={props.onCancelReject}
      />
    );
  }
  if (props.openForm) return null;
  return (
    <div className="flex flex-wrap gap-2">
      <Button
        ref={props.approveButtonRef}
        size="sm"
        disabled={props.busy}
        onClick={props.onApproveClick}
        className="min-h-11 sm:min-h-0"
      >
        {props.busy && <Spinner aria-hidden className="mr-1.5" />}
        {t("coordinator:approve")}
      </Button>
      <Button
        ref={props.editButtonRef}
        size="sm"
        variant="outline"
        disabled={props.busy}
        onClick={props.onEditClick}
        className="min-h-11 sm:min-h-0"
      >
        {t("coordinator:edit")}
      </Button>
      <Button
        ref={props.rejectButtonRef}
        size="sm"
        variant="outline"
        disabled={props.busy}
        onClick={props.onRejectClick}
        className="min-h-11 sm:min-h-0"
      >
        {t("coordinator:reject")}
      </Button>
    </div>
  );
}

// eslint-disable-next-line max-lines-per-function -- one component owns state, effects and rendering wiring; branching itself lives in applyDecisionOutcome/shouldForceCloseForm/ProposalCardActions
export function ProposalCard({
  proposal,
  variant,
  canManage,
  workspaceId,
  coordinatorId,
  workflowNameById,
  stepNameByWorkflowStep,
  coordinatorName,
  autoOpenForm,
  onAutoFormOpened,
  onNavigateToForm,
  onFormForceClosed,
  computeNeedsYouCount,
}: ProposalCardProps) {
  const { t } = useTranslation();
  const { toast } = useToast();
  const now = useNowTick();
  const decision = useProposalDecision(workspaceId, coordinatorId, proposal.id);
  const [openForm, setOpenForm] = useState<ProposalCardForm | null>(null);
  const [serverError, setServerError] = useState<EditFormServerError | null>(null);
  const [pendingFocusReturn, setPendingFocusReturn] = useState<ProposalCardForm | null>(null);
  const { label: cardLabel, resolveLabel: resolveCardLabel } = useApprovedCardLabel(proposal);
  const approveButtonRef = useRef<HTMLButtonElement>(null);
  const editButtonRef = useRef<HTMLButtonElement>(null);
  const rejectButtonRef = useRef<HTMLButtonElement>(null);

  const autoOpenedRef = useRef(false);
  useEffect(() => {
    // `autoOpenForm` can still be null on this card's first mount (its item
    // reaching the Needs-you list does not imply every other coordinator
    // input has loaded yet) and only turn truthy once every input has, on a
    // later render of an already-mounted card, so this reacts to the prop
    // rather than running mount-once.
    if (!autoOpenForm || autoOpenedRef.current) return;
    autoOpenedRef.current = true;
    setOpenForm(autoOpenForm);
    onAutoFormOpened?.();
  }, [autoOpenForm, onAutoFormOpened]);

  const prevStatusRef = useRef(proposal.status);
  useEffect(() => {
    if (
      shouldForceCloseForm(prevStatusRef.current, proposal.status, openForm !== null, decision.busy)
    ) {
      setOpenForm(null);
      setServerError(null);
      onFormForceClosed?.();
    }
    prevStatusRef.current = proposal.status;
    // Only reacts to the proposal's own status changing (a remote decision).
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [proposal.status]);

  useEffect(() => {
    if (!pendingFocusReturn) return;
    (pendingFocusReturn === "edit" ? editButtonRef : rejectButtonRef).current?.focus();
    setPendingFocusReturn(null);
  }, [pendingFocusReturn]);

  function closeForm(returnFocusTo: "edit" | "reject") {
    setOpenForm(null);
    setServerError(null);
    // The Edit/Reject buttons are unmounted while a form is open, so their
    // refs are null until this closes and the buttons remount; the effect
    // above focuses them once that render lands.
    setPendingFocusReturn(returnFocusTo);
  }

  const outcomeCtx: OutcomeContext = {
    t,
    toast,
    variant,
    stepNameByWorkflowStep,
    computeNeedsYouCount,
    setServerError,
    setOpenForm,
    resolveCardLabel,
  };

  async function handleApprove(edits?: ApproveProposalEdits) {
    const outcome = await decision.approve(edits);
    applyDecisionOutcome(outcome, edits === undefined, outcomeCtx);
  }

  async function handleReject(reason?: string) {
    const outcome = await decision.reject(reason);
    applyDecisionOutcome(outcome, false, outcomeCtx);
  }

  function openOrNavigateToForm(form: ProposalCardForm) {
    if (variant === "compact") {
      onNavigateToForm?.(form);
      return;
    }
    setServerError(null);
    setOpenForm(form);
  }

  const effSpec = effectiveProposalSpec(proposal);
  const isStaleApproving =
    proposal.status === "approving" && isApprovalClaimStale(proposal.claimed_at, now);
  const statusLine =
    proposal.status === "approved"
      ? approvedStatusLine(cardLabel, t)
      : proposalStatusLine(proposal, t, now);
  const showActions =
    canManage &&
    (proposal.status === "pending" || proposal.status === "failed" || isStaleApproving);
  const label = workflowStepLabel(effSpec, workflowNameById, stepNameByWorkflowStep);

  return (
    <div className="space-y-2" data-testid={`proposal-card-${proposal.id}`}>
      <div role="status" aria-live="polite" className="sr-only">
        {decision.busy ? t("coordinator:proposalWorking") : statusLine}
      </div>
      <p className="text-sm">{statusLine}</p>
      <p className="text-sm font-medium">{effSpec.title}</p>
      {variant === "full" && <CardDescription>{effSpec.description}</CardDescription>}
      <CardDescription>{label}</CardDescription>
      {variant === "full" && (
        <>
          <CardDescription>
            {t("coordinator:proposedBy", { name: coordinatorName ?? "" })}
          </CardDescription>
          <CardDescription>{t("coordinator:policyProposeOnly")}</CardDescription>
        </>
      )}
      {variant === "compact" && openForm === null && serverError && (
        <p role="alert" className="text-destructive text-xs/relaxed font-normal">
          {serverError.message}
        </p>
      )}
      <ProposalCardActions
        showActions={showActions}
        openForm={openForm}
        variant={variant}
        busy={decision.busy}
        isStaleApproving={isStaleApproving}
        workspaceId={workspaceId}
        effSpec={effSpec}
        serverError={serverError}
        approveButtonRef={approveButtonRef}
        editButtonRef={editButtonRef}
        rejectButtonRef={rejectButtonRef}
        onApproveClick={() => void handleApprove()}
        onEditClick={() => openOrNavigateToForm("edit")}
        onRejectClick={() => openOrNavigateToForm("reject")}
        onApproveWithEdits={(edits) => void handleApprove(edits)}
        onRejectConfirm={(reason) => void handleReject(reason)}
        onCancelEdit={() => closeForm("edit")}
        onCancelReject={() => closeForm("reject")}
      />
    </div>
  );
}
