import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { sessionWorktreesQueryOptions, taskSessionQueryOptions } from "@/lib/query/query-options";
import type { Worktree } from "@/lib/state/slices/session/types";
import type { TaskSession } from "@/lib/types/http";

function primaryWorktreeFromSession(session: TaskSession | null | undefined): Worktree | null {
  if (!session?.worktree_id) return null;
  return {
    id: session.worktree_id,
    sessionId: session.id,
    repositoryId: session.repository_id ?? undefined,
    path: session.worktree_path ?? undefined,
    branch: session.worktree_branch ?? undefined,
  };
}

function mergePrimaryWorktree(worktrees: Worktree[], session: TaskSession | null | undefined) {
  const primary = primaryWorktreeFromSession(session);
  if (!primary || worktrees.some((worktree) => worktree.id === primary.id)) return worktrees;
  return [primary, ...worktrees];
}

export function useSessionWorktrees(sessionId: string | null) {
  const sessionQuery = useQuery(taskSessionQueryOptions(sessionId ?? ""));
  const session = sessionQuery.data;
  const worktreesQuery = useQuery(sessionWorktreesQueryOptions(sessionId ?? ""));

  return useMemo(() => {
    if (!sessionId) return [];
    if (worktreesQuery.data?.length) {
      return mergePrimaryWorktree(worktreesQuery.data, session);
    }
    const primary = primaryWorktreeFromSession(session);
    if (primary) return [primary];
    return [];
  }, [
    session?.id,
    session?.repository_id,
    session?.worktree_branch,
    session?.worktree_id,
    session?.worktree_path,
    sessionId,
    worktreesQuery.data,
  ]);
}
