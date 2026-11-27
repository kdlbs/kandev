"use client";

import { useCallback, useState } from "react";
import {
  deliverProposalReply,
  isStoredProposal,
  replyToProposal,
  type WireProposal,
} from "@/lib/api/domains/coordinator-api";
import { outcomeFromError, type ProposalDecisionOutcome } from "./use-proposal-decision";
import { useProposalsStore, type ProposalApplyResult } from "./use-proposals";

export const REPLY_TEXT_MAX = 2000;

/** Length in Unicode code points, the unit the server validates. */
export function replyTextLength(text: string): number {
  return Array.from(text).length;
}

export type UseProposalReplyResult = {
  busy: boolean;
  reply: (text: string) => Promise<ProposalDecisionOutcome>;
  redeliver: () => Promise<ProposalDecisionOutcome>;
};

/**
 * Wraps `replyToProposal`/`deliverProposalReply` with an in-flight lock and the
 * same outcome kinds and store-apply rule as `useProposalDecision`. A 2xx reply
 * whose row is returned but not delivered is still a `decided` outcome; the
 * caller reads `reply_delivered_at` to tell them apart.
 */
export function useProposalReply(
  workspaceId: string,
  coordinatorId: string,
  proposalId: string,
): UseProposalReplyResult {
  const [busy, setBusy] = useState(false);

  const apply = useCallback(
    (result: ProposalApplyResult) => {
      const seq = useProposalsStore.getState().takeProposalTicket(coordinatorId);
      useProposalsStore.getState().applyProposalResult(coordinatorId, proposalId, seq, result);
    },
    [coordinatorId, proposalId],
  );

  const run = useCallback(
    async (action: () => Promise<WireProposal>): Promise<ProposalDecisionOutcome> => {
      setBusy(true);
      try {
        const proposal = await action();
        if (!isStoredProposal(proposal)) {
          apply({ kind: "not_found" });
          return { kind: "not_found" };
        }
        apply({ kind: "success", proposal });
        return { kind: "decided", proposal };
      } catch (error) {
        return outcomeFromError(error, apply);
      } finally {
        setBusy(false);
      }
    },
    [apply],
  );

  const reply = useCallback(
    (text: string) => run(() => replyToProposal(workspaceId, coordinatorId, proposalId, text)),
    [run, workspaceId, coordinatorId, proposalId],
  );
  const redeliver = useCallback(
    () => run(() => deliverProposalReply(workspaceId, coordinatorId, proposalId)),
    [run, workspaceId, coordinatorId, proposalId],
  );

  return { busy, reply, redeliver };
}
