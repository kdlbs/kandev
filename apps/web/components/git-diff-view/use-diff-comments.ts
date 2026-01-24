import { useState, useCallback, useEffect } from 'react';
import type { DiffComment } from './types';
import { SplitSide } from '@git-diff-view/react';

const STORAGE_KEY_PREFIX = 'git-diff-comments';

function getStorageKey(filePath: string): string {
  return `${STORAGE_KEY_PREFIX}:${filePath}`;
}

function loadComments(filePath: string): DiffComment[] {
  if (typeof window === 'undefined') return [];
  try {
    const raw = window.localStorage.getItem(getStorageKey(filePath));
    if (!raw) return [];
    return JSON.parse(raw) as DiffComment[];
  } catch {
    return [];
  }
}

function saveComments(filePath: string, comments: DiffComment[]): void {
  if (typeof window === 'undefined') return;
  try {
    window.localStorage.setItem(getStorageKey(filePath), JSON.stringify(comments));
  } catch {
    // Ignore write failures
  }
}

export interface UseDiffCommentsOptions {
  /** File path for localStorage key */
  filePath: string;
  /** External comments (controlled mode) */
  externalComments?: DiffComment[];
  /** Callback when comment is added (controlled mode) */
  onCommentAdd?: (comment: DiffComment) => void;
  /** Callback when comment is deleted (controlled mode) */
  onCommentDelete?: (commentId: string) => void;
}

export interface UseDiffCommentsReturn {
  comments: DiffComment[];
  addComment: (startLine: number, endLine: number, side: SplitSide, content: string) => void;
  deleteComment: (commentId: string) => void;
}

export function useDiffComments({
  filePath,
  externalComments,
  onCommentAdd,
  onCommentDelete,
}: UseDiffCommentsOptions): UseDiffCommentsReturn {
  const isControlled = externalComments !== undefined;

  const [internalComments, setInternalComments] = useState<DiffComment[]>(() => {
    if (isControlled) return [];
    return loadComments(filePath);
  });

  // Persist internal comments to localStorage
  useEffect(() => {
    if (!isControlled) {
      saveComments(filePath, internalComments);
    }
  }, [filePath, internalComments, isControlled]);

  const comments = isControlled ? externalComments : internalComments;

  const addComment = useCallback(
    (startLine: number, endLine: number, side: SplitSide, content: string) => {
      const newComment: DiffComment = {
        id: `${filePath}-${Date.now()}`,
        startLine,
        endLine,
        side,
        content,
        createdAt: new Date().toISOString(),
      };

      if (isControlled && onCommentAdd) {
        onCommentAdd(newComment);
      } else {
        setInternalComments((prev) => [...prev, newComment]);
      }
    },
    [filePath, isControlled, onCommentAdd]
  );

  const deleteComment = useCallback(
    (commentId: string) => {
      if (isControlled && onCommentDelete) {
        onCommentDelete(commentId);
      } else {
        setInternalComments((prev) => prev.filter((c) => c.id !== commentId));
      }
    },
    [isControlled, onCommentDelete]
  );

  return { comments, addComment, deleteComment };
}
