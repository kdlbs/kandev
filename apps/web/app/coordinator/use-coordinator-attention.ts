"use client";

import { useCallback, useMemo } from "react";
import type { TaskPR } from "@/lib/types/github";
import {
  classify,
  type AttentionAutonomyInput,
  type AttentionTask,
  type ClassifyResult,
} from "@/lib/coordinator/attention";
import { useAutonomy, type AutonomyInput } from "@/hooks/domains/coordinator/use-autonomy";
import { useCoordinatorPhase3Effective } from "@/hooks/domains/settings/use-coordinator-phase3-effective";
import {
  isOpenProposal,
  isVisibleProposal,
  useProposalsStore,
} from "@/hooks/domains/coordinator/use-proposals";
import { useCoordinatorInputs } from "./use-coordinator-inputs";
import { useCoordinatorPRs } from "./use-coordinator-prs";
import { useCoordinatorTasks } from "./use-coordinator-tasks";
import { useNowTick } from "./use-now-tick";
import { useCoordinatorWatchSet } from "./use-coordinator-watch-set";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { filterWatched, type WatchSet } from "@/lib/coordinator/watch-filter";

export type CoordinatorInputKind = "tasks" | "stalls" | "proposals" | "watches";

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
  /** True while the phase-2 watch set has not loaded: task and stall items and the counts derived from them are withheld. */
  watchSetUnavailable: boolean;
  /** The coordinator's loaded watch set (phase 2), undefined until it has loaded. */
  watchSet: WatchSet | undefined;
  /** Per-input status, in the banner order tasks, stall records, proposals. */
  inputs: CoordinatorInputStatus[];
  /** The phase-3 autonomy read. It is not one of `inputs`: it has its own error line and retry. */
  autonomy: AutonomyInput;
  /** Whether the autonomy surface exists (coordinator, phase 2 and phase 3 all on). */
  phase3Effective: boolean;
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

/** The held-autonomy classify() input, absent while the read is missing or failed (fail closed). */
function autonomyClassifyInput(
  coordinatorId: string | null,
  autonomy: AutonomyInput,
): AttentionAutonomyInput | null {
  const { value, loadedAt, error } = autonomy;
  if (!coordinatorId || !value || loadedAt === null || error) return null;
  return {
    coordinatorId,
    enabled: value.autonomy_enabled,
    reason: value.admission?.reason,
    detail: value.admission?.detail ?? "",
    pendingWakes: value.pending_wakes,
    oldestPendingAt: value.oldest_pending_at,
    loadedAt,
    conditions: value.containment.conditions,
  };
}

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
  const phase2 = useFeature("coordinatorPhase2");
  const {
    stalls,
    proposals,
    retryFailed: retryStallsAndProposals,
  } = useCoordinatorInputs(workspaceId, coordinatorId, phase2);
  const now = useNowTick();
  const phase3Effective = useCoordinatorPhase3Effective();
  const autonomy = useAutonomy(
    workspaceId ?? "",
    coordinatorId ?? "",
    phase3Effective && !!workspaceId && !!coordinatorId,
  );
  const autonomyInput = useMemo(
    () => autonomyClassifyInput(coordinatorId, autonomy),
    // eslint-disable-next-line react-hooks/exhaustive-deps -- `autonomy` is a fresh object each render; its value, loadedAt and error are the inputs
    [coordinatorId, autonomy.value, autonomy.loadedAt, autonomy.error],
  );
  const watchSet = useCoordinatorWatchSet(workspaceId, coordinatorId, phase2);
  const watchValue = watchSet.input.value;
  const watchSetUnavailable = phase2 && watchValue === undefined;

  const { tasks: watchedTasks, stalls: watchedStalls } = useMemo(() => {
    const input = { tasks: tasksInput.tasks, stalls: stalls.value ?? [] };
    if (!phase2) return input;
    return watchValue ? filterWatched(input, watchValue) : { tasks: [], stalls: [] };
  }, [phase2, watchValue, tasksInput.tasks, stalls.value]);

  const classification = useMemo(
    () => classify(watchedTasks, watchedStalls, proposals.value ?? [], now, autonomyInput),
    [watchedTasks, watchedStalls, proposals.value, now, autonomyInput],
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
    ...(phase2
      ? [
          {
            kind: "watches" as const,
            error: watchSet.input.error,
            loadedAt: watchSet.input.loadedAt,
          },
        ]
      : []),
  ];

  const retryFailed = () => {
    if (tasksInput.error) tasksInput.retry();
    if (phase2 && watchSet.input.error) watchSet.retry();
    retryStallsAndProposals();
  };

  const tasks = watchedTasks;
  const stallValues = watchedStalls;
  const computeNeedsYouCount = useCallback((): number => {
    if (!coordinatorId) return classification.needsYou.length;
    const coordinatorProposals = useProposalsStore.getState().byCoordinator[coordinatorId];
    const freshProposals = coordinatorProposals
      ? Object.values(coordinatorProposals.byId).filter(
          (proposal) => isOpenProposal(proposal) && isVisibleProposal(proposal, phase2),
        )
      : [];
    return classify(tasks, stallValues ?? [], freshProposals, Date.now(), autonomyInput).needsYou
      .length;
    // eslint-disable-next-line react-hooks/exhaustive-deps -- classification.needsYou.length is only the no-coordinator fallback, not a dependency of the fresh read
  }, [coordinatorId, tasks, stallValues, phase2, autonomyInput]);

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
    watchSetUnavailable,
    watchSet: phase2 ? watchValue : undefined,
    inputs,
    autonomy,
    phase3Effective,
    retryFailed,
    computeNeedsYouCount,
  };
}
