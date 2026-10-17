"use client";

import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Spinner } from "@kandev/ui/spinner";
import type { StoredProposal } from "@/lib/api/domains/coordinator-api";
import {
  useProposalReply,
  type UseProposalReplyResult,
} from "@/hooks/domains/coordinator/use-proposal-reply";
import type { ProposalDecisionOutcome } from "@/hooks/domains/coordinator/use-proposal-decision";
import { useCoordinatorPhase3Effective } from "@/hooks/domains/settings/use-coordinator-phase3-effective";
import { useToast } from "@/components/toast-provider";
import { ReplyForm, type ReplyFormServerError } from "./reply-form";

export type ReplySectionProps = {
  proposal: StoredProposal;
  canManage: boolean;
  workspaceId: string;
  coordinatorId: string;
  /** Called when a reply request starts, so the Needs you list keeps rendering the item. */
  onReplyStarted?: () => void;
};

type TFn = ReturnType<typeof useTranslation>["t"];
type ToastFn = ReturnType<typeof useToast>["toast"];

function toastReplyOutcome(
  outcome: ProposalDecisionOutcome,
  route: "reply" | "deliver",
  t: TFn,
  toast: ToastFn,
) {
  switch (outcome.kind) {
    case "decided":
      if (outcome.proposal.reply_delivered_at) {
        toast({ title: t("coordinator:toastReplySent"), variant: "success" });
      } else {
        toast({ title: t("coordinator:toastReplyNotDelivered"), variant: "error" });
      }
      return;
    case "conflict":
      toast({ title: t("coordinator:toastConflict"), variant: "error" });
      return;
    case "forbidden":
      toast({ title: t("coordinator:toastForbidden"), variant: "error" });
      return;
    case "validation":
      toast({ title: t("coordinator:toastReplyKindRefused"), variant: "error" });
      return;
    case "not_found":
      toast({ title: t("coordinator:toastNotFound"), variant: "error" });
      return;
    case "network":
      toast({
        title: t(
          route === "reply"
            ? "coordinator:toastReplyNetworkError"
            : "coordinator:toastDeliverNetworkError",
        ),
        variant: "error",
      });
  }
}

function ReturnedActions({
  proposal,
  reply,
  onRedeliver,
}: {
  proposal: StoredProposal;
  reply: UseProposalReplyResult;
  onRedeliver: () => void;
}) {
  const { t } = useTranslation();
  if (proposal.reply_delivered_at) return null;
  return (
    <div className="space-y-2">
      <div role="status" aria-live="polite">
        <p className="text-sm">
          {reply.busy ? t("coordinator:replySending") : t("coordinator:replyNotDelivered")}
        </p>
      </div>
      <Button
        size="sm"
        variant="outline"
        disabled={reply.busy}
        onClick={onRedeliver}
        className="min-h-11 sm:min-h-0"
      >
        {reply.busy && <Spinner aria-hidden className="mr-1.5" />}
        {t("coordinator:replySendAgain")}
      </Button>
    </div>
  );
}

/**
 * The reply affordance of a card: a "Reply" button and form while the proposal
 * is pending, and the not-delivered state with "Send again" once it is
 * returned but the message is not yet delivered.
 */
export function ReplySection({
  proposal,
  canManage,
  workspaceId,
  coordinatorId,
  onReplyStarted,
}: ReplySectionProps) {
  const { t } = useTranslation();
  const { toast } = useToast();
  const reply = useProposalReply(workspaceId, coordinatorId, proposal.id);
  const [open, setOpen] = useState(false);
  const [serverError, setServerError] = useState<ReplyFormServerError | null>(null);
  const openButtonRef = useRef<HTMLButtonElement>(null);

  const phase3 = useCoordinatorPhase3Effective();
  if (!canManage) return null;

  function close() {
    setOpen(false);
    setServerError(null);
    requestAnimationFrame(() => openButtonRef.current?.focus());
  }

  async function send(text: string) {
    onReplyStarted?.();
    const outcome = await reply.reply(text);
    if (outcome.kind === "network") {
      toastReplyOutcome(outcome, "reply", t, toast);
      return;
    }
    if (outcome.kind === "validation" && outcome.field !== "kind") {
      setServerError({ message: outcome.message, field: outcome.field });
      return;
    }
    setServerError(null);
    setOpen(false);
    toastReplyOutcome(outcome, "reply", t, toast);
  }

  async function redeliver() {
    toastReplyOutcome(await reply.redeliver(), "deliver", t, toast);
  }

  if (proposal.status === "returned") {
    if (!phase3) return null;
    return (
      <ReturnedActions proposal={proposal} reply={reply} onRedeliver={() => void redeliver()} />
    );
  }
  if (proposal.status !== "pending" || !phase3) return null;
  if (open) {
    return (
      <ReplyForm
        busy={reply.busy}
        serverError={serverError}
        onSend={(text) => void send(text)}
        onCancel={close}
      />
    );
  }
  return (
    <Button
      ref={openButtonRef}
      size="sm"
      variant="outline"
      onClick={() => setOpen(true)}
      className="min-h-11 sm:min-h-0"
    >
      {t("coordinator:replyAction")}
    </Button>
  );
}
