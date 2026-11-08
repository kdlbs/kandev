"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { CardDescription } from "@kandev/ui/card";
import { Spinner } from "@kandev/ui/spinner";
import { useFeature } from "@/hooks/domains/features/use-feature";
import {
  useProposalDecision,
  type ProposalDecisionOutcome,
} from "@/hooks/domains/coordinator/use-proposal-decision";
import { useToast } from "@/components/toast-provider";
import type {
  AnswerProposal,
  AnswerSpec,
  ApproveProposalEdits,
  StoredProposal,
} from "@/lib/api/domains/coordinator-api";
import type { AttentionTask } from "@/lib/coordinator/attention";
import {
  approvedStatusLine,
  isApprovalClaimStale,
  proposalStatusLine,
} from "@/lib/coordinator/proposal-text";
import { useNowTick } from "../use-now-tick";
import { AnswerCardBody, answerSpecOf } from "./answer-card-body";
import { AnswerDraftEditor } from "./answer-draft-editor";
import type { EditFormServerError } from "./edit-form";
import { answerRefusedKey, kindFailureText } from "./outcome-copy";
import { RejectForm } from "./reject-form";
import { ReplySection } from "./reply-section";
import { RevisedNote } from "./revised-note";
import { useAnswerCurrency, type AnswerCurrency } from "./use-answer-currency";
import { useTargetTask } from "./use-target-task";

type Form = "edit" | "reject";

export type AnswerProposalCardProps = {
  proposal: AnswerProposal;
  variant: "full" | "compact";
  canManage: boolean;
  workspaceId: string;
  coordinatorId: string;
  openTasksById?: Map<string, AttentionTask>;
  coordinatorName?: string;
  autoOpenForm?: Form | null;
  onAutoFormOpened?: () => void;
  onNavigateToForm?: (form: Form) => void;
  computeNeedsYouCount?: () => number;
  onReplyStarted?: () => void;
};

const RETRY_CODES: ReadonlySet<string> = new Set(["resolution_in_progress", "not_delivered"]);
const EDIT_CODES: ReadonlySet<string> = new Set(["invalid_draft", "option_not_offered"]);

/** A refused answer is a rejected row with a refusal code and no deciding person. */
export function isRefusedAnswerCard(proposal: AnswerProposal): boolean {
  return (
    proposal.status === "rejected" &&
    !proposal.decided_by &&
    (proposal.error === "stale" || proposal.error === "task_archived")
  );
}

type Actions = { approve: boolean; retry: boolean; edit: boolean; reject: boolean };

function actionsFor(
  proposal: AnswerProposal,
  stale: boolean,
  flagOn: boolean,
  policyDenied: boolean,
): Actions {
  const none = { approve: false, retry: false, edit: false, reject: false };
  if (stale) return { ...none, retry: flagOn };
  const code = proposal.error ?? "";
  if (proposal.status === "pending") {
    const may = flagOn && !policyDenied;
    return { approve: may, retry: false, edit: may, reject: true };
  }
  if (proposal.status !== "failed") return none;
  if (RETRY_CODES.has(code)) return { ...none, retry: flagOn && !policyDenied, reject: true };
  if (EDIT_CODES.has(code)) return { ...none, edit: flagOn && !policyDenied, reject: true };
  return { ...none, reject: true };
}

function statusLineOf(
  proposal: AnswerProposal,
  label: string,
  t: ReturnType<typeof useTranslation>["t"],
  now: number,
): string {
  if (isRefusedAnswerCard(proposal)) return t(answerRefusedKey(proposal.error ?? null));
  if (proposal.status === "failed") return kindFailureText(proposal, t);
  if (proposal.status === "approved") return approvedStatusLine(label, t);
  return proposalStatusLine(proposal, t, now);
}

