export type FileTreeEditorTarget = {
  /** Path relative to the resolved worktree root, as the editors API expects. */
  filePath: string;
  worktreeId?: string;
};

type WorktreeLike = { id: string; path?: string };

function pathBasename(path: string): string {
  return path.split(/[\\/]/).filter(Boolean).pop() ?? "";
}

/**
 * Maps a Files-panel tree path onto the `{worktreeId, filePath}` pair the
 * editors API resolves against, or `null` when the node is not addressable by
 * that API.
 *
 * The Files tree is served either from inside a worktree — in which case node
 * paths are already worktree-relative — or from the task workspace root, where
 * the first segment names a workspace entry. A task reaches the second shape
 * both by holding several worktrees and by having any source attached through
 * *Add Repositories to workspace*, which rebinds the session to the task root
 * even when it adds no worktree. The worktree count therefore cannot tell the
 * two apart; the tree's own root directory name can.
 *
 * Entries that are not worktrees (an attached plain folder) have no worktree to
 * resolve against, and the editors API rejects paths that escape the worktree,
 * so those nodes return `null` and callers hide the action.
 */
export function resolveFileTreeEditorTarget(
  nodePath: string,
  worktrees: WorktreeLike[],
  treeRootName?: string,
): FileTreeEditorTarget | null {
  const known = worktrees.filter((worktree) => worktree.path);
  // Repository-backed sessions without a worktree fall back to the repository
  // checkout server-side, and an unloaded tree has no root to compare against.
  // Both keep the pass-through behaviour rather than hiding the action.
  if (known.length === 0 || !treeRootName) return { filePath: nodePath };

  const rooted = known.find((worktree) => pathBasename(worktree.path!) === treeRootName);
  if (rooted) return { filePath: nodePath, worktreeId: rooted.id };

  const separatorIndex = nodePath.indexOf("/");
  const head = separatorIndex === -1 ? nodePath : nodePath.slice(0, separatorIndex);
  const rest = separatorIndex === -1 ? "" : nodePath.slice(separatorIndex + 1);
  const match = known.find((worktree) => pathBasename(worktree.path!) === head);
  if (!match) return null;
  return { filePath: rest, worktreeId: match.id };
}
