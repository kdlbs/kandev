import { useCallback, useEffect, useMemo } from 'react';
import { useCommentsStore } from '@/lib/state/slices/comments';
import type { PlanComment } from '@/lib/state/slices/comments';
import { isPlanComment } from '@/lib/state/slices/comments';

const EMPTY_COMMENTS: PlanComment[] = [];

/**
 * Hook for managing plan comments for a session.
 *
 * Replaces the local-state-then-sync pattern in task-plan-panel.tsx.
 * The store IS the source of truth — no local state needed.
 */
export function usePlanComments(sessionId: string | null | undefined) {
  const byId = useCommentsStore((state) => state.byId);
  const sessionIds = useCommentsStore((state) =>
    sessionId ? state.bySession[sessionId] : undefined
  );
  const addCommentToStore = useCommentsStore((state) => state.addComment);
  const updateCommentInStore = useCommentsStore((state) => state.updateComment);
  const removeCommentFromStore = useCommentsStore((state) => state.removeComment);
  const editingCommentId = useCommentsStore((state) => state.editingCommentId);
  const setEditingComment = useCommentsStore((state) => state.setEditingComment);
  const hydrateSession = useCommentsStore((state) => state.hydrateSession);

  // Hydrate on mount
  useEffect(() => {
    if (sessionId) hydrateSession(sessionId);
  }, [sessionId, hydrateSession]);

  const comments = useMemo((): PlanComment[] => {
    if (!sessionIds || sessionIds.length === 0) return EMPTY_COMMENTS;
    const result: PlanComment[] = [];
    for (const id of sessionIds) {
      const comment = byId[id];
      if (comment && isPlanComment(comment)) {
        result.push(comment);
      }
    }
    return result.length === 0 ? EMPTY_COMMENTS : result;
  }, [byId, sessionIds]);

  const handleAddComment = useCallback(
    (commentText: string, selectedText: string) => {
      if (!sessionId) return;

      if (editingCommentId) {
        updateCommentInStore(editingCommentId, { text: commentText, selectedText } as Partial<PlanComment>);
        setEditingComment(null);
      } else {
        const comment: PlanComment = {
          id: crypto.randomUUID(),
          sessionId,
          source: 'plan',
          text: commentText,
          selectedText,
          createdAt: new Date().toISOString(),
          status: 'pending',
        };
        addCommentToStore(comment);
      }
    },
    [sessionId, editingCommentId, addCommentToStore, updateCommentInStore, setEditingComment]
  );

  const handleDeleteComment = useCallback(
    (commentId: string) => {
      removeCommentFromStore(commentId);
    },
    [removeCommentFromStore]
  );

  return {
    comments,
    editingCommentId,
    setEditingCommentId: setEditingComment,
    handleAddComment,
    handleDeleteComment,
  };
}
