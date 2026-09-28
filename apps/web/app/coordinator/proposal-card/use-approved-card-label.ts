"use client";

import { useEffect, useRef, useState } from "react";
import { fetchTask } from "@/lib/api/domains/kanban-api";
import type { Proposal } from "@/lib/api/domains/coordinator-api";
import { approvedCardFallbackTitle, effectiveProposalSpec } from "@/lib/coordinator/proposal-text";

/**
 * `<card>`: the approved proposal's task identifier, read once with
 * `fetchTask(task_id)` the first time this proposal instance is seen
 * `approved`, falling back to the current spec's title when the identifier
 * is absent, the read fails, or the task was deleted
 * (docs/specs/coordinator/system-design/proposal-cards.md#cards "`<card>`
 * and `<step>`").
 */
export function useApprovedCardLabel(proposal: Proposal): string {
  const fallback = approvedCardFallbackTitle(effectiveProposalSpec(proposal));
  const [identifier, setIdentifier] = useState<string | undefined>(undefined);
  const fetchedForIdRef = useRef<string | null>(null);

  useEffect(() => {
    if (proposal.status !== "approved" || !proposal.task_id) return;
    if (fetchedForIdRef.current === proposal.id) return;
    fetchedForIdRef.current = proposal.id;
    const taskId = proposal.task_id;
    fetchTask(taskId)
      .then((task) => setIdentifier(task.identifier || undefined))
      .catch(() => {
        // Read failure or a deleted task: keep the spec-title fallback.
      });
  }, [proposal]);

  return identifier ?? fallback;
}
