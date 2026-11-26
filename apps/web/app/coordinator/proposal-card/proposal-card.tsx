"use client";

import { useEffect, useRef, useState, type RefObject } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { CardDescription } from "@kandev/ui/card";
import { Spinner } from "@kandev/ui/spinner";
import {
  isCreateTaskProposal,
  isAnswerProposal,
  isImprovementProposal,
  type ApproveProposalEdits,
  type CreateTaskProposal,
  type ImprovementProposal,
  isMergeProposal,
  type KindProposal,
  type MergeProposal,
  type MessageSpec,
  type ProposalSpec,
  type StoredProposal,
} from "@/lib/api/domains/coordinator-api";
import { AnswerProposalCard } from "./answer-card";
import { MergeProposalCard } from "./merge-card";
import { RunsWithLine } from "./runs-with-line";
import type { AttentionTask } from "@/lib/coordinator/attention";
import type { StandingOrder } from "@/lib/api/domains/coordinator-api";
import { useProposalDecision } from "@/hooks/domains/coordinator/use-proposal-decision";
import {
  approvedStatusLine,
  effectiveProposalSpec,
  isApprovalClaimStale,
  proposalStatusLine,
  workflowStepLabel,
} from "@/lib/coordinator/proposal-text";
import { useToast } from "@/components/toast-provider";
import {
  applyDecisionOutcome,
  type OutcomeContext,
  type TFn,
  type ProposalCardForm,
  type ProposalCardVariant,
} from "./decision-outcome";
import { useNowTick } from "../use-now-tick";
import { EditForm, type EditFormServerError } from "./edit-form";
import {
  ImprovementBody,
  improvementGateOf,
  improvementApprovedLine,
  improvementFailedLine,
} from "./improvement-card";
import { KindBody } from "./kind-body";
import { MessageEditForm } from "./message-edit-form";
import { kindFailureText, policyLineText } from "./outcome-copy";
import { usePhase2CardContext } from "./phase2-context";
import { RejectForm } from "./reject-form";
import { ReplySection } from "./reply-section";
import { RevisedNote } from "./revised-note";
import { ShapedBy } from "./shaped-by";
import { useApprovedCardLabel } from "./use-approved-card-label";
import { useTargetTask } from "./use-target-task";

export type { ProposalCardForm, ProposalCardVariant } from "./decision-outcome";

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

type GenericCardProps = Omit<ProposalCardProps, "proposal"> & {
  proposal: Exclude<StoredProposal, MergeProposal>;
};

const ACTIONABLE: ReadonlySet<StoredProposal["status"]> = new Set(["pending", "failed"]);

function splitProposal(proposal: Exclude<StoredProposal, MergeProposal>) {
  const improvementProposal: ImprovementProposal | null = isImprovementProposal(proposal)
    ? proposal
    : null;
  const createProposal: CreateTaskProposal | null = isCreateTaskProposal(proposal)
    ? proposal
    : null;
  const kindProposal: KindProposal | null =
    createProposal || improvementProposal ? null : (proposal as KindProposal);
  const kindSpec = kindProposal ? (kindProposal.final_spec ?? kindProposal.spec) : null;
  return { createProposal, kindProposal, improvementProposal, kindSpec };
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
  if (isImprovementProposal(proposal)) {
    if (proposal.status === "approved") return improvementApprovedLine(proposal, t);
    if (proposal.status === "failed") return improvementFailedLine(t);
    return proposalStatusLine(proposal, t, now);
  }
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
  /** Set on an improvement card: Approve/Retry stay disabled until its diff was shown. */
  improvementGate: { diffShown: boolean } | null;
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
  const gate = props.improvementGate;
  const gated = gate !== null && !gate.diffShown;
  let approveLabel = gate ? t("coordinator:improvementApprove") : t("coordinator:approve");
  if (props.isStaleApproving) approveLabel = t("coordinator:retry");
  return (
    <div className="flex flex-wrap items-center gap-2">
      {mayApprove && (
        <Button
          ref={props.approveButtonRef}
          size="sm"
          disabled={props.busy || gated}
          onClick={props.onApproveClick}
          className="min-h-11 sm:min-h-0"
        >
          {props.busy && <Spinner aria-hidden className="mr-1.5" />}
          {approveLabel}
        </Button>
      )}
      {mayApprove && gated && (
        <span className="text-xs text-muted-foreground" data-testid="improvement-show-first">
          {t("coordinator:improvementShowFirst")}
        </span>
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
  improvementProposal: ImprovementProposal | null;
  workspaceId: string;
  coordinatorId: string;
  diffShown: boolean;
  onShowDiff: () => void;
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
  const { proposal, variant, createProposal, createSpec, kindProposal, improvementProposal } =
    props;
  return (
    <>
      {createSpec && (
        <>
          <p className="text-sm font-medium">{createSpec.title}</p>
          {variant === "full" && <CardDescription>{createSpec.description}</CardDescription>}
          <CardDescription>{props.specLabel}</CardDescription>
          <RunsWithLine runsWith={proposal.runs_with} />
        </>
      )}
      {kindProposal && (
        <KindBody
          proposal={kindProposal}
          openTasksById={props.openTasksById}
          stepNameByWorkflowStep={props.stepNameByWorkflowStep}
        />
      )}
      {improvementProposal && (
        <ImprovementBody
          proposal={improvementProposal}
          workspaceId={props.workspaceId}
          coordinatorId={props.coordinatorId}
          openTasksById={props.openTasksById}
          shown={props.diffShown}
          onShow={props.onShowDiff}
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
          {!improvementProposal && (
            <CardDescription>
              {kindProposal
                ? policyLineText(
                    proposal as Exclude<StoredProposal, ImprovementProposal | MergeProposal>,
                    t,
                  )
                : t("coordinator:policyProposeOnly")}
            </CardDescription>
          )}
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
function GenericProposalCard({
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
}: GenericCardProps) {
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
  const { createProposal, kindProposal, improvementProposal, kindSpec } = splitProposal(proposal);
  const [diffShown, setDiffShown] = useState(false);
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
        improvementProposal={improvementProposal}
        workspaceId={workspaceId}
        coordinatorId={coordinatorId}
        diffShown={diffShown}
        onShowDiff={() => setDiffShown(true)}
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
        improvementGate={improvementGateOf(improvementProposal, diffShown)}
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

/** Answer proposals render their own card; every other kind shares the generic card. */
export function ProposalCard(props: ProposalCardProps) {
  if (isAnswerProposal(props.proposal)) {
    return <AnswerProposalCard {...props} proposal={props.proposal} />;
  }
  if (isMergeProposal(props.proposal)) {
    return <MergeProposalCard {...props} proposal={props.proposal} />;
  }
  return <GenericProposalCard {...props} proposal={props.proposal} />;
}
