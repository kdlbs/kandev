'use client';

import { useState, useRef, memo } from 'react';
import { FileDiff } from '@pierre/diffs/react';
import { cn } from '@kandev/ui/lib/utils';
import type { FileDiffData, DiffComment } from '@/lib/diff/types';
import { useHunkHover } from './use-hunk-hover';
import { useAnnotationRenderer } from './use-diff-annotation-renderer';
import { useDiffOptions } from './use-diff-options';
import { useDiffViewerState } from './use-diff-viewer-state';

export type RevertBlockInfo = {
  /** 1-based line number in the new file where additions start */
  addStart: number;
  /** Number of addition lines to remove (0 for pure deletions) */
  addCount: number;
  /** Original lines to restore (empty for pure additions) */
  oldLines: string[];
};

interface DiffViewerProps {
  data: FileDiffData;
  enableComments?: boolean;
  sessionId?: string;
  onCommentAdd?: (comment: DiffComment) => void;
  onCommentDelete?: (commentId: string) => void;
  comments?: DiffComment[];
  className?: string;
  compact?: boolean;
  hideHeader?: boolean;
  onOpenFile?: (filePath: string) => void;
  onRevert?: (filePath: string) => void;
  enableAcceptReject?: boolean;
  onRevertBlock?: (filePath: string, info: RevertBlockInfo) => Promise<void> | void;
  wordWrap?: boolean;
}

const SCALAR_PROP_KEYS: (keyof DiffViewerProps)[] = [
  'enableComments', 'sessionId', 'compact', 'hideHeader', 'className',
  'onOpenFile', 'onRevert', 'enableAcceptReject', 'onRevertBlock', 'wordWrap',
];

const DATA_KEYS: (keyof FileDiffData)[] = ['filePath', 'diff', 'oldContent', 'newContent'];

function areCommentsEqual(prev: DiffComment[] | undefined, next: DiffComment[] | undefined): boolean {
  if (prev === next) return true;
  if (!prev || !next || prev.length !== next.length) return false;
  return prev.every((c, i) => c.id === next[i].id && c.annotation === next[i].annotation);
}

function arePropsEqual(prevProps: DiffViewerProps, nextProps: DiffViewerProps): boolean {
  for (const key of DATA_KEYS) {
    if (prevProps.data[key] !== nextProps.data[key]) return false;
  }
  for (const key of SCALAR_PROP_KEYS) {
    if (prevProps[key] !== nextProps[key]) return false;
  }
  return areCommentsEqual(prevProps.comments, nextProps.comments);
}

export const DiffViewer = memo(function DiffViewer({
  data,
  enableComments = false,
  sessionId,
  onCommentAdd,
  onCommentDelete,
  comments: externalComments,
  className,
  compact = false,
  hideHeader = false,
  onOpenFile,
  onRevert,
  enableAcceptReject = false,
  onRevertBlock,
  wordWrap: wordWrapProp,
}: DiffViewerProps) {
  const [wordWrapLocal, setWordWrap] = useState(false);
  const wordWrap = wordWrapProp ?? wordWrapLocal;
  const wrapperRef = useRef<HTMLDivElement>(null);

  const state = useDiffViewerState({
    data, enableComments, enableAcceptReject, sessionId,
    onCommentAdd, onCommentDelete, externalComments, onRevertBlock,
  });

  const { onLineEnter, onLineLeave, onButtonEnter, onButtonLeave } = useHunkHover({
    wrapperRef,
    changeLineMapRef: state.changeLineMapRef,
    hideTimeoutRef: state.hideTimeoutRef,
  });

  const renderAnnotation = useAnnotationRenderer({
    handleRevertBlock: state.handleRevertBlock,
    onButtonEnter, onButtonLeave,
    handleCommentSubmit: state.handleCommentSubmit,
    handleCommentUpdate: state.handleCommentUpdate,
    handleCommentDelete: state.handleCommentDelete,
    setShowCommentForm: state.setShowCommentForm,
    setSelectedLines: state.setSelectedLines,
    setEditingComment: state.setEditingComment,
  });

  const showHeader = !hideHeader && !compact;

  const { options, renderHeaderMetadata, renderHoverUtility } = useDiffOptions({
    filePath: data.filePath, diff: data.diff, enableComments, showHeader, wordWrap,
    setWordWrap, handleLineSelectionEnd: state.handleLineSelectionEnd,
    onLineEnter, onLineLeave, onOpenFile, onRevert,
  });

  const controlledSelection = state.showCommentForm ? state.selectedLines : null;

  if (!state.fileDiffMetadata) {
    return (
      <div className={cn(
        'rounded-md border border-border/50 bg-muted/20 p-4 text-muted-foreground',
        compact ? 'text-xs' : 'text-sm',
        className
      )}>
        No diff available
      </div>
    );
  }

  return (
    <div ref={wrapperRef} className={cn('diff-viewer', className)}>
      <FileDiff
        fileDiff={state.fileDiffMetadata}
        options={options}
        selectedLines={controlledSelection}
        lineAnnotations={state.annotations}
        renderAnnotation={renderAnnotation}
        renderHeaderMetadata={renderHeaderMetadata}
        renderHoverUtility={renderHoverUtility}
        className={cn(
          'rounded-md border border-border/50',
          compact && 'text-xs'
        )}
      />
    </div>
  );
}, arePropsEqual);

/** Compact inline diff viewer for chat messages (Pierre implementation). */
export function DiffViewInline({
  data,
  className,
}: {
  data: FileDiffData;
  className?: string;
}) {
  return (
    <DiffViewer
      data={data}
      compact
      hideHeader
      className={className}
    />
  );
}
