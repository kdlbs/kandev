import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { sessionWorktreesQueryOptions, taskSessionQueryOptions } from "@/lib/query/query-options";
import type { Worktree } from "@/lib/state/slices/session/types";
import type { TaskSession } from "@/lib/types/http";

export function resolveSessionWorktrees(
  sessionId: string,
  session: TaskSession | null | undefined,
  worktrees: Record<string, Worktree>,
  sessionWorktreeIds: string[] | undefined,
): Worktree[] {
  const result: Worktree[] = [...(session?.worktrees ?? [])]
    .sort((a, b) => a.position - b.position)
    .map((item) => {
      const id = item.worktree_id || item.id;
      const live = worktrees[id];
      return {
        id,
        sessionId,
        repositoryId: item.repository_id || live?.repositoryId,
        path: item.worktree_path || live?.path,
        branch: item.worktree_branch || live?.branch,
      };
    });
  const seen = new Set(result.map((item) => item.id));
  for (const id of sessionWorktreeIds ?? []) {
    const live = worktrees[id];
    if (!live || seen.has(id)) continue;
    result.push(live);
    seen.add(id);
  }
  if (result.length === 0 && session?.worktree_id) {
    const live = worktrees[session.worktree_id];
    return live ? [live] : [];
  }
  return result;
}

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
    if (session?.worktrees?.length) {
      const worktrees = Object.fromEntries(
        (worktreesQuery.data ?? []).map((item) => [item.id, item]),
      );
      return resolveSessionWorktrees(
        sessionId,
        session,
        worktrees,
        worktreesQuery.data?.map((item) => item.id),
      );
    }
    if (worktreesQuery.data?.length) return mergePrimaryWorktree(worktreesQuery.data, session);
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
