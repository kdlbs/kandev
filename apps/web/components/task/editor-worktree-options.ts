import type { Worktree } from "@/lib/state/slices/session/types";
import type { Repository } from "@/lib/types/http";

export type WorktreeOption = {
  worktreeId: string;
  label: string;
  branch?: string;
};

function worktreePathBasename(path: string | undefined): string | undefined {
  if (!path) return undefined;
  return path.split(/[\\/]/).filter(Boolean).pop();
}

/**
 * Builds display options for a session's worktrees: labeled by repository name
 * when known, otherwise by the worktree folder name.
 */
export function buildWorktreeOptions(
  worktrees: Worktree[],
  repositories: Repository[],
): WorktreeOption[] {
  return worktrees.map((worktree, index) => {
    const repository = repositories.find((repo) => repo.id === worktree.repositoryId);
    const label =
      repository?.name ?? worktreePathBasename(worktree.path) ?? `Worktree ${index + 1}`;
    return { worktreeId: worktree.id, label, branch: worktree.branch };
  });
}
