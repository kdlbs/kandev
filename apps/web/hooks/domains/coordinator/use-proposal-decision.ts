"use client";

import { useCallback, useState } from "react";
import { ApiError } from "@/lib/api/client";
import {
  approveProposal,
  getProposalConflict,
  rejectProposal,
  type ApproveProposalEdits,
  type Proposal,
} from "@/lib/api/domains/coordinator-api";
import { useProposalsStore } from "./use-proposals";

export type ProposalDecisionOutcome =
  | { kind: "decided"; proposal: Proposal }
  | { kind: "validation"; message: string; field: string | null }
  | { kind: "conflict"; proposal: Proposal }
  | { kind: "forbidden" }
  | { kind: "not_found" }
  | { kind: "network" };

export type UseProposalDecisionResult = {
  /** True from the click of Approve/Approve with edits/Confirm reject until the request settles (the card's in-flight lock). */
  busy: boolean;
  approve: (edits?: ApproveProposalEdits) => Promise<ProposalDecisionOutcome>;
  reject: (reason?: string) => Promise<ProposalDecisionOutcome>;
};

function fieldErrorBody(body: unknown): { message: string; field: string | null } {
  if (!body || typeof body !== "object") return { message: "", field: null };
  const record = body as { error?: unknown; field?: unknown };
  return {
    message: typeof record.error === "string" ? record.error : "",
    field: typeof record.field === "string" ? record.field : null,
  };
}

/**
 * Wraps `approveProposal`/`rejectProposal` with the card's in-flight lock and
 * the decision-outcomes table (docs/specs/coordinator/system-design/
 * proposal-cards.md#cards "In-flight lock", "Decision outcomes"): every
 * non-2xx status is translated to one outcome kind instead of a thrown
 * error, and every outcome that has a settled row merges it into the shared
 * store before resolving. Callers (ProposalCard) own the resulting UI:
 * closing forms, toasts, and focus.
 */
export function useProposalDecision(
  workspaceId: string,
  coordinatorId: string,
  proposalId: string,
): UseProposalDecisionResult {
  const [busy, setBusy] = useState(false);

  const run = useCallback(
    async (action: () => Promise<Proposal>): Promise<ProposalDecisionOutcome> => {
      setBusy(true);
      try {
        const proposal = await action();
        useProposalsStore.getState().mergeOne(coordinatorId, proposal);
        return { kind: "decided", proposal };
      } catch (error) {
        if (error instanceof ApiError) {
          if (error.status === 409) {
            const conflict = getProposalConflict(error);
            if (conflict) {
              useProposalsStore.getState().mergeOne(coordinatorId, conflict);
              return { kind: "conflict", proposal: conflict };
            }
          }
          if (error.status === 400) {
            const { message, field } = fieldErrorBody(error.body);
            return { kind: "validation", message, field };
          }
          if (error.status === 403) return { kind: "forbidden" };
          if (error.status === 404) {
            useProposalsStore.getState().evict(coordinatorId, proposalId);
            return { kind: "not_found" };
          }
        }
        return { kind: "network" };
      } finally {
        setBusy(false);
      }
    },
    [coordinatorId, proposalId],
  );

  const approve = useCallback(
    (edits?: ApproveProposalEdits) =>
      run(() => approveProposal(workspaceId, coordinatorId, proposalId, edits)),
    [run, workspaceId, coordinatorId, proposalId],
  );

  const reject = useCallback(
    (reason?: string) => run(() => rejectProposal(workspaceId, coordinatorId, proposalId, reason)),
    [run, workspaceId, coordinatorId, proposalId],
  );

  return { busy, approve, reject };
}
