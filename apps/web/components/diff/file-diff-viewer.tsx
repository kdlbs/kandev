'use client';

import { memo, useMemo } from 'react';
import { DiffViewer } from './diff-viewer';
import { transformGitDiff } from '@/lib/diff';
import type { DiffComment } from '@/lib/diff/types';

interface FileDiffViewerProps {
  /** File path */
  filePath: string;
  /** Raw diff content */
  diff: string;
  /** File status (M, A, D, etc.) */
  status?: string;
  /** Enable line selection for comments */
  enableComments?: boolean;
  /** Session ID for comment storage */
  sessionId?: string;
  /** Callback when comment is added */
  onCommentAdd?: (comment: DiffComment) => void;
  /** Callback when comment is deleted */
  onCommentDelete?: (commentId: string) => void;
  /** External comments (controlled mode) */
  comments?: DiffComment[];
  /** Additional class name */
  className?: string;
  /** Whether to show in compact mode */
  compact?: boolean;
  /** Whether to hide the file header */
  hideHeader?: boolean;
  /** Callback to open file in editor */
  onOpenFile?: (filePath: string) => void;
  /** External word wrap override */
  wordWrap?: boolean;
}

/**
 * Wrapper around DiffViewer that handles data transformation with memoization.
 *
 * Use this component when you have raw diff data (filePath, diff, status).
 * The transformation to FileDiffData is memoized internally, preventing
 * unnecessary re-renders when parent components update.
 *
 * This is the recommended component to use for rendering git diffs.
 */
export const FileDiffViewer = memo(function FileDiffViewer({
  filePath,
  diff,
  status = 'M',
  enableComments,
  sessionId,
  onCommentAdd,
  onCommentDelete,
  comments,
  className,
  compact,
  hideHeader,
  onOpenFile,
  wordWrap,
}: FileDiffViewerProps) {
  // Memoize the transformation - only recalculates when raw data changes
  const data = useMemo(
    () => transformGitDiff(filePath, diff, status),
    [filePath, diff, status]
  );

  return (
    <DiffViewer
      data={data}
      enableComments={enableComments}
      sessionId={sessionId}
      onCommentAdd={onCommentAdd}
      onCommentDelete={onCommentDelete}
      comments={comments}
      className={className}
      compact={compact}
      hideHeader={hideHeader}
      onOpenFile={onOpenFile}
      wordWrap={wordWrap}
    />
  );
});
