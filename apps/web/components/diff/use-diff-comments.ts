'use client';

import { useCallback, useMemo } from 'react';
import { toast } from 'sonner';
import type { SelectedLineRange } from '@pierre/diffs';
import { useCommentsStore } from '@/lib/state/slices/comments';
import { useDiffFileComments } from '@/hooks/domains/comments/use-diff-comments';
import {
  commentsToAnnotations,
  extractCodeFromDiff,
  extractCodeFromContent,
} from '@/lib/diff';
import type { DiffComment, AnnotationSide, CommentAnnotation } from '@/lib/diff/types';

interface UseDiffCommentsOptions {
  sessionId: string;
  filePath: string;
  /** Diff string for extracting code from line selection */
  diff?: string;
  /** New content for extracting code from line selection */
  newContent?: string;
  /** Old content for extracting code from line selection */
  oldContent?: string;
}

interface UseDiffCommentsReturn {
  /** Comments for this file */
  comments: DiffComment[];
  /** Annotations formatted for @pierre/diffs */
  annotations: CommentAnnotation[];
  /** Add a new comment */
  addComment: (
    range: SelectedLineRange,
    text: string
  ) => void;
  /** Remove a comment */
  removeComment: (commentId: string) => void;
  /** Update a comment */
  updateComment: (commentId: string, updates: Partial<DiffComment>) => void;
  /** Currently editing comment ID */
  editingCommentId: string | null;
  /** Set the editing comment ID */
  setEditingComment: (commentId: string | null) => void;
}

/**
 * Hook to manage comments for a specific file's diff
 */
export function useDiffComments({
  sessionId,
  filePath,
  diff,
  newContent,
  oldContent,
}: UseDiffCommentsOptions): UseDiffCommentsReturn {
  const comments = useDiffFileComments(sessionId, filePath);
  const editingCommentId = useCommentsStore(
    (state) => state.editingCommentId
  );
  const storeAddComment = useCommentsStore((state) => state.addComment);
  const storeRemoveComment = useCommentsStore((state) => state.removeComment);
  const storeUpdateComment = useCommentsStore((state) => state.updateComment);
  const storeSetEditingComment = useCommentsStore((state) => state.setEditingComment);

  const annotations = useMemo(
    () => commentsToAnnotations(comments),
    [comments]
  );

  const addComment = useCallback(
    (range: SelectedLineRange, text: string) => {
      const side = (range.side || 'additions') as AnnotationSide;
      const startLine = Math.min(range.start, range.end);
      const endLine = Math.max(range.start, range.end);

      // Extract the code content from the selected lines
      let codeContent = '';
      if (diff) {
        codeContent = extractCodeFromDiff(diff, startLine, endLine, side);
      } else {
        const content = side === 'additions' ? newContent : oldContent;
        if (content) {
          codeContent = extractCodeFromContent(content, startLine, endLine);
        }
      }

      const comment: DiffComment = {
        id: `${filePath}-${Date.now()}-${Math.random().toString(36).substr(2, 9)}`,
        source: 'diff',
        sessionId,
        filePath,
        startLine,
        endLine,
        side,
        codeContent,
        text,
        createdAt: new Date().toISOString(),
        status: 'pending',
      };

      storeAddComment(comment);
      toast.success('Comment added', {
        description: 'Your comment will be sent with your next message.',
        duration: 2000,
      });
    },
    [sessionId, filePath, diff, newContent, oldContent, storeAddComment]
  );

  const removeComment = useCallback(
    (commentId: string) => {
      storeRemoveComment(commentId);
    },
    [storeRemoveComment]
  );

  const updateComment = useCallback(
    (commentId: string, updates: Partial<DiffComment>) => {
      storeUpdateComment(commentId, updates);
    },
    [storeUpdateComment]
  );

  const setEditingComment = useCallback(
    (commentId: string | null) => {
      storeSetEditingComment(commentId);
    },
    [storeSetEditingComment]
  );

  return {
    comments,
    annotations,
    addComment,
    removeComment,
    updateComment,
    editingCommentId,
    setEditingComment,
  };
}
