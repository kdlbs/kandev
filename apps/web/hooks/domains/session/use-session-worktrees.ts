import { useMemo } from "react";
import { useAppStore } from "@/components/state-provider";
import { useSession } from "@/hooks/domains/session/use-session";
import type { Worktree } from "@/lib/state/slices/session/types";
import type { TaskSession } from "@/lib/types/http";

/**
 * Resolves the worktrees of a session by unioning the API-provided
 * `session.worktrees` (complete, survives reloads) with the WS-populated
 * per-session list (fresher after live add-branch events), filling in details
 * from the WS worktrees map. Falls back to the legacy single `worktree_id`
 * field when neither source has entries.
 */
export function resolveSessionWorktrees(
  sessionId: string,
  session: TaskSession | null,
  worktrees: Record<string, Worktree>,
  sessionWorktreeIds: string[] | undefined,
): Worktree[] {
  const result: Worktree[] = [...(session?.worktrees ?? [])]
    .sort((a, b) => a.position - b.position)
    .map((wt) => {
      const id = wt.worktree_id || wt.id;
      const live = worktrees[id];
      // `||` (not `??`): the Go backend may serialize these as empty strings,
      // which should also fall back to the live WS-populated values.
      return {
        id,
        sessionId,
        repositoryId: wt.repository_id || live?.repositoryId,
        path: wt.worktree_path || live?.path,
        branch: wt.worktree_branch || live?.branch,
      };
    });
  const seen = new Set(result.map((wt) => wt.id));
  for (const id of sessionWorktreeIds ?? []) {
    const live = worktrees[id];
    if (!live || seen.has(id)) continue;
    seen.add(id);
    result.push(live);
  }
  if (result.length === 0 && session?.worktree_id) {
    const worktree = worktrees[session.worktree_id];
    return worktree ? [worktree] : [];
  }
  return result;
}

export function useSessionWorktrees(sessionId: string | null) {
  const { session } = useSession(sessionId);
  const worktrees = useAppStore((state) => state.worktrees.items);
  const sessionWorktreesBySessionId = useAppStore(
    (state) => state.sessionWorktreesBySessionId.itemsBySessionId,
  );

  return useMemo(() => {
    if (!sessionId) return [];
    return resolveSessionWorktrees(
      sessionId,
      session,
      worktrees,
      sessionWorktreesBySessionId[sessionId],
    );
  }, [session, sessionId, sessionWorktreesBySessionId, worktrees]);
}
