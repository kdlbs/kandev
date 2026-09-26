import type {
  CIAutomationQueueRemovalCause,
  TaskCIPRAutomationState,
  TaskPR,
  TaskPRAutomationOptions,
} from "@/lib/types/github";

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

export type CIAutomationQueueStatus =
  | "none"
  | "queued"
  | "removed_actionable"
  | "removed_not_actionable"
  | "repair_requested"
  | "waiting_for_commit"
  | "waiting_for_checks";

export type CIAutomationQueueRecoveryState = {
  context: "normal" | "queued" | "recovery";
  status: CIAutomationQueueStatus;
  removalCause: CIAutomationQueueRemovalCause;
  repairAccepted: boolean;
  waitingForCommit: boolean;
};

const ACTIONABLE_QUEUE_REMOVAL_CAUSES: CIAutomationQueueRemovalCause[] = [
  "checks_failed",
  "checks_timed_out",
  "conflict",
];

function normalizedQueueRemovalReason(value: string | undefined): string {
  return (value ?? "")
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "_")
    .replace(/^_+|_+$/g, "");
}

const QUEUE_REMOVAL_CAUSE_ALIASES: Record<string, CIAutomationQueueRemovalCause> = {
  checks_failed: "checks_failed",
  checks_timed_out: "checks_timed_out",
  conflict: "conflict",
  manual: "manual",
  branch_protection: "branch_protection",
  check_failed: "checks_failed",
  checks_failure: "checks_failed",
  ci_checks_failed: "checks_failed",
  ci_check_failed: "checks_failed",
  checks_failed_on_merge_group: "checks_failed",
  checks_failed_on_merge_queue: "checks_failed",
  ci_checks_failed_on_merge_group: "checks_failed",
  ci_checks_failed_on_merge_queue: "checks_failed",
  check_timed_out: "checks_timed_out",
  checks_timeout: "checks_timed_out",
  timeout: "checks_timed_out",
  timed_out: "checks_timed_out",
  checks_timed_out_on_merge_group: "checks_timed_out",
  merge_conflict: "conflict",
  merge_conflicts: "conflict",
  unmergeable: "conflict",
  removed_manually: "manual",
  user_removed: "manual",
  branch_protection_failed: "branch_protection",
  required_branch_protection: "branch_protection",
};

function normalizeQueueRemovalCause(
  value: string | undefined,
  pr?: TaskPR,
): CIAutomationQueueRemovalCause {
  const normalized = normalizedQueueRemovalReason(value);
  const knownCause = QUEUE_REMOVAL_CAUSE_ALIASES[normalized];
  if (knownCause) {
    return knownCause;
  }
  if (pr) {
    const mergeableState = pr.mergeable_state?.trim().toLowerCase() ?? "";
    const queueState = pr.merge_queue_state?.trim().toLowerCase() ?? "";
    if (
      mergeableState === "dirty" ||
      mergeableState.includes("conflict") ||
      queueState === "unmergeable" ||
      queueState.includes("unmergeable")
    ) {
      return "conflict";
    }
  }
  return "unknown";
}

function hasActiveQueueEntry(pr: TaskPR): boolean {
  return pr.state === "open" && Boolean(pr.merge_queue_state?.trim());
}

function isReadyForMerge(pr: TaskPR): boolean {
  if (pr.state !== "open" || pr.checks_state !== "success" || pr.mergeable_state !== "clean") {
    return false;
  }
  if (
    pr.review_state === "changes_requested" ||
    pr.pending_review_count > 0 ||
    pr.unresolved_review_threads > 0
  ) {
    return false;
  }
  return pr.required_reviews == null || pr.review_count >= pr.required_reviews;
}

function queueStateForRemoval(
  pr: TaskPR,
  options: Pick<TaskPRAutomationOptions, "auto_fix_enabled" | "auto_merge_enabled">,
  state: TaskCIPRAutomationState | undefined,
  removalID: string,
): CIAutomationQueueRecoveryState {
  const stateCause = normalizeQueueRemovalCause(state?.last_queue_removal_cause);
  const removalCause =
    stateCause !== "unknown"
      ? stateCause
      : normalizeQueueRemovalCause(pr.merge_queue_last_removal_reason, pr);
  const actionable = ACTIONABLE_QUEUE_REMOVAL_CAUSES.includes(removalCause);
  const headContext = queueHeadContext(pr, state, removalID);

  if (headContext.repairAccepted) {
    return {
      context: "recovery",
      status: "repair_requested",
      removalCause,
      repairAccepted: true,
      waitingForCommit: options.auto_merge_enabled,
    };
  }
  if (options.auto_merge_enabled && headContext.sameHead) {
    return {
      context: "recovery",
      status: "waiting_for_commit",
      removalCause,
      repairAccepted: false,
      waitingForCommit: true,
    };
  }
  if (shouldWaitForNewHead(pr, options, headContext)) {
    return {
      context: "recovery",
      status: "waiting_for_checks",
      removalCause,
      repairAccepted: false,
      waitingForCommit: false,
    };
  }
  return {
    context: "recovery",
    status: actionable ? "removed_actionable" : "removed_not_actionable",
    removalCause,
    repairAccepted: false,
    waitingForCommit: false,
  };
}

function queueHeadContext(
  pr: TaskPR,
  state: TaskCIPRAutomationState | undefined,
  removalID: string,
): {
  currentHead: string;
  lastAttemptHead: string;
  sameHead: boolean;
  repairAccepted: boolean;
} {
  const currentHead = pr.head_sha?.trim() ?? "";
  const lastAttemptHead = state?.last_queue_attempt_head_sha?.trim() ?? "";
  const sameHead = Boolean(currentHead && lastAttemptHead && currentHead === lastAttemptHead);
  return {
    currentHead,
    lastAttemptHead,
    sameHead,
    repairAccepted: state?.last_queue_fix_event_id === removalID && sameHead,
  };
}

function shouldWaitForNewHead(
  pr: TaskPR,
  options: Pick<TaskPRAutomationOptions, "auto_merge_enabled">,
  headContext: ReturnType<typeof queueHeadContext>,
): boolean {
  return Boolean(
    options.auto_merge_enabled &&
    headContext.lastAttemptHead &&
    headContext.currentHead &&
    !headContext.sameHead &&
    !isReadyForMerge(pr),
  );
}

export function deriveCIAutomationQueueState(
  pr: TaskPR,
  options: Pick<TaskPRAutomationOptions, "auto_fix_enabled" | "auto_merge_enabled">,
  state: TaskCIPRAutomationState | undefined,
): CIAutomationQueueRecoveryState {
  if (hasActiveQueueEntry(pr)) {
    return {
      context: "queued",
      status: "queued",
      removalCause: "unknown",
      repairAccepted: false,
      waitingForCommit: false,
    };
  }

  const removalID = pr.merge_queue_last_removal_id?.trim() ?? "";
  if (!removalID) {
    return {
      context: "normal",
      status: "none",
      removalCause: "unknown",
      repairAccepted: false,
      waitingForCommit: false,
    };
  }
  return queueStateForRemoval(pr, options, state, removalID);
}

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
