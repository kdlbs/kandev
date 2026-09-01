import type { Repository, RepositorySet } from "@/lib/types/http";
import type { TaskRepoRow } from "@/components/task-create-dialog-types";

/**
 * Applying a repository set fills the picker with one row per member. The rules
 * exist so the action is safe to repeat and never loses the user's work:
 *
 *   - additive and idempotent: a repository already in the form is skipped, so
 *     applying the same set twice changes nothing and two overlapping sets give
 *     the union;
 *   - a single blank placeholder row is consumed rather than left behind;
 *   - a row the user configured is never discarded or reordered;
 *   - a member that no longer resolves to a live workspace repository is skipped
 *     and counted, so the UI can say how many were dropped and why;
 *   - a saved base is copied to baseBranch on the new row. The ordinary branch
 *     field stays empty so a local executor can still autoselect the current
 *     checkout branch without using the saved base as checkout state.
 *
 * Kept as a pure function so those rules are testable without rendering the
 * dialog.
 */

/** Prefix for keys of rows this module adds. */
const APPLIED_ROW_KEY_PREFIX = "set-row-";

export type ApplyRepositorySetInput = {
  rows: TaskRepoRow[];
  set: RepositorySet;
  /** The workspace's live repositories, as the picker already has them. */
  repositories: Repository[];
};

export type ApplyRepositorySetResult = {
  rows: TaskRepoRow[];
  /** Members turned into a new row. */
  addedCount: number;
  /** Members skipped because the form already had that repository. */
  alreadyPresentCount: number;
  /** Members skipped because they no longer resolve to a live repository. */
  missingCount: number;
};

export function applyRepositorySet({
  rows,
  set,
  repositories,
}: ApplyRepositorySetInput): ApplyRepositorySetResult {
  const available = new Set(repositories.map((repository) => repository.id as string));
  const present = new Set(selectedRepositoryIdsForSet(rows));
  const takenKeys = new Set(rows.map((row) => row.key));

  const added: TaskRepoRow[] = [];
  let alreadyPresentCount = 0;
  let missingCount = 0;

  for (const member of set.repositories) {
    const id = member.repository_id as string;
    if (!available.has(id)) {
      missingCount += 1;
      continue;
    }
    if (present.has(id)) {
      alreadyPresentCount += 1;
      continue;
    }
    present.add(id);
    const savedBase = member.base_branch || undefined;
    added.push({
      key: nextAppliedRowKey(takenKeys),
      repositoryId: id,
      branch: "",
      ...(savedBase ? { baseBranch: savedBase } : {}),
    });
  }

  return {
    rows: mergeAppliedRows(rows, added),
    addedCount: added.length,
    alreadyPresentCount,
    missingCount,
  };
}

/**
 * Puts the applied rows after the existing ones, dropping a lone blank
 * placeholder. Nothing is dropped when there is nothing to add: consuming the
 * placeholder then would leave the form with no row at all.
 */
function mergeAppliedRows(rows: TaskRepoRow[], added: TaskRepoRow[]): TaskRepoRow[] {
  if (added.length === 0) return rows;
  const keep = isLonePlaceholder(rows) ? [] : rows;
  return [...keep, ...added];
}

/**
 * A single row with nothing chosen at all. A row carrying only a branch counts as
 * the user's work and is kept.
 */
function isLonePlaceholder(rows: TaskRepoRow[]): boolean {
  if (rows.length !== 1) return false;
  const row = rows[0];
  return Boolean(row) && !row.repositoryId && !row.localPath && !row.branch;
}

/**
 * Allocates a key no current row holds, using a prefix distinct from the
 * `row-N` sequence `useRepositoriesState` hands out. Sharing that sequence would
 * let a later `addRepository()` mint a key an applied row already uses, which
 * duplicates React keys and breaks the row's uncontrolled inputs.
 */
function nextAppliedRowKey(takenKeys: Set<string>): string {
  let index = takenKeys.size;
  let key = `${APPLIED_ROW_KEY_PREFIX}${index}`;
  while (takenKeys.has(key)) {
    index += 1;
    key = `${APPLIED_ROW_KEY_PREFIX}${index}`;
  }
  takenKeys.add(key);
  return key;
}

/**
 * The workspace repository ids currently chosen in the form, in row order and
 * deduped. Discovered local-path rows are excluded: they are not workspace
 * repository entities, so they cannot be members of a set.
 */
export function selectedRepositoryIdsForSet(rows: TaskRepoRow[]): string[] {
  const seen = new Set<string>();
  const ids: string[] = [];
  for (const row of rows) {
    const id = row.repositoryId;
    if (!id || seen.has(id)) continue;
    seen.add(id);
    ids.push(id);
  }
  return ids;
}

/**
 * Projects the current task rows into the ordered member shape used by the
 * repository-set API. Local rows use the repository default as their effective
 * base because their row branch is the checkout branch on disk.
 */
export function selectedRepositoryMembersForSet(
  rows: TaskRepoRow[],
  repositories: Repository[] = [],
  isLocalExecutor = false,
): Array<{ repositoryId: string; baseBranch: string }> {
  const byId = new Map(repositories.map((repository) => [repository.id as string, repository]));
  const seen = new Set<string>();
  return rows.flatMap((row) => {
    const repositoryId = row.repositoryId;
    if (!repositoryId || seen.has(repositoryId)) return [];
    seen.add(repositoryId);
    const repositoryDefault = byId.get(repositoryId)?.default_branch ?? "";
    const baseBranch = row.baseBranch || (isLocalExecutor ? repositoryDefault : row.branch) || "";
    return [{ repositoryId, baseBranch }];
  });
}
