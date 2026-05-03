/**
 * Pure helpers for the branch chip in the task-create dialog.
 *
 * Extracted from `task-create-dialog-repo-chips.tsx` to keep that file under
 * the 600-line cap. No React deps — these are string functions tested
 * directly in `task-create-dialog-branch-utils.test.ts`.
 */

/**
 * Decide the muted text shown before the branch value to qualify intent.
 *   "current: "        — local exec, picked branch matches the current checkout
 *                        (also covers detached HEAD where the "branch" is the
 *                        short SHA returned by the backend).
 *   "will switch to: " — local exec, picked branch differs from current.
 *                        Surfaces the destructive `git checkout` decision the
 *                        backend will make on submit.
 *   "from: "           — fork mode (forking a new branch off this base) or
 *                        non-local executors (worktree base).
 *
 * Returns "" when there's no value yet, so the chip's placeholder ("branch")
 * doesn't get a misleading prefix.
 */
export function computeBranchPrefix({
  isLocalExecutor,
  rowBranch,
  currentLocalBranch,
  freshBranchEnabled,
}: {
  isLocalExecutor: boolean;
  rowBranch: string;
  currentLocalBranch: string;
  freshBranchEnabled: boolean;
}): string {
  if (!rowBranch) return "";
  if (freshBranchEnabled) return "from: ";
  if (isLocalExecutor) {
    if (currentLocalBranch && rowBranch === currentLocalBranch) return "current: ";
    return "will switch to: ";
  }
  return "from: ";
}

/**
 * Branch chip hover tooltip, picked to match whatever prefix the chip is
 * currently showing so the explanation lines up with the qualifier text.
 *   "current: "        → no git ops will run; agent uses your existing checkout
 *   "will switch to: " → backend runs `git checkout` before the agent starts
 *   "from: "           → a new branch / worktree is forked off this base
 *   ""                 → no branch picked yet; generic hint
 */
export function computeBranchTooltip(branchPrefix: string | undefined): string {
  if (branchPrefix === "current: ") {
    return "Your repository's current checkout. The agent runs against it as-is, no git operations.";
  }
  if (branchPrefix === "will switch to: ") {
    return "The backend will run `git checkout` to switch your repository to this branch before the agent starts.";
  }
  if (branchPrefix === "from: ") {
    return "Base branch. A new branch (or worktree) is forked from here and the agent runs there.";
  }
  return "Branch the agent will run against.";
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
  if (branchLocked) {
    return "The local executor uses your repository's current checkout, so the branch can't change here. Toggle 'Fork a new branch' to pick a different base.";
  }
  if (!hasRepo) return "Select a repository first.";
  if (branchesLoading) return "Loading branches…";
  if (optionCount === 0) return "No branches available for this repository.";
  return undefined;
}
