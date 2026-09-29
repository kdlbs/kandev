"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { fetchWorkflowSnapshot, listWorkflows } from "@/lib/api/domains/kanban-api";
import { listRepositories } from "@/lib/api/domains/workspace-api";
import { eligibleStep, type EligibleStepNode } from "@/lib/coordinator/eligible-step";
import type { Repository, Workflow, WorkflowStepDTO } from "@/lib/types/http";
import { stepHasOnEnterAction } from "@/lib/types/http";

export type OptionsField<T> = {
  status: "loading" | "loaded" | "error";
  value: T[];
};

function initialField<T>(): OptionsField<T> {
  return { status: "loading", value: [] };
}

export type UseProposalEditOptionsResult = {
  /** The workspace's workflows. */
  workflows: OptionsField<Workflow>;
  /** The eligible steps of `workflowId`'s snapshot (`lib/coordinator/eligible-step.ts`). */
  steps: OptionsField<WorkflowStepDTO>;
  /** "No repository" plus the workspace's repositories. */
  repositories: OptionsField<Repository>;
  retryWorkflows: () => void;
  retryRepositories: () => void;
  retrySteps: () => void;
  /** The selected workflow's name from its snapshot; null until that snapshot loads. */
  snapshotWorkflowName: string | null;
};

function toEligibleNode(step: WorkflowStepDTO): EligibleStepNode {
  return {
    id: step.id,
    isStart: step.is_start_step ?? false,
    allowManualMove: step.allow_manual_move,
    autoStartOnEnter: stepHasOnEnterAction(step, "auto_start_agent"),
    pullFromStepId: step.pull_from_step_id ?? null,
  };
}

function eligibleSteps(steps: WorkflowStepDTO[]): WorkflowStepDTO[] {
  const nodes = steps.map(toEligibleNode);
  return steps.filter((step) => eligibleStep(nodes, step.id));
}

/**
 * Loads the Edit form's workflow/step/repository options
 * (docs/specs/coordinator/system-design/proposal-cards.md#cards "Edit form
 * options"): workflows and repositories load once per opening; the step
 * snapshot reloads whenever `workflowId` changes, discarding a response for a
 * workflow that is no longer selected.
 */
// eslint-disable-next-line max-lines-per-function -- one hook owns three independent option reads plus their retries
export function useProposalEditOptions(
  workspaceId: string,
  workflowId: string,
): UseProposalEditOptionsResult {
  const [workflows, setWorkflows] = useState<OptionsField<Workflow>>(initialField);
  const [steps, setSteps] = useState<OptionsField<WorkflowStepDTO>>(initialField);
  const [repositories, setRepositories] = useState<OptionsField<Repository>>(initialField);
  const [snapshotWorkflowName, setSnapshotWorkflowName] = useState<string | null>(null);
  const stepsSeqRef = useRef(0);

  const readWorkflows = useCallback(() => {
    setWorkflows(initialField());
    listWorkflows(workspaceId)
      .then((res) => setWorkflows({ status: "loaded", value: res.workflows }))
      .catch(() => setWorkflows({ status: "error", value: [] }));
  }, [workspaceId]);

  const readRepositories = useCallback(() => {
    setRepositories(initialField());
    listRepositories(workspaceId)
      .then((res) => setRepositories({ status: "loaded", value: res.repositories }))
      .catch(() => setRepositories({ status: "error", value: [] }));
  }, [workspaceId]);

  const readSteps = useCallback(() => {
    const seq = ++stepsSeqRef.current;
    setSteps(initialField());
    setSnapshotWorkflowName(null);
    fetchWorkflowSnapshot(workflowId)
      .then((snapshot) => {
        if (stepsSeqRef.current !== seq) return;
        setSnapshotWorkflowName(snapshot.workflow?.name ?? null);
        setSteps({ status: "loaded", value: eligibleSteps(snapshot.steps) });
      })
      .catch(() => {
        if (stepsSeqRef.current !== seq) return;
        setSteps({ status: "error", value: [] });
      });
  }, [workflowId]);

  // Issued once per opening (mount): listWorkflows/listRepositories are not
  // re-read on a workflowId change, only the step snapshot below is.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  useEffect(() => {
    readWorkflows();
    readRepositories();
  }, []);

  useEffect(() => {
    readSteps();
  }, [readSteps]);

  return {
    workflows,
    steps,
    repositories,
    retryWorkflows: readWorkflows,
    retryRepositories: readRepositories,
    retrySteps: readSteps,
    snapshotWorkflowName,
  };
}
