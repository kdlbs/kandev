import type { PlanCommentMigrationState } from "@/lib/state/slices/session/types";

export const EMPTY_PLAN_COMMENT_MIGRATION: PlanCommentMigrationState = {
  status: "idle",
  pendingCount: 0,
  failure: null,
};

/** Only identified, unacknowledged feedback can be omitted by composer Send. */
export function planCommentRecovery(state = EMPTY_PLAN_COMMENT_MIGRATION) {
  const isBlocking = state.pendingCount > 0;
  return {
    ...state,
    isBlocking,
    isReady: !isBlocking,
    needsAttention:
      isBlocking && (state.status === "failed" || state.status === "waiting_for_plan"),
  };
}

/** Three connected attempts, then a capped background backoff. */
export function planCommentRecoveryDelay(failures: number) {
  if (failures < 3) return failures * 1000;
  return Math.min(30_000 * 2 ** (failures - 3), 120_000);
}
