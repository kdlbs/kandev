"use client";

import { useCallback, useMemo } from "react";
import type { TaskPR } from "@/lib/types/github";
import { classify, type AttentionTask, type ClassifyResult } from "@/lib/coordinator/attention";
import { isOpenProposal, useProposalsStore } from "@/hooks/domains/coordinator/use-proposals";
import { useCoordinatorInputs } from "./use-coordinator-inputs";
import { useCoordinatorPRs } from "./use-coordinator-prs";
import { useCoordinatorTasks } from "./use-coordinator-tasks";
import { useNowTick } from "./use-now-tick";

export type CoordinatorInputKind = "tasks" | "stalls" | "proposals";

export type CoordinatorInputStatus = {
  kind: CoordinatorInputKind;
  error: boolean;
  loadedAt: number | undefined;
};

export type UseCoordinatorAttentionResult = {
  classification: ClassifyResult;
  /** The name of each task's current step, keyed by task id (Adoption decision 3). */
  stepNameByTaskId: Map<string, string>;
  /** The name of each loaded workflow of the workspace, keyed by workflow id (for a proposal's target). */
  workflowNameById: Map<string, string>;
  /** The name of a workflow's step, keyed by `${workflowId}:${stepId}` (for a proposal's target). */
  stepNameByWorkflowStep: Map<string, string>;
  /** Open (non-archived) tasks of the workspace, keyed by id, for a proposal's source-task lookup. */
  openTasksById: Map<string, AttentionTask>;
  /** Every loaded PR association for the workspace's tasks, keyed by task id (Queue's PR detail column). */
  prsByTaskId: ReadonlyMap<string, TaskPR[]>;
  /** Every task of the workspace's loaded snapshots, for availability lookups (What it did). */
  tasks: AttentionTask[];
  /** When the tasks input first completed for the workspace; set even when `error` is true after a partial failure. */
  loadedAt: number | undefined;
  /** True while the latest tasks read has failed. */
  error: boolean;
  /** True once the tasks input has never had a successful read. */
  tasksNeverLoaded: boolean;
  /** Per-input status, in the banner order tasks, stall records, proposals. */
  inputs: CoordinatorInputStatus[];
  /** Re-issues only the reads currently in an error state, in parallel. */
  retryFailed: () => void;
  /**
   * The Needs-you item count a decision toast's "Next" line reads
   * (proposal-cards.md#cards "Toast counts"), computed from a fresh
   * `useProposalsStore` read rather than this render's `classification` so
   * it reflects the store merge a decision just made, not last render's
   * snapshot.
   */
  computeNeedsYouCount: () => number;
};

/**
 * Combines the coordinator's three screen inputs (tasks, stall records,
 * pending proposals) with the 30-second age timer into one classification
 * result for the Needs you and Queue screens (docs/specs/coordinator/
 * system-design/needs-you.md#classification, #failure-and-recovery).
 */
export function useCoordinatorAttention(
  workspaceId: string | null,
  coordinatorId: string | null,
): UseCoordinatorAttentionResult {
  const tasksInput = useCoordinatorTasks(workspaceId);
  const prsByTaskId = useCoordinatorPRs(workspaceId);
  const {
    stalls,
    proposals,
    retryFailed: retryStallsAndProposals,
  } = useCoordinatorInputs(workspaceId, coordinatorId);
  const now = useNowTick();

  const classification = useMemo(
    () => classify(tasksInput.tasks, stalls.value ?? [], proposals.value ?? [], now),
    [tasksInput.tasks, stalls.value, proposals.value, now],
  );

  const openTasksById = useMemo(
    () =>
      new Map(tasksInput.tasks.filter((task) => !task.isArchived).map((task) => [task.id, task])),
    [tasksInput.tasks],
  );

  const inputs: CoordinatorInputStatus[] = [
    { kind: "tasks", error: tasksInput.error, loadedAt: tasksInput.loadedAt },
    { kind: "stalls", error: stalls.error, loadedAt: stalls.loadedAt },
    { kind: "proposals", error: proposals.error, loadedAt: proposals.loadedAt },
  ];

  const retryFailed = () => {
    if (tasksInput.error) tasksInput.retry();
    retryStallsAndProposals();
  };

  const tasks = tasksInput.tasks;
  const stallValues = stalls.value;
  const computeNeedsYouCount = useCallback((): number => {
    if (!coordinatorId) return classification.needsYou.length;
    const coordinatorProposals = useProposalsStore.getState().byCoordinator[coordinatorId];
    const freshProposals = coordinatorProposals
      ? Object.values(coordinatorProposals.byId).filter(isOpenProposal)
      : [];
    return classify(tasks, stallValues ?? [], freshProposals, Date.now()).needsYou.length;
    // eslint-disable-next-line react-hooks/exhaustive-deps -- classification.needsYou.length is only the no-coordinator fallback, not a dependency of the fresh read
  }, [coordinatorId, tasks, stallValues]);

  return {
    classification,
    stepNameByTaskId: tasksInput.stepNameByTaskId,
    workflowNameById: tasksInput.workflowNameById,
    stepNameByWorkflowStep: tasksInput.stepNameByWorkflowStep,
    openTasksById,
    prsByTaskId,
    tasks: tasksInput.tasks,
    loadedAt: tasksInput.loadedAt,
    error: tasksInput.error,
    tasksNeverLoaded: tasksInput.loadedAt === undefined,
    inputs,
    retryFailed,
    computeNeedsYouCount,
  };
}
