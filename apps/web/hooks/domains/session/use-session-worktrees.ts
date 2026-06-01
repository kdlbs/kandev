import { useMemo } from "react";
import { useTaskSessionById } from "@/hooks/domains/session/use-task-session-by-id";
import type { Worktree } from "@/lib/state/slices/session/types";

/**
 * Derive the worktree(s) for a session from the canonical TaskSession TQ cache
 * (`qk.taskSession.byId` -> worktree_* fields), populated by the session-state
 * bridge (live WS), the SSR seed, and by-task list fetches.
 *
 * Replaces the former `state.worktrees` / `state.sessionWorktreesBySessionId`
 * Zustand mirror. A session maps to at most one worktree (its own
 * `worktree_id`); the array shape is preserved for existing callers.
 */
export function useSessionWorktrees(sessionId: string | null): Worktree[] {
  const session = useTaskSessionById(sessionId);
  return useMemo(() => {
    if (!sessionId || !session?.worktree_id) return [];
    return [
      {
        id: session.worktree_id,
        sessionId: session.id,
        repositoryId: session.repository_id ?? undefined,
        path: session.worktree_path ?? undefined,
        branch: session.worktree_branch ?? undefined,
      },
    ];
  }, [sessionId, session]);
}
