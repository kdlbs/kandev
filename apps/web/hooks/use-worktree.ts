import { useMemo } from "react";
import { useAllTaskSessions } from "@/hooks/domains/session/use-task-session-by-id";
import type { Worktree } from "@/lib/state/slices/session/types";

/**
 * Derive a single worktree by id from the canonical TaskSession TQ cache.
 *
 * Replaces the former `state.worktrees.items[worktreeId]` Zustand mirror. The
 * worktree fields live on the owning TaskSession (`worktree_id` / `_path` /
 * `_branch`), populated by the session-state bridge, the SSR seed, and by-task
 * list fetches. Returns `null` until a session carrying that worktree_id is
 * cached.
 */
export function useWorktree(worktreeId: string | null): Worktree | null {
  const sessions = useAllTaskSessions();
  return useMemo(() => {
    if (!worktreeId) return null;
    const session = sessions.find((s) => s.worktree_id === worktreeId);
    if (!session) return null;
    return {
      id: worktreeId,
      sessionId: session.id,
      repositoryId: session.repository_id ?? undefined,
      path: session.worktree_path ?? undefined,
      branch: session.worktree_branch ?? undefined,
    };
  }, [worktreeId, sessions]);
}
