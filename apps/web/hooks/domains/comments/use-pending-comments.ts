import { useMemo } from 'react';
import { useCommentsStore } from '@/lib/state/slices/comments';
import type { Comment, DiffComment, PlanComment } from '@/lib/state/slices/comments';
import { isDiffComment, isPlanComment } from '@/lib/state/slices/comments';

const EMPTY_COMMENTS: Comment[] = [];
const EMPTY_DIFF_COMMENTS: DiffComment[] = [];
const EMPTY_PLAN_COMMENTS: PlanComment[] = [];

/**
 * Get all pending comments (any source).
 */
export function usePendingComments(): Comment[] {
  const byId = useCommentsStore((state) => state.byId);
  const pendingForChat = useCommentsStore((state) => state.pendingForChat);

  return useMemo(() => {
    if (pendingForChat.length === 0) return EMPTY_COMMENTS;
    const pending: Comment[] = [];
    for (const id of pendingForChat) {
      const comment = byId[id];
      if (comment) pending.push(comment);
    }
    return pending.length === 0 ? EMPTY_COMMENTS : pending;
  }, [byId, pendingForChat]);
}

/**
 * Get all pending diff comments.
 */
export function usePendingDiffComments(): DiffComment[] {
  const byId = useCommentsStore((state) => state.byId);
  const pendingForChat = useCommentsStore((state) => state.pendingForChat);

  return useMemo(() => {
    if (pendingForChat.length === 0) return EMPTY_DIFF_COMMENTS;
    const pending: DiffComment[] = [];
    for (const id of pendingForChat) {
      const comment = byId[id];
      if (comment && isDiffComment(comment)) pending.push(comment);
    }
    return pending.length === 0 ? EMPTY_DIFF_COMMENTS : pending;
  }, [byId, pendingForChat]);
}

/**
 * Get all pending plan comments.
 */
export function usePendingPlanComments(): PlanComment[] {
  const byId = useCommentsStore((state) => state.byId);
  const pendingForChat = useCommentsStore((state) => state.pendingForChat);

  return useMemo(() => {
    if (pendingForChat.length === 0) return EMPTY_PLAN_COMMENTS;
    const pending: PlanComment[] = [];
    for (const id of pendingForChat) {
      const comment = byId[id];
      if (comment && isPlanComment(comment)) pending.push(comment);
    }
    return pending.length === 0 ? EMPTY_PLAN_COMMENTS : pending;
  }, [byId, pendingForChat]);
}