function useAnswerDecision(props: AnswerProposalCardProps, setForm: (f: Form | null) => void) {
  const { t } = useTranslation();
  const { toast } = useToast();
  const decision = useProposalDecision(props.workspaceId, props.coordinatorId, props.proposal.id);
  const [serverError, setServerError] = useState<EditFormServerError | null>(null);
  const [policyDenied, setPolicyDenied] = useState(false);

  function apply(outcome: ProposalDecisionOutcome, inlineEdit: boolean) {
    switch (outcome.kind) {
      case "decided":
        setServerError(null);
        setForm(null);
        if (outcome.proposal.status === "approved") {
          toast({ title: t("coordinator:toastApprovedAnswer"), variant: "success" });
        } else if (outcome.proposal.status === "rejected") {
          toast({ title: t("coordinator:toastRejected"), variant: "success" });
        }
        return;
      case "validation":
        setServerError({ message: outcome.message, field: outcome.field });
        if (inlineEdit && props.variant === "full") setForm("edit");
        return;
      case "policy_denied":
        setForm(null);
        setServerError(null);
        setPolicyDenied(true);
        return;
      case "conflict":
        setForm(null);
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

  return {
    busy: decision.busy,
    serverError,
    setServerError,
    policyDenied,
    approve: async (edits?: ApproveProposalEdits) =>
      apply(await decision.approve(edits), edits === undefined),
    reject: async (reason?: string) => apply(await decision.reject(reason), false),
  };
}

function ActionRow({
  actions,
  busy,
  onApprove,
  onEdit,
  onReject,
}: {
  actions: Actions;
  busy: boolean;
  onApprove: () => void;
  onEdit: () => void;
  onReject: () => void;
}) {
  const { t } = useTranslation();
  const cls = "min-h-11 sm:min-h-0";
  return (
    <div className="flex flex-wrap items-center gap-2">
      {(actions.approve || actions.retry) && (
        <Button size="sm" disabled={busy} onClick={onApprove} className={cls}>
          {busy && <Spinner aria-hidden className="mr-1.5" />}
          {actions.retry ? t("coordinator:retry") : t("coordinator:approve")}
        </Button>
      )}
      {actions.edit && (
        <Button size="sm" variant="outline" disabled={busy} onClick={onEdit} className={cls}>
          {t("coordinator:edit")}
        </Button>
      )}
      {actions.reject && (
        <Button size="sm" variant="outline" disabled={busy} onClick={onReject} className={cls}>
          {t("coordinator:reject")}
        </Button>
      )}
    </div>
  );
}

type FormsProps = {
  form: Form | null;
  variant: "full" | "compact";
  showActions: boolean;
  spec: AnswerSpec;
  currency: AnswerCurrency;
  busy: boolean;
  serverError: EditFormServerError | null;
  onApprove: (edits: ApproveProposalEdits) => void;
  onReject: (reason: string | undefined) => void;
  onCancel: () => void;
};

/** The inline Edit and Reject forms; only the full (Needs-you) card hosts them. */
function AnswerForms(props: FormsProps) {
  const { t } = useTranslation();
  if (props.variant !== "full" || !props.showActions) return null;
  if (props.form === "reject") {
    return (
      <RejectForm
        busy={props.busy}
        serverError={props.serverError}
        onConfirm={props.onReject}
        onCancel={props.onCancel}
      />
    );
  }
  if (props.form !== "edit") return null;
  return (
    <>
      {props.currency === "stale" && (
        <p role="status" className="text-xs" data-testid="answer-out-of-date">
          {t("coordinator:answerOutOfDate")}
        </p>
      )}
      <AnswerDraftEditor
        spec={props.spec}
        draft={props.spec.draft}
        busy={props.busy}
        serverError={props.serverError}
        onApprove={props.onApprove}
        onCancel={props.onCancel}
      />
    </>
  );
}

type NoticesProps = {
  proposal: AnswerProposal;
  variant: "full" | "compact";
  coordinatorName?: string;
  flagOff: boolean;
  policyDenied: boolean;
  serverError: EditFormServerError | null;
};

function AnswerNotices(props: NoticesProps) {
  const { t } = useTranslation();
  const { proposal } = props;
  return (
    <>
      {proposal.status === "failed" && proposal.error === "outcome_unknown" && (
        <p role="alert" className="text-xs" data-testid="answer-may-have-been-delivered">
          {t("coordinator:answerMayHaveBeenDelivered")}
        </p>
      )}
      {props.flagOff && (
        <CardDescription>{t("coordinator:answerFlagOffRejectOnly")}</CardDescription>
      )}
      {props.policyDenied && (
        <p role="alert" className="text-destructive text-xs/relaxed font-normal">
          {t("coordinator:policyDeniedLine")}
        </p>
      )}
      {props.variant === "full" && (
        <CardDescription>
          {t("coordinator:proposedBy", { name: props.coordinatorName ?? "" })}
        </CardDescription>
      )}
      {props.variant === "compact" && props.serverError && (
        <p role="alert" className="text-destructive text-xs/relaxed font-normal">
          {props.serverError.message}
        </p>
      )}
    </>
  );
}

export function AnswerProposalCard(props: AnswerProposalCardProps) {
  const { proposal, variant, canManage, workspaceId, coordinatorId } = props;
  const { t } = useTranslation();
  const now = useNowTick();
  const flagOn = useFeature("coordinatorAnswer");
  const spec = answerSpecOf(proposal);
  const [form, setForm] = useState<Form | null>(props.autoOpenForm ?? null);
  const d = useAnswerDecision(props, setForm);
  const target = useTargetTask(spec.task_id, props.openTasksById);
  const currency = useAnswerCurrency(workspaceId, coordinatorId, spec, form === "edit");
  const stale = proposal.status === "approving" && isApprovalClaimStale(proposal.claimed_at, now);
  const actions = actionsFor(proposal, stale, flagOn, d.policyDenied);
  const open = proposal.status === "pending" || proposal.status === "failed" || stale;
  const showActions = canManage && open;
  const statusLine = statusLineOf(proposal, target.label, t, now);

  function openForm(next: Form) {
    if (variant === "compact") {
      props.onNavigateToForm?.(next);
      return;
    }
    d.setServerError(null);
    setForm(next);
  }
  function closeForm() {
    setForm(null);
    d.setServerError(null);
  }

  return (
    <div className="space-y-2" data-testid={`proposal-card-${proposal.id}`}>
      <div role="status" aria-live="polite" className="sr-only">
        {d.busy ? t("coordinator:proposalWorking") : statusLine}
      </div>
      <p className="text-sm">{statusLine}</p>
      <AnswerCardBody proposal={proposal} label={target.label} known={target.known} />
      <AnswerNotices
        proposal={proposal}
        variant={variant}
        coordinatorName={props.coordinatorName}
        flagOff={!flagOn && showActions}
        policyDenied={d.policyDenied}
        serverError={d.serverError}
      />
      {proposal.in_reply_to && (
        <RevisedNote
          workspaceId={workspaceId}
          coordinatorId={coordinatorId}
          inReplyTo={proposal.in_reply_to}
        />
      )}
      <AnswerForms
        form={form}
        variant={variant}
        showActions={showActions}
        spec={spec}
        currency={currency}
        busy={d.busy}
        serverError={d.serverError}
        onApprove={(edits) => void d.approve(edits)}
        onReject={(reason) => void d.reject(reason)}
        onCancel={closeForm}
      />
      {showActions && form === null && (
        <ActionRow
          actions={actions}
          busy={d.busy}
          onApprove={() => void d.approve()}
          onEdit={() => openForm("edit")}
          onReject={() => openForm("reject")}
        />
      )}
      {form === null && !stale && (
        <ReplySection
          proposal={proposal as StoredProposal}
          canManage={canManage}
          workspaceId={workspaceId}
          coordinatorId={coordinatorId}
          onReplyStarted={props.onReplyStarted}
        />
      )}
    </div>
  );
}
