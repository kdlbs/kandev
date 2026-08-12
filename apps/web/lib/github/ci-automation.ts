import type { TaskCIPRAutomationState, TaskPR, TaskPRAutomationOptions } from "@/lib/types/github";

const DEFAULT_AUTO_FIX_MAX_ROUNDS = 10;

export const DISABLED_PR_AUTOMATION_OPTIONS: Omit<
  TaskPRAutomationOptions,
  "task_id" | "repository_id" | "pr_number" | "created_at" | "updated_at"
> = {
  auto_fix_enabled: false,
  auto_merge_enabled: false,
  prompt_on_review_requested: false,
  prompt_on_merged: false,
  prompt_on_closed: false,
};

/**
 * Selects the given PR's own automation switches out of the task-scoped
 * pr_options array. Falls back to all-off defaults when the PR has no
 * stored row yet (never enabled), mirroring findCIAutomationStateForPR.
 */
export function findPRAutomationOptionsForPR(
  options: TaskPRAutomationOptions[] | undefined,
  pr: TaskPR,
): TaskPRAutomationOptions {
  const repositoryID = pr.repository_id ?? "";
  const found = options?.find(
    (option) => option.pr_number === pr.pr_number && option.repository_id === repositoryID,
  );
  if (found) return found;
  return {
    task_id: pr.task_id,
    repository_id: repositoryID,
    pr_number: pr.pr_number,
    created_at: "",
    updated_at: "",
    ...DISABLED_PR_AUTOMATION_OPTIONS,
  };
}

export type AutoFixRoundInfo = {
  current: number;
  max: number;
  exhausted: boolean;
};

export function findCIAutomationStateForPR(
  states: TaskCIPRAutomationState[] | undefined,
  pr: TaskPR,
): TaskCIPRAutomationState | undefined {
  const repositoryID = pr.repository_id ?? "";
  return states?.find(
    (state) => state.pr_number === pr.pr_number && state.repository_id === repositoryID,
  );
}

export function autoFixRoundForState(
  state: TaskCIPRAutomationState | undefined,
  maxRounds: number | null | undefined,
): AutoFixRoundInfo {
  const max = normalizeAutoFixMaxRounds(maxRounds);
  const current = clampAutoFixRound(state?.auto_fix_round_count, max);
  return {
    current,
    max,
    exhausted: Boolean(state?.auto_fix_exhausted_at),
  };
}

export function normalizeAutoFixMaxRounds(value: number | null | undefined) {
  if (typeof value !== "number" || !Number.isFinite(value)) return DEFAULT_AUTO_FIX_MAX_ROUNDS;
  return Math.max(1, Math.trunc(value));
}

export function clampAutoFixRound(value: number | null | undefined, maxRounds: number) {
  if (typeof value !== "number" || !Number.isFinite(value)) return 0;
  return Math.min(maxRounds, Math.max(0, Math.trunc(value)));
}
