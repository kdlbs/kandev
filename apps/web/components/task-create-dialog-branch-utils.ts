/**
 * Pure helpers for the branch chip in the task-create dialog.
 *
 * Extracted from `task-create-dialog-repo-chips.tsx` to keep that file under
 * the 600-line cap. Tested directly in `task-create-dialog-branch-utils.test.ts`.
 */
import { t } from "@/lib/i18n";

/**
 * What the picked branch means for this row.
 *
 * This used to be the prefix STRING itself: `computeBranchPrefix` returned
 * `"will switch to: "` and `computeBranchTooltip` matched it with `===`. That
 * made the qualifier both display copy and a comparison token, so translating
 * it would have silently collapsed every tooltip to the generic fallback in
 * every non-English locale. The discriminant carries the meaning; the copy
 * hangs off it.
 */
export type BranchIntent = "none" | "current" | "switch" | "from";

/**
 * Decide what the picked branch means, so the chip can qualify it.
 *   "current" — local exec, picked branch matches the current checkout
 *               (also covers detached HEAD where the "branch" is the short SHA
 *               returned by the backend).
 *   "switch"  — local exec, picked branch differs from current. Surfaces the
 *               destructive `git checkout` decision the backend makes on submit.
 *   "from"    — fork mode (forking a new branch off this base) or non-local
 *               executors (worktree base).
 *   "none"    — no value yet, so the chip's placeholder ("branch") does not get
 *               a misleading prefix.
 */
export function computeBranchIntent({
  isLocalExecutor,
  rowBranch,
  currentLocalBranch,
  freshBranchEnabled,
}: {
  isLocalExecutor: boolean;
  rowBranch: string;
  currentLocalBranch: string;
  freshBranchEnabled: boolean;
}): BranchIntent {
  if (!rowBranch) return "none";
  if (freshBranchEnabled) return "from";
  if (isLocalExecutor) {
    if (currentLocalBranch && rowBranch === currentLocalBranch) return "current";
    return "switch";
  }
  return "from";
}

const PREFIX_KEYS: Record<Exclude<BranchIntent, "none">, string> = {
  current: "task:branchPrefixCurrent",
  switch: "task:branchPrefixWillSwitchTo",
  from: "task:branchPrefixFrom",
};

/** Muted text shown before the branch value, or "" when there is nothing to qualify. */
export function computeBranchPrefix(intent: BranchIntent): string {
  return intent === "none" ? "" : t(PREFIX_KEYS[intent]);
}

const TOOLTIP_KEYS: Record<Exclude<BranchIntent, "none">, string> = {
  current: "task:branchTooltipCurrent",
  switch: "task:branchTooltipWillSwitchTo",
  from: "task:branchTooltipFrom",
};

/**
 * Branch chip hover tooltip, matched to the intent so the explanation lines up
 * with the qualifier the chip is showing.
 */
export function computeBranchTooltip(intent: BranchIntent | undefined): string {
  if (!intent || intent === "none") return t("task:branchTooltipGeneric");
  return t(TOOLTIP_KEYS[intent]);
}

export function computeBranchDisabledReason({
  branchLocked,
  hasRepo,
  branchesLoading,
  optionCount,
}: {
  branchLocked: boolean;
  hasRepo: boolean;
  branchesLoading: boolean;
  optionCount: number;
}): string | undefined {
  if (branchLocked) return t("task:branchDisabledLocalExecutor");
  if (!hasRepo) return t("task:branchDisabledSelectRepositoryFirst");
  if (branchesLoading) return t("task:loadingBranches");
  if (optionCount === 0) return t("task:branchDisabledNoBranches");
  return undefined;
}
