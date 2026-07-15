"use client";

import { useCallback, useMemo, useRef, useState } from "react";
import type { WorkflowStep } from "@/lib/types/http";
import {
  analyzeWorkflowReplayCycles,
  type WorkflowReplayCycleDiagnostic,
  type WorkflowReplayCycleSeverity,
} from "@/lib/workflows/replay-cycle-analysis";

export type WorkflowMutationIntent = "apply" | "create";

export type WorkflowMutationProposal = {
  diagnostics: WorkflowReplayCycleDiagnostic[];
  intent: WorkflowMutationIntent;
  severity: WorkflowReplayCycleSeverity;
};

type GuardMutationParams = {
  proposedSteps: WorkflowStep[];
  operation: () => Promise<void>;
  intent?: WorkflowMutationIntent;
  baselineSteps?: WorkflowStep[];
};

export type WorkflowMutationGuardController = {
  diagnostics: WorkflowReplayCycleDiagnostic[];
  proposal: WorkflowMutationProposal | null;
  isMutationPending: boolean;
  guardMutation: (params: GuardMutationParams) => Promise<void>;
  confirmProposal: () => Promise<void>;
  cancelProposal: () => void;
};

type MutationPhase = "idle" | "proposal" | "running";

function introducedDiagnostics(
  baselineSteps: WorkflowStep[],
  proposedSteps: WorkflowStep[],
): WorkflowReplayCycleDiagnostic[] {
  const existingIdentities = new Set(
    analyzeWorkflowReplayCycles(baselineSteps).map((diagnostic) => diagnostic.identity),
  );
  return analyzeWorkflowReplayCycles(proposedSteps).filter(
    (diagnostic) => !existingIdentities.has(diagnostic.identity),
  );
}

export function useWorkflowMutationGuard(
  displayedSteps: WorkflowStep[],
): WorkflowMutationGuardController {
  const diagnostics = useMemo(() => analyzeWorkflowReplayCycles(displayedSteps), [displayedSteps]);
  const [proposal, setProposal] = useState<WorkflowMutationProposal | null>(null);
  const [isMutationPending, setIsMutationPending] = useState(false);
  const mutationPhase = useRef<MutationPhase>("idle");
  const pendingOperation = useRef<(() => Promise<void>) | null>(null);

  const releaseMutation = useCallback(() => {
    mutationPhase.current = "idle";
    setIsMutationPending(false);
  }, []);

  const cancelProposal = useCallback(() => {
    if (mutationPhase.current !== "proposal") return;
    pendingOperation.current = null;
    setProposal(null);
    releaseMutation();
  }, [releaseMutation]);

  const confirmProposal = useCallback(async () => {
    if (mutationPhase.current !== "proposal") return;
    const operation = pendingOperation.current;
    if (!operation) {
      setProposal(null);
      releaseMutation();
      return;
    }
    pendingOperation.current = null;
    mutationPhase.current = "running";
    setProposal(null);
    try {
      await operation();
    } finally {
      releaseMutation();
    }
  }, [releaseMutation]);

  const guardMutation = useCallback(
    async ({
      proposedSteps,
      operation,
      intent = "apply",
      baselineSteps = displayedSteps,
    }: GuardMutationParams) => {
      if (mutationPhase.current !== "idle") return;
      mutationPhase.current = "running";
      setIsMutationPending(true);

      try {
        const introduced = introducedDiagnostics(baselineSteps, proposedSteps);
        if (introduced.length === 0) {
          await operation();
          releaseMutation();
          return;
        }

        const severity = introduced.some((diagnostic) => diagnostic.severity === "blocking")
          ? "blocking"
          : "warning";
        pendingOperation.current = severity === "warning" ? operation : null;
        mutationPhase.current = "proposal";
        setProposal({ diagnostics: introduced, intent, severity });
      } catch (error) {
        releaseMutation();
        throw error;
      }
    },
    [displayedSteps, releaseMutation],
  );

  return {
    diagnostics,
    proposal,
    isMutationPending,
    guardMutation,
    confirmProposal,
    cancelProposal,
  };
}
