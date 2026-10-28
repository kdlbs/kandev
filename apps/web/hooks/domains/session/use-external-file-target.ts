import { useAppStore } from "@/components/state-provider";
import { useSessionWorktrees } from "@/hooks/domains/session/use-session-worktrees";
import type { Worktree } from "@/lib/state/slices/session/types";
import type { TaskSession } from "@/lib/types/http";
import { isWorkspaceTreePath, workspaceRelativeFilePath } from "@/lib/workspace-file-path";

type FileTarget = { worktreeId?: string } | null;
type TargetInput = { repositoryId?: string | null; repositoryName?: string | null };

function isLegacySingleWorktree(session: TaskSession, worktrees: Worktree[]): boolean {
  if (session.worktrees?.length || worktrees.length !== 1 || !session.worktree_id) return false;
  const worktree = worktrees[0];
  return !worktree.repositoryId && worktree.id === session.worktree_id;
}

function targetForRepositoryId(
  session: TaskSession,
  worktrees: Worktree[],
  namedMatches: Worktree[],
  id: string,
  name?: string,
): FileTarget {
  const matches = worktrees.filter((worktree) => worktree.repositoryId === id);
  if (matches.length === 1 && matches[0].id) {
    if (namedMatches.length === 1 && namedMatches[0].id !== matches[0].id) return null;
    return { worktreeId: matches[0].id };
  }
  if (matches.length || name || session.repository_id !== id) return null;
  if (worktrees.length === 0 || isLegacySingleWorktree(session, worktrees)) return {};
  return null;
}

function unscopedTarget(session: TaskSession, worktrees: Worktree[]): FileTarget {
  if (worktrees.length === 1 && worktrees[0].id) return { worktreeId: worktrees[0].id };
  if (worktrees.length > 0) return null;
  return session.worktree_id || session.worktree_path || session.repository_id ? {} : null;
}

export function resolveExternalFileTarget(
  session: TaskSession | null,
  worktrees: Worktree[],
  { repositoryId, repositoryName }: TargetInput,
): FileTarget {
  if (!session) return null;
  const id = repositoryId || undefined;
  const name = repositoryName || undefined;
  const namedMatches =
    name && session.workspace_path && isWorkspaceTreePath(name)
      ? worktrees.filter(
          (worktree) =>
            worktree.path &&
            workspaceRelativeFilePath(worktree.path, session.workspace_path) === name,
        )
      : [];

  if (id) return targetForRepositoryId(session, worktrees, namedMatches, id, name);
  if (name) {
    return namedMatches.length === 1 && namedMatches[0].id
      ? { worktreeId: namedMatches[0].id }
      : null;
  }
  return unscopedTarget(session, worktrees);
}

export function useExternalFileTarget(sessionId: string | null, input: TargetInput): FileTarget {
  const session = useAppStore((state) =>
    sessionId ? (state.taskSessions.items[sessionId] ?? null) : null,
  );
  const worktrees = useSessionWorktrees(sessionId, false);
  return resolveExternalFileTarget(session, worktrees, input);
}
