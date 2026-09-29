"use client";

import { useEffect, useRef, useState, type RefObject } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { CardDescription } from "@kandev/ui/card";
import { Spinner } from "@kandev/ui/spinner";
import {
  isCreateTaskProposal,
  type ApproveProposalEdits,
  type CreateTaskProposal,
  type KindProposal,
  type MessageSpec,
  type ProposalSpec,
  type StoredProposal,
} from "@/lib/api/domains/coordinator-api";
import type { AttentionTask } from "@/lib/coordinator/attention";
import type { StandingOrder } from "@/lib/api/domains/coordinator-api";
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
import { KindBody } from "./kind-body";
import { MessageEditForm } from "./message-edit-form";
import { kindApprovedToast, kindFailureText, policyLineText } from "./outcome-copy";
import { usePhase2CardContext } from "./phase2-context";
import { RejectForm } from "./reject-form";
import { ReplySection } from "./reply-section";
import { RevisedNote } from "./revised-note";
import { ShapedBy } from "./shaped-by";
import { useApprovedCardLabel } from "./use-approved-card-label";
import { useTargetTask } from "./use-target-task";

export type ProposalCardVariant = "full" | "compact";
export type ProposalCardForm = "edit" | "reject";

export type ProposalCardProps = {
  proposal: StoredProposal;
  /** "full" is the Needs-you card (description, attribution, policy line, inline forms); "compact" is the chat card. */
  variant: ProposalCardVariant;
  canManage: boolean;
  workspaceId: string;
  coordinatorId: string;
  workflowNameById: Map<string, string>;
  stepNameByWorkflowStep: Map<string, string>;
  /** Open tasks of the workspace, for a resume, message or move card's target; absent on the compact chat card, which reads the task itself. */
  openTasksById?: Map<string, AttentionTask>;
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
  /** "full" variant only: a reply request started, so the Needs you list keeps this item. */
  onReplyStarted?: () => void;
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
  resolveCardLabel: (proposal: StoredProposal) => Promise<string>;
  /** The target task label of a resume, message or move card. */
  targetLabel: string;
  setPolicyDenied: (denied: boolean) => void;
  offerReject?: (offer: { proposalId: string; reason: string }) => void;
};

