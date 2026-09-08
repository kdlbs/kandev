import type {
  TaskMR,
  TaskMRAutomationOptionsForMR,
  TaskMRLifecycleState,
} from "@/lib/types/gitlab";

const DEFAULT_AUTO_FIX_MAX_ROUNDS = 10;

export const DISABLED_MR_AUTOMATION_SWITCHES: Omit<
  TaskMRAutomationOptionsForMR,
  "task_id" | "repository_id" | "project_path" | "mr_iid" | "created_at" | "updated_at"
> = {
  auto_fix_enabled: false,
  auto_merge_enabled: false,
  prompt_on_review_requested: false,
  prompt_on_merged: false,
  prompt_on_closed: false,
};

/**
 * Selects the given MR's own automation switches out of the task-scoped
 * `mr_options` array, falling back to all-off defaults when the MR has no
 * stored row yet (never configured). Mirrors findMRAutomationStateForMR.
 */
export function findMRAutomationOptionsForMR(
  options: TaskMRAutomationOptionsForMR[] | undefined,
  mr: TaskMR,
): TaskMRAutomationOptionsForMR {
  const repositoryID = mr.repository_id ?? "";
  const found = options?.find(
    (option) =>
      option.mr_iid === mr.mr_iid &&
      option.project_path === mr.project_path &&
      option.repository_id === repositoryID,
  );
  if (found) return found;
  return {
    task_id: mr.task_id,
    repository_id: repositoryID,
    project_path: mr.project_path,
    mr_iid: mr.mr_iid,
    created_at: "",
    updated_at: "",
    ...DISABLED_MR_AUTOMATION_SWITCHES,
  };
}

export type AutoFixRoundInfo = {
  current: number;
  max: number;
  exhausted: boolean;
};

export function findMRAutomationStateForMR(
  states: TaskMRLifecycleState[] | undefined,
  mr: TaskMR,
): TaskMRLifecycleState | undefined {
  const repositoryID = mr.repository_id ?? "";
  return states?.find(
    (state) =>
      state.mr_iid === mr.mr_iid &&
      state.project_path === mr.project_path &&
      state.repository_id === repositoryID,
  );
}

export function autoFixRoundForState(
  state: TaskMRLifecycleState | undefined,
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
