import type { TaskPR } from "@/lib/types/github";

/** Shared label for the PR dockview panel tab and dropdown items. */
export function prPanelLabel(prNumber: number): string {
  return `PR #${prNumber}`;
}

/**
 * Stable per-PR identifier for React keys, DOM ids, and e2e test ids.
 * Includes owner + repo + PR number so two repos that share a name (or two
 * owners that share a repo name) never collide on PR number alone.
 */
export function prIdentitySlug(pr: TaskPR): string {
  return `${pr.owner}-${pr.repo}-${pr.pr_number}`;
}

/** Stable per-PR key used by task-scoped state and dockview panels. */
export function prTaskKey(pr: TaskPR): string {
  return `${pr.owner}/${pr.repo}/${pr.pr_number}`;
}