async function toastApproved(proposal: StoredProposal, ctx: OutcomeContext) {
  let title: string;
  if (isCreateTaskProposal(proposal)) {
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

const ACTIONABLE: ReadonlySet<StoredProposal["status"]> = new Set(["pending", "failed"]);

function splitProposal(proposal: StoredProposal) {
  const createProposal: CreateTaskProposal | null = isCreateTaskProposal(proposal)
    ? proposal
    : null;
  const kindProposal: KindProposal | null = createProposal ? null : (proposal as KindProposal);
  const kindSpec = kindProposal ? (kindProposal.final_spec ?? kindProposal.spec) : null;
  return { createProposal, kindProposal, kindSpec };
}

function messageTextOf(kindSpec: KindProposal["spec"] | null): string {
  return kindSpec ? ((kindSpec as MessageSpec).text ?? "") : "";
}

function cardStatusLine(
  proposal: StoredProposal,
  kindProposal: KindProposal | null,
  approvedLabel: string,
  t: TFn,
  now: number,
): string {
  if (proposal.status === "approved") return approvedStatusLine(approvedLabel, t);
  if (proposal.status === "failed" && kindProposal) return kindFailureText(kindProposal, t);
  return proposalStatusLine(proposal, t, now);
}

function editableForm(
  createProposal: unknown,
  kindProposal: KindProposal | null,
): "create" | "message" | null {
  if (createProposal) return "create";
  return kindProposal?.kind === "message" ? "message" : null;
}

function kindStepName(proposal: KindProposal, names: Map<string, string>): string {
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
function applyDecisionOutcome(
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

/** True only for a remote decision arriving while this card's own form is open and it isn't the one deciding it. */
function shouldForceCloseForm(
  prevStatus: StoredProposal["status"],
  status: StoredProposal["status"],
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
  editable: "create" | "message" | null;
  createSpec: ProposalSpec | null;
  messageText: string;
  policyDenied: boolean;
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

function ActionButtons(props: ProposalCardActionsProps) {
  const { t } = useTranslation();
  const mayApprove = !props.policyDenied || props.isStaleApproving;
  return (
    <div className="flex flex-wrap gap-2">
      {mayApprove && (
        <Button
          ref={props.approveButtonRef}
          size="sm"
          disabled={props.busy}
          onClick={props.onApproveClick}
          className="min-h-11 sm:min-h-0"
        >
          {props.busy && <Spinner aria-hidden className="mr-1.5" />}
          {props.isStaleApproving ? t("coordinator:retry") : t("coordinator:approve")}
        </Button>
      )}
      {mayApprove && props.editable && !props.isStaleApproving && (
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
      )}
      {!props.isStaleApproving && (
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
      )}
    </div>
  );
}

function OpenForm(props: ProposalCardActionsProps) {
  if (props.variant !== "full") return null;
  if (props.openForm === "reject") {
    return (
      <RejectForm
        busy={props.busy}
        serverError={props.serverError}
        onConfirm={props.onRejectConfirm}
        onCancel={props.onCancelReject}
      />
    );
  }
  if (props.editable === "message") {
    return (
      <MessageEditForm
        text={props.messageText}
        busy={props.busy}
        serverError={props.serverError}
        onApprove={props.onApproveWithEdits}
        onCancel={props.onCancelEdit}
      />
    );
  }
  if (props.editable === "create" && props.createSpec) {
    return (
      <EditForm
        workspaceId={props.workspaceId}
        spec={props.createSpec}
        busy={props.busy}
        serverError={props.serverError}
        onApprove={props.onApproveWithEdits}
        onCancel={props.onCancelEdit}
      />
    );
  }
  return null;
}

/** Approve/Edit/Reject buttons, or whichever form is open. */
function ProposalCardActions(props: ProposalCardActionsProps) {
  if (!props.showActions) return null;
  if (props.isStaleApproving) return <ActionButtons {...props} />;
  if (props.openForm) return <OpenForm {...props} />;
  return <ActionButtons {...props} />;
}

type CardBodyProps = {
  proposal: StoredProposal;
  variant: ProposalCardVariant;
  createProposal: CreateTaskProposal | null;
  createSpec: ProposalSpec | null;
  kindProposal: KindProposal | null;
  specLabel: string | null;
  openTasksById?: Map<string, AttentionTask>;
  stepNameByWorkflowStep: Map<string, string>;
  coordinatorName?: string;
  orders: StandingOrder[] | undefined;
  policyDenied: boolean;
  serverError: EditFormServerError | null;
};

function CardBody(props: CardBodyProps) {
  const { t } = useTranslation();
  const { proposal, variant, createProposal, createSpec, kindProposal } = props;
  return (
    <>
      {createSpec && (
        <>
          <p className="text-sm font-medium">{createSpec.title}</p>
          {variant === "full" && <CardDescription>{createSpec.description}</CardDescription>}
          <CardDescription>{props.specLabel}</CardDescription>
        </>
      )}
      {kindProposal && (
        <KindBody
          proposal={kindProposal}
          openTasksById={props.openTasksById}
          stepNameByWorkflowStep={props.stepNameByWorkflowStep}
        />
      )}
      {createProposal && createProposal.starts_agent === true && (
        <CardDescription>{t("coordinator:startsAgentLine")}</CardDescription>
      )}
      {variant === "full" && (
        <>
          <CardDescription>
            {t("coordinator:proposedBy", { name: props.coordinatorName ?? "" })}
          </CardDescription>
          <CardDescription>
            {kindProposal ? policyLineText(proposal, t) : t("coordinator:policyProposeOnly")}
          </CardDescription>
          <ShapedBy ids={proposal.standing_order_ids} orders={props.orders} />
        </>
      )}
      {props.policyDenied && (
        <p role="alert" className="text-destructive text-xs/relaxed font-normal">
          {t("coordinator:policyDeniedLine")}
        </p>
      )}
      {variant === "compact" && props.serverError && (
        <p role="alert" className="text-destructive text-xs/relaxed font-normal">
          {props.serverError.message}
        </p>
      )}
    </>
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
  openTasksById,
  coordinatorName,
  autoOpenForm,
  onAutoFormOpened,
  onNavigateToForm,
  onFormForceClosed,
  computeNeedsYouCount,
  onReplyStarted,
}: ProposalCardProps) {
  const { t } = useTranslation();
  const { toast } = useToast();
  const now = useNowTick();
  const decision = useProposalDecision(workspaceId, coordinatorId, proposal.id);
  const [openForm, setOpenForm] = useState<ProposalCardForm | null>(null);
  const [serverError, setServerError] = useState<EditFormServerError | null>(null);
  const [pendingFocusReturn, setPendingFocusReturn] = useState<ProposalCardForm | null>(null);
  const { label: cardLabel, resolveLabel: resolveCardLabel } = useApprovedCardLabel(proposal);
  const phase2 = usePhase2CardContext();
  const [policyDenied, setPolicyDenied] = useState(false);
  const { createProposal, kindProposal, kindSpec } = splitProposal(proposal);
  const target = useTargetTask(kindSpec?.task_id ?? "", kindSpec ? openTasksById : new Map());
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
    targetLabel: target.label,
    setPolicyDenied,
    offerReject: phase2.offerReject,
  };

  async function handleApprove(edits?: ApproveProposalEdits) {
    const outcome = await decision.approve(edits);
    applyDecisionOutcome(outcome, edits === undefined, outcomeCtx);
  }

  async function handleReject(reason?: string) {
    const outcome = await decision.reject(reason);
    applyDecisionOutcome(outcome, false, outcomeCtx, reason);
  }

  function openOrNavigateToForm(form: ProposalCardForm) {
    if (variant === "compact") {
      onNavigateToForm?.(form);
      return;
    }
    setServerError(null);
    setOpenForm(form);
  }

  const createSpec = createProposal ? effectiveProposalSpec(createProposal) : null;
  const editable = editableForm(createProposal, kindProposal);
  const isStaleApproving =
    proposal.status === "approving" && isApprovalClaimStale(proposal.claimed_at, now);
  const statusLine = cardStatusLine(
    proposal,
    kindProposal,
    kindProposal ? target.label : cardLabel,
    t,
    now,
  );
  const showActions = canManage && (ACTIONABLE.has(proposal.status) || isStaleApproving);
  const label = createSpec
    ? workflowStepLabel(createSpec, workflowNameById, stepNameByWorkflowStep)
    : null;

  return (
    <div className="space-y-2" data-testid={`proposal-card-${proposal.id}`}>
      <div role="status" aria-live="polite" className="sr-only">
        {decision.busy ? t("coordinator:proposalWorking") : statusLine}
      </div>
      <p className="text-sm">{statusLine}</p>
      <CardBody
        proposal={proposal}
        variant={variant}
        createProposal={createProposal}
        createSpec={createSpec}
        kindProposal={kindProposal}
        specLabel={label}
        openTasksById={openTasksById}
        stepNameByWorkflowStep={stepNameByWorkflowStep}
        coordinatorName={coordinatorName}
        orders={phase2.orders}
        policyDenied={policyDenied}
        serverError={openForm === null ? serverError : null}
      />
      {proposal.in_reply_to && (
        <RevisedNote
          workspaceId={workspaceId}
          coordinatorId={coordinatorId}
          inReplyTo={proposal.in_reply_to}
        />
      )}
      <ProposalCardActions
        showActions={showActions}
        openForm={openForm}
        variant={variant}
        busy={decision.busy}
        isStaleApproving={isStaleApproving}
        workspaceId={workspaceId}
        editable={editable}
        createSpec={createSpec}
        messageText={messageTextOf(kindSpec)}
        policyDenied={policyDenied}
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
      {openForm === null && !isStaleApproving && (
        <ReplySection
          proposal={proposal}
          canManage={canManage}
          workspaceId={workspaceId}
          coordinatorId={coordinatorId}
          onReplyStarted={onReplyStarted}
        />
      )}
    </div>
  );
}
