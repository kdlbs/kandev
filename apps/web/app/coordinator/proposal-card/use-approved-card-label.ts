"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { fetchTask } from "@/lib/api/domains/kanban-api";
import type { Proposal } from "@/lib/api/domains/coordinator-api";
import { approvedCardFallbackTitle, effectiveProposalSpec } from "@/lib/coordinator/proposal-text";

export type ApprovedCardLabel = {
  /** The label for the card's own "Approved: <card>" status line. */
  label: string;
  /** Resolves `<card>` for an approved proposal, sharing this card's single task read. */
  resolveLabel: (proposal: Proposal) => Promise<string>;
};

/**
 * `<card>`: the approved proposal's task identifier, read once with
 * `fetchTask(task_id)` the first time this proposal instance is seen
 * `approved`, falling back to the current spec's title when the identifier
 * is absent, the read fails, or the task was deleted
 * (docs/specs/coordinator/system-design/proposal-cards.md#cards "`<card>`
 * and `<step>`"). The status line and the approve toast share that one read.
 */
export function useApprovedCardLabel(proposal: Proposal): ApprovedCardLabel {
  const fallback = approvedCardFallbackTitle(effectiveProposalSpec(proposal));
  const [identifier, setIdentifier] = useState<string | undefined>(undefined);
  const readRef = useRef<{ proposalId: string; read: Promise<string | undefined> } | null>(null);

  const readIdentifier = useCallback((p: Proposal): Promise<string | undefined> => {
    if (readRef.current?.proposalId === p.id) return readRef.current.read;
    const read = p.task_id
      ? fetchTask(p.task_id)
          .then((task) => task.identifier || undefined)
          // Read failure or a deleted task: keep the spec-title fallback.
          .catch(() => undefined)
      : Promise.resolve(undefined);
    readRef.current = { proposalId: p.id, read };
    return read;
  }, []);

  useEffect(() => {
    if (proposal.status !== "approved" || !proposal.task_id) return;
    void readIdentifier(proposal).then(setIdentifier);
  }, [proposal, readIdentifier]);

  const resolveLabel = useCallback(
    async (p: Proposal) =>
      (await readIdentifier(p)) ?? approvedCardFallbackTitle(effectiveProposalSpec(p)),
    [readIdentifier],
  );

  return { label: identifier ?? fallback, resolveLabel };
}
