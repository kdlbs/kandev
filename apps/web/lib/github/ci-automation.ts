import type { TaskCIPRAutomationState, TaskPR } from "@/lib/types/github";

const DEFAULT_AUTO_FIX_MAX_ROUNDS = 10;

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
