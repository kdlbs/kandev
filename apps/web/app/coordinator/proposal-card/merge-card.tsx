"use client";

import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { CardDescription } from "@kandev/ui/card";
import { Skeleton } from "@kandev/ui/skeleton";
import { Spinner } from "@kandev/ui/spinner";
import { useToast } from "@/components/toast-provider";
import { useMergeDetail } from "@/hooks/domains/coordinator/use-merge-detail";
import {
  useProposalDecision,
  type ProposalDecisionOutcome,
} from "@/hooks/domains/coordinator/use-proposal-decision";
import type { MergeProposal } from "@/lib/api/domains/coordinator-api";
import type { MergeDetail } from "@/lib/api/domains/coordinator-merge-api";
import {
  conflictTri,
  mergeNoteText,
  mergeReasonText,
  mergeStatusText,
  parseChangeKey,
  shortSha,
} from "@/lib/coordinator/merge-text";
import { MergeConditions } from "../components/merge-conditions";
import { RejectForm } from "./reject-form";

export type MergeProposalCardProps = {
  proposal: MergeProposal;
  variant: "full" | "compact";
  canManage: boolean;
  workspaceId: string;
  coordinatorId: string;
  coordinatorName?: string;
  /** "compact" variant: Reject navigates to Needs you instead of opening in place. */
  onNavigateToForm?: (form: "edit" | "reject") => void;
};

const OPEN_FOR_DECISION = new Set(["pending", "failed"]);

function Title({ changeKey }: { changeKey: string }) {
  const { t } = useTranslation();
  const parsed = parseChangeKey(changeKey);
  return (
    <p className="text-sm font-medium break-words" data-testid="merge-card-title">
      {parsed
        ? t("coordinator:mergeCardTitle", { provider: parsed.provider, number: parsed.number })
        : t("coordinator:mergeCardTitleUnknown")}
    </p>
  );
}

function Facts({ proposal, detail }: { proposal: MergeProposal; detail: MergeDetail | null }) {
  const { t } = useTranslation();
  const spec = proposal.final_spec ?? proposal.spec;
  const head = detail?.head_sha ?? spec.head_sha;
  const method = detail?.method ?? spec.method;
  return (
    <CardDescription data-testid="merge-card-facts">
      {t("coordinator:mergeCardHead", { sha: shortSha(head) })} ·{" "}
      {t("coordinator:mergeCardMethod", { method })}
      {detail?.automatic ? ` · ${t("coordinator:mergeCardAutomatic")}` : ""}
    </CardDescription>
  );
}

function Notes({ detail }: { detail: MergeDetail | null }) {
  const { t } = useTranslation();
  if (!detail) return null;
  const note = detail.manager_only ? mergeNoteText(detail.note, t) : null;
  const reason = mergeReasonText(detail.reason, t);
  return (
    <>
      {note && (
        <p role="status" className="text-sm font-medium" data-testid="merge-card-note">
          {note}
        </p>
      )}
      {reason && (
        <p className="text-muted-foreground text-xs" data-testid="merge-card-reason">
          {t("coordinator:mergeCardNotReady", { reason })}
        </p>
      )}
    </>
  );
}

type LiveProps = {
  detail: MergeDetail | null;
  status: "loading" | "ready" | "error";
  canManage: boolean;
  onRetry: () => void;
};

function Live({ detail, status, canManage, onRetry }: LiveProps) {
  const { t } = useTranslation();
  if (status === "loading" && !detail) {
    return <Skeleton className="h-4 w-48" data-testid="merge-card-loading" />;
  }
  const live = detail?.live;
  if (live) {
    return (
      <MergeConditions
        values={{
          pipeline: live.pipeline,
          mergeable: live.mergeable,
          conflict: conflictTri(live.conflict),
          approvals: live.approvals,
          draft: live.draft,
          threads: live.unresolved_threads,
        }}
      />
    );
  }
  if (!canManage && status !== "error") return null;
  return (
    <div className="space-y-2" data-testid="merge-card-unavailable">
      <p className="text-muted-foreground text-sm">{t("coordinator:mergeConditionsUnavailable")}</p>
      <Button
        type="button"
        size="sm"
        variant="outline"
        className="min-h-11 cursor-pointer sm:min-h-0"
        onClick={onRetry}
      >
        {t("coordinator:retry")}
      </Button>
    </div>
  );
}

type UncertainProps = {
  detail: MergeDetail | null;
  canManage: boolean;
  busy: boolean;
  actionError: "check" | "release" | null;
  onCheck: () => void;
  onRelease: () => void;
};

function UncertainHints({ dispatch }: { dispatch: MergeDetail["dispatch"] | undefined }) {
  const { t } = useTranslation();
  if (!dispatch) return null;
  return (
    <>
      {!dispatch.can_check && (
        <p className="text-muted-foreground text-xs">{t("coordinator:mergeCheckLater")}</p>
      )}
      {!dispatch.can_release && (
        <p className="text-muted-foreground text-xs">{t("coordinator:mergeReleaseLater")}</p>
      )}
      {dispatch.can_release && (
        <p className="text-muted-foreground text-xs">{t("coordinator:mergeReleaseHelp")}</p>
      )}
    </>
  );
}

function ActionError({ kind }: { kind: "check" | "release" }) {
  const { t } = useTranslation();
  return (
    <p role="alert" className="text-destructive text-xs" data-testid="merge-action-error">
      {t(kind === "check" ? "coordinator:mergeCheckFailed" : "coordinator:mergeReleaseFailed")}
    </p>
  );
}

