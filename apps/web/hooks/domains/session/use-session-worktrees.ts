import { useMemo } from 'react';
import { useAppStore } from '@/components/state-provider';
import { useSession } from '@/hooks/domains/session/use-session';

export function useSessionWorktrees(sessionId: string | null) {
  const { session } = useSession(sessionId);
  const worktrees = useAppStore((state) => state.worktrees.items);
  const sessionWorktreesBySessionId = useAppStore(
    (state) => state.sessionWorktreesBySessionId.itemsBySessionId
  );

  return useMemo(() => {
    if (!sessionId) return [];
    const worktreeIds = sessionWorktreesBySessionId[sessionId];
    if (worktreeIds?.length) {
      return worktreeIds.map((id: string) => worktrees[id]).filter(Boolean);
    }
    if (session?.worktree_id) {
      const worktree = worktrees[session.worktree_id];
      return worktree ? [worktree] : [];
    }
    return [];
  }, [session?.worktree_id, sessionId, sessionWorktreesBySessionId, worktrees]);
}
