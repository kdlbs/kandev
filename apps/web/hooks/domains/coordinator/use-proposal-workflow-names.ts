"use client";

import { useEffect, useRef, useState } from "react";
import { fetchWorkflowSnapshot } from "@/lib/api/domains/kanban-api";

export type ProposalWorkflowNames = {
  workflowNameById: Map<string, string>;
  stepNameByWorkflowStep: Map<string, string>;
};

const EMPTY_NAMES: ProposalWorkflowNames = {
  workflowNameById: new Map(),
  stepNameByWorkflowStep: new Map(),
};

function workflowStepKey(workflowId: string, stepId: string): string {
  return `${workflowId}:${stepId}`;
}

/**
 * Resolves one workflow's name and step names for the chat card's
 * "`<workflow>` · `<step>`" label (docs/specs/coordinator/system-design/
 * proposal-cards.md#cards "Content by state"). Unlike the Needs-you
 * attention input, the chat card is attached to a single proposal, so this
 * fetches only that proposal's current workflow snapshot instead of every
 * workspace workflow. A failed or missing read leaves the maps empty, which
 * falls back to the raw id per `workflowStepLabel`.
 */
export function useProposalWorkflowNames(workflowId: string | null): ProposalWorkflowNames {
  const [names, setNames] = useState<ProposalWorkflowNames>(EMPTY_NAMES);
  const seqRef = useRef(0);

  useEffect(() => {
    if (!workflowId) {
      setNames(EMPTY_NAMES);
      return;
    }
    const seq = ++seqRef.current;
    fetchWorkflowSnapshot(workflowId, { cache: "no-store" })
      .then((snapshot) => {
        if (seqRef.current !== seq) return;
        setNames({
          workflowNameById: new Map([[workflowId, snapshot.workflow?.name ?? workflowId]]),
          stepNameByWorkflowStep: new Map(
            snapshot.steps.map((step) => [workflowStepKey(workflowId, step.id), step.name]),
          ),
        });
      })
      .catch(() => {
        if (seqRef.current !== seq) return;
        setNames(EMPTY_NAMES);
      });
  }, [workflowId]);

  return names;
}
