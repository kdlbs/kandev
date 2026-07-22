import { resolvePRReviewRepositoryName, sanitizeReviewRepositoryName } from "./types";

type ReviewPRIdentity = {
  repository_id?: string;
  repo: string;
  head_branch?: string;
};

export type ReviewTaskRepositoryIdentity = {
  repository_id: string;
  base_branch?: string;
  checkout_branch?: string;
  position: number;
};

export type ReviewWorktreeIdentity = {
  id?: string;
  repositoryId?: string;
  branchSlug?: string;
  branch?: string;
  path?: string;
  position?: number;
};

type ResolvePRReviewRepositoryIdentityInput = {
  pr: ReviewPRIdentity | null | undefined;
  workspaceRepositoryName?: string | null;
  taskRepositories: ReviewTaskRepositoryIdentity[];
  worktrees: ReviewWorktreeIdentity[];
};

function worktreeDirectoryName(path: string | undefined): string | undefined {
  const directory = path
    ?.replace(/[\\/]+$/, "")
    .split(/[\\/]/)
    .pop();
  return directory ? sanitizeReviewRepositoryName(directory) || undefined : undefined;
}

function findPRTaskRepository(
  pr: ReviewPRIdentity,
  taskRepositories: ReviewTaskRepositoryIdentity[],
): ReviewTaskRepositoryIdentity | undefined {
  const matchingRepository = taskRepositories.filter(
    (taskRepository) => taskRepository.repository_id === pr.repository_id,
  );
  return (
    matchingRepository.find(
      (taskRepository) => taskRepository.checkout_branch === pr.head_branch,
    ) ??
    matchingRepository.find((taskRepository) => taskRepository.base_branch === pr.head_branch) ??
    (matchingRepository.length === 1 ? matchingRepository[0] : undefined)
  );
}

function findPRWorktree(
  pr: ReviewPRIdentity,
  taskRepository: ReviewTaskRepositoryIdentity | undefined,
  worktrees: ReviewWorktreeIdentity[],
): ReviewWorktreeIdentity | undefined {
  const branchSlug = sanitizeReviewRepositoryName(pr.head_branch ?? "");
  return worktrees.find((worktree) => {
    if (worktree.repositoryId !== pr.repository_id) return false;
    if (pr.head_branch && worktree.branch === pr.head_branch) return true;
    if (branchSlug && worktree.branchSlug === branchSlug) return true;
    return taskRepository !== undefined && worktree.position === taskRepository.position;
  });
}

/**
 * Resolves the repository_name stamped by agentctl for a selected PR. Same-repo
 * multi-branch tasks use sibling worktree directories (`repo-branch-slug`), so
 * the canonical workspace repo name alone is not a stable review identity.
 */
export function resolvePRReviewRepositoryIdentity({
  pr,
  workspaceRepositoryName,
  taskRepositories,
  worktrees,
}: ResolvePRReviewRepositoryIdentityInput): string | undefined {
  const canonicalName = resolvePRReviewRepositoryName(pr, workspaceRepositoryName);
  if (!pr?.repository_id || !canonicalName) return canonicalName;

  const taskRepository = findPRTaskRepository(pr, taskRepositories);
  const worktreeName = worktreeDirectoryName(findPRWorktree(pr, taskRepository, worktrees)?.path);
  if (worktreeName) return worktreeName;

  const siblingRepositories = taskRepositories
    .filter((candidate) => candidate.repository_id === pr.repository_id)
    .sort((left, right) => left.position - right.position);
  if (!taskRepository || siblingRepositories.length < 2) return canonicalName;
  if (taskRepository === siblingRepositories[0]) return canonicalName;

  const branch = taskRepository.checkout_branch || taskRepository.base_branch || pr.head_branch;
  const branchSlug = sanitizeReviewRepositoryName(branch ?? "");
  return branchSlug ? `${canonicalName}-${branchSlug}` : canonicalName;
}
