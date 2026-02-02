'use client';

import { useCallback, useMemo } from 'react';
import { toast } from 'sonner';
import type { SelectedLineRange } from '@pierre/diffs';
import {
  useDiffCommentsStore,
  useFileComments,
} from '@/lib/state/slices/diff-comments';
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
    annotation: string
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
  const comments = useFileComments(sessionId, filePath);
  const editingCommentId = useDiffCommentsStore(
    (state) => state.editingCommentId
  );
  const storeAddComment = useDiffCommentsStore((state) => state.addComment);
  const storeRemoveComment = useDiffCommentsStore((state) => state.removeComment);
  const storeUpdateComment = useDiffCommentsStore((state) => state.updateComment);
  const storeSetEditingComment = useDiffCommentsStore((state) => state.setEditingComment);

  const annotations = useMemo(
    () => commentsToAnnotations(comments),
    [comments]
  );

  const addComment = useCallback(
    (range: SelectedLineRange, annotation: string) => {
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
        sessionId,
        filePath,
        startLine,
        endLine,
        side,
        codeContent,
        annotation,
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
      storeRemoveComment(sessionId, filePath, commentId);
    },
    [sessionId, filePath, storeRemoveComment]
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
