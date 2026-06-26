import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { sessionWorktreesQueryOptions, taskSessionQueryOptions } from "@/lib/query/query-options";

export function useSessionWorktrees(sessionId: string | null) {
  const sessionQuery = useQuery(taskSessionQueryOptions(sessionId ?? ""));
  const session = sessionQuery.data;
  const worktreesQuery = useQuery(sessionWorktreesQueryOptions(sessionId ?? ""));

  return useMemo(() => {
    if (!sessionId) return [];
    if (worktreesQuery.data?.length) {
      return worktreesQuery.data;
    }
    if (session?.worktree_id) {
      return [
        {
          id: session.worktree_id,
          sessionId: session.id,
          repositoryId: session.repository_id ?? undefined,
          path: session.worktree_path ?? undefined,
          branch: session.worktree_branch ?? undefined,
        },
      ];
    }
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