function Uncertain({ detail, canManage, busy, actionError, onCheck, onRelease }: UncertainProps) {
  const { t } = useTranslation();
  const dispatch = detail?.dispatch;
  return (
    <div className="space-y-2" data-testid="merge-card-uncertain">
      <p className="text-sm">{t("coordinator:mergeUncertainExplain")}</p>
      {canManage && (
        <div className="flex flex-col gap-2 sm:flex-row sm:flex-wrap sm:items-center">
          <Button
            type="button"
            size="sm"
            className="min-h-11 cursor-pointer sm:min-h-0"
            disabled={busy || !dispatch?.can_check}
            onClick={onCheck}
            data-testid="merge-check-now"
          >
            {busy && <Spinner aria-hidden className="mr-1.5" />}
            {t("coordinator:mergeCheckNow")}
          </Button>
          {dispatch?.can_release && (
            <Button
              type="button"
              size="sm"
              variant="outline"
              className="min-h-11 cursor-pointer sm:min-h-0"
              disabled={busy}
              onClick={onRelease}
              data-testid="merge-release"
            >
              {t("coordinator:mergeRelease")}
            </Button>
          )}
        </div>
      )}
      {canManage && <UncertainHints dispatch={dispatch} />}
      {actionError && <ActionError kind={actionError} />}
    </div>
  );
}

function decisionToastKey(outcome: ProposalDecisionOutcome): string | null {
  switch (outcome.kind) {
    case "decided":
      return null;
    case "conflict":
      return "coordinator:toastConflict";
    case "forbidden":
      return "coordinator:toastForbidden";
    case "not_found":
      return "coordinator:toastNotFound";
    case "policy_denied":
      return "coordinator:mergeToastPolicyDenied";
    default:
      return "coordinator:toastNetworkError";
  }
}

type Announcer = {
  t: (key: string) => string;
  toast: (input: { title: string; variant?: "success" | "error" }) => void;
};

function announceDecision(outcome: ProposalDecisionOutcome, approved: boolean, ctx: Announcer) {
  const key = decisionToastKey(outcome);
  if (key) ctx.toast({ title: ctx.t(key), variant: "error" });
  else if (approved && outcome.kind === "decided" && outcome.proposal.status === "approved") {
    ctx.toast({ title: ctx.t("coordinator:mergeToastMerged"), variant: "success" });
  }
}

/** The merge proposal card: change, conditions, owner, outcome, and only the actions its status allows. */
export function MergeProposalCard(props: MergeProposalCardProps) {
  const { proposal, variant, canManage, workspaceId, coordinatorId } = props;
  const { t } = useTranslation();
  const { toast } = useToast();
  const decision = useProposalDecision(workspaceId, coordinatorId, proposal.id);
  const merge = useMergeDetail(workspaceId, coordinatorId, proposal);
  const [rejecting, setRejecting] = useState(false);
  const lockRef = useRef(false);
  const spec = proposal.final_spec ?? proposal.spec;
  const statusLine = mergeStatusText({ ...proposal, merge: merge.detail ?? undefined }, t);
  const decidable = canManage && OPEN_FOR_DECISION.has(proposal.status);
  const fullReject = rejecting && variant === "full";

  // A second press while a decision is in flight is dropped before it reaches the network.
  async function decide(run: () => Promise<ProposalDecisionOutcome>, approved: boolean) {
    if (lockRef.current) return;
    lockRef.current = true;
    try {
      const outcome = await run();
      announceDecision(outcome, approved, { t, toast });
      if (outcome.kind === "decided") setRejecting(false);
    } finally {
      lockRef.current = false;
    }
  }

  return (
    <div className="space-y-2" data-testid={`proposal-card-${proposal.id}`}>
      <div role="status" aria-live="polite" className="sr-only">
        {decision.busy ? t("coordinator:proposalWorking") : statusLine}
      </div>
      <Title changeKey={merge.detail?.change_key ?? spec.change_key} />
      <p className="text-sm" data-testid="merge-card-status">
        {statusLine}
      </p>
      <Facts proposal={proposal} detail={merge.detail} />
      <CardDescription>
        {t("coordinator:proposedBy", { name: props.coordinatorName ?? "" })}
      </CardDescription>
      <Notes detail={merge.detail} />
      <Live
        detail={merge.detail}
        status={merge.status}
        canManage={canManage}
        onRetry={() => void merge.reload()}
      />
      {proposal.status === "uncertain" && (
        <Uncertain
          detail={merge.detail}
          canManage={canManage}
          busy={merge.busy}
          actionError={merge.actionError}
          onCheck={() => void merge.checkNow()}
          onRelease={() => void merge.release()}
        />
      )}
      <p className="text-muted-foreground text-xs" data-testid="merge-cannot-undo">
        {t("coordinator:mergeCannotUndo")}
      </p>
      {decidable && fullReject && (
        <RejectForm
          busy={decision.busy}
          serverError={null}
          onConfirm={(reason) => void decide(() => decision.reject(reason), false)}
          onCancel={() => setRejecting(false)}
        />
      )}
      {decidable && !fullReject && (
        <div className="flex flex-col gap-2 sm:flex-row sm:flex-wrap sm:items-center">
          <Button
            size="sm"
            className="min-h-11 cursor-pointer sm:min-h-0"
            disabled={decision.busy}
            onClick={() => void decide(() => decision.approve(), true)}
            data-testid="merge-approve"
          >
            {decision.busy && <Spinner aria-hidden className="mr-1.5" />}
            {t("coordinator:mergeApprove")}
          </Button>
          <Button
            size="sm"
            variant="outline"
            className="min-h-11 cursor-pointer sm:min-h-0"
            disabled={decision.busy}
            onClick={() => {
              if (variant === "compact") props.onNavigateToForm?.("reject");
              else setRejecting(true);
            }}
            data-testid="merge-reject"
          >
            {t("coordinator:reject")}
          </Button>
        </div>
      )}
    </div>
  );
}
