'use client';

import { useState, useCallback, useMemo, memo, useSyncExternalStore, type ReactNode } from 'react';
import { useTheme } from 'next-themes';
import { FileDiff } from '@pierre/diffs/react';
import { parsePatchFiles, parseDiffFromFile } from '@pierre/diffs';
import type { FileDiffOptions, FileDiffMetadata, DiffLineAnnotation, RenderHeaderMetadataProps, AnnotationSide, SelectedLineRange } from '@pierre/diffs';
import { cn } from '@kandev/ui/lib/utils';
import { Button } from '@kandev/ui/button';
import { Tooltip, TooltipTrigger, TooltipContent } from '@kandev/ui/tooltip';
import { IconCopy, IconTextWrap, IconLayoutRows, IconLayoutColumns, IconExternalLink, IconPlus, IconArrowBackUp } from '@tabler/icons-react';
import type { FileDiffData, DiffComment } from '@/lib/diff/types';
import { useDiffComments } from './use-diff-comments';
import { CommentForm } from './comment-form';
import { CommentDisplay } from './comment-display';

// Local storage key for global diff view mode
const DIFF_VIEW_MODE_KEY = 'diff-view-mode';
const DEFAULT_VIEW_MODE = 'unified' as const;

// Custom event for syncing view mode across components
const VIEW_MODE_CHANGE_EVENT = 'diff-view-mode-change';

type ViewMode = 'split' | 'unified';

// Helper to get view mode from localStorage
function getStoredViewMode(): ViewMode {
  if (typeof window === 'undefined') return DEFAULT_VIEW_MODE;
  const stored = localStorage.getItem(DIFF_VIEW_MODE_KEY);
  return stored === 'split' || stored === 'unified' ? stored : DEFAULT_VIEW_MODE;
}

// Helper to set view mode in localStorage and dispatch event
function setStoredViewMode(mode: ViewMode): void {
  localStorage.setItem(DIFF_VIEW_MODE_KEY, mode);
  window.dispatchEvent(new CustomEvent(VIEW_MODE_CHANGE_EVENT, { detail: mode }));
}

// Hook to subscribe to global view mode changes
function useGlobalViewMode(): [ViewMode, (mode: ViewMode) => void] {
  const subscribe = useCallback((callback: () => void) => {
    window.addEventListener(VIEW_MODE_CHANGE_EVENT, callback);
    window.addEventListener('storage', callback);
    return () => {
      window.removeEventListener(VIEW_MODE_CHANGE_EVENT, callback);
      window.removeEventListener('storage', callback);
    };
  }, []);

  const getSnapshot = useCallback(() => getStoredViewMode(), []);
  const getServerSnapshot = useCallback(() => DEFAULT_VIEW_MODE, []);

  const viewMode = useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot);

  return [viewMode, setStoredViewMode];
}

interface DiffViewerProps {
  /** Diff data to display */
  data: FileDiffData;
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
  /** Whether to show in compact mode (smaller text, no header) */
  compact?: boolean;
  /** Whether to hide the file header and toolbar */
  hideHeader?: boolean;
  /** Callback to open file in editor */
  onOpenFile?: (filePath: string) => void;
}

type AnnotationMetadata = {
  type: 'comment' | 'new-comment-form';
  comment?: DiffComment;
  isEditing?: boolean;
};

// Custom comparison for memo - only re-render if actual content changes
function arePropsEqual(prevProps: DiffViewerProps, nextProps: DiffViewerProps): boolean {
  // Compare data by content, not reference
  if (prevProps.data.filePath !== nextProps.data.filePath) return false;
  if (prevProps.data.diff !== nextProps.data.diff) return false;
  if (prevProps.data.oldContent !== nextProps.data.oldContent) return false;
  if (prevProps.data.newContent !== nextProps.data.newContent) return false;

  // Compare other props
  if (prevProps.enableComments !== nextProps.enableComments) return false;
  if (prevProps.sessionId !== nextProps.sessionId) return false;
  if (prevProps.compact !== nextProps.compact) return false;
  if (prevProps.hideHeader !== nextProps.hideHeader) return false;
  if (prevProps.className !== nextProps.className) return false;
  if (prevProps.onOpenFile !== nextProps.onOpenFile) return false;

  // Comments array - compare by length and IDs
  const prevComments = prevProps.comments;
  const nextComments = nextProps.comments;
  if (prevComments !== nextComments) {
    if (!prevComments || !nextComments) return false;
    if (prevComments.length !== nextComments.length) return false;
    for (let i = 0; i < prevComments.length; i++) {
      if (prevComments[i].id !== nextComments[i].id) return false;
      if (prevComments[i].annotation !== nextComments[i].annotation) return false;
    }
  }

  return true;
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
}: DiffViewerProps) {
  const { resolvedTheme } = useTheme();
  const [selectedLines, setSelectedLines] = useState<SelectedLineRange | null>(null);
  const [showCommentForm, setShowCommentForm] = useState(false);

  // Global view mode (synced via localStorage)
  const [globalViewMode, setGlobalViewMode] = useGlobalViewMode();

  // Local state for word wrap (per-diff)
  const [wordWrap, setWordWrap] = useState(false);

  // Use internal comment management if sessionId provided
  const {
    comments: internalComments,
    addComment,
    removeComment,
    updateComment,
    editingCommentId,
    setEditingComment,
  } = useDiffComments({
    sessionId: sessionId || '',
    filePath: data.filePath,
    diff: data.diff,
    newContent: data.newContent,
    oldContent: data.oldContent,
  });

  const comments = externalComments || internalComments;

  // Create annotations from comments + new comment form
  const annotations = useMemo<DiffLineAnnotation<AnnotationMetadata>[]>(() => {
    const result: DiffLineAnnotation<AnnotationMetadata>[] = comments.map((comment) => ({
      side: comment.side,
      lineNumber: comment.endLine,
      metadata: {
        type: 'comment' as const,
        comment,
        isEditing: editingCommentId === comment.id,
      },
    }));

    // Add new comment form annotation at the selected line
    if (showCommentForm && selectedLines) {
      result.push({
        side: (selectedLines.side || 'additions') as AnnotationSide,
        lineNumber: Math.max(selectedLines.start, selectedLines.end),
        metadata: {
          type: 'new-comment-form' as const,
        },
      });
    }

    return result;
  }, [comments, editingCommentId, showCommentForm, selectedLines]);

  // Stable handler for line selection - only fires when selection is complete
  const handleLineSelectionEnd = useCallback(
    (range: SelectedLineRange | null) => {
      setSelectedLines(range);
      if (range && enableComments) {
        setShowCommentForm(true);
      }
    },
    [enableComments]
  );

  // Handler for comment submission
  const handleCommentSubmit = useCallback(
    (content: string) => {
      if (!selectedLines) return;

      if (onCommentAdd && externalComments !== undefined) {
        const comment: DiffComment = {
          id: `${data.filePath}-${Date.now()}`,
          sessionId: sessionId || '',
          filePath: data.filePath,
          startLine: Math.min(selectedLines.start, selectedLines.end),
          endLine: Math.max(selectedLines.start, selectedLines.end),
          side: (selectedLines.side || 'additions') as DiffComment['side'],
          codeContent: '',
          annotation: content,
          createdAt: new Date().toISOString(),
          status: 'pending',
        };
        onCommentAdd(comment);
      } else if (sessionId) {
        addComment(selectedLines, content);
      }

      setShowCommentForm(false);
      setSelectedLines(null);
    },
    [selectedLines, sessionId, data.filePath, addComment, onCommentAdd, externalComments]
  );

  // Handler for comment deletion
  const handleCommentDelete = useCallback(
    (commentId: string) => {
      if (onCommentDelete && externalComments !== undefined) {
        onCommentDelete(commentId);
      } else {
        removeComment(commentId);
      }
    },
    [removeComment, onCommentDelete, externalComments]
  );

  // Handle comment update
  const handleCommentUpdate = useCallback(
    (commentId: string, content: string) => {
      updateComment(commentId, { annotation: content });
      setEditingComment(null);
    },
    [updateComment, setEditingComment]
  );

  // Render annotation callback
  const renderAnnotation = useCallback(
    (annotation: DiffLineAnnotation<AnnotationMetadata>): ReactNode => {
      const { type, comment, isEditing } = annotation.metadata;

      if (type === 'new-comment-form') {
        return (
          <div className="my-1 px-2">
            <CommentForm
              onSubmit={handleCommentSubmit}
              onCancel={() => {
                setShowCommentForm(false);
                setSelectedLines(null);
              }}
            />
          </div>
        );
      }

      if (isEditing && comment) {
        return (
          <div className="my-1 px-2">
            <CommentForm
              initialContent={comment.annotation}
              onSubmit={(content) => handleCommentUpdate(comment.id, content)}
              onCancel={() => setEditingComment(null)}
              isEditing
            />
          </div>
        );
      }

      if (comment) {
        return (
          <div className="my-1 px-2">
            <CommentDisplay
              comment={comment}
              onDelete={() => handleCommentDelete(comment.id)}
              onEdit={() => setEditingComment(comment.id)}
              showCode={false}
            />
          </div>
        );
      }

      return null;
    },
    [setEditingComment, handleCommentDelete, handleCommentUpdate, handleCommentSubmit]
  );

  // Render header metadata toolbar for each diff file
  const renderHeaderMetadata = useCallback(
    (props: RenderHeaderMetadataProps): ReactNode => {
      const filePath = props.fileDiff?.name || data.filePath;

      return (
        <div className="flex items-center gap-1">
          {/* Copy diff button */}
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="ghost"
                size="sm"
                className="h-6 w-6 p-0 cursor-pointer opacity-60 hover:opacity-100"
                onClick={() => navigator.clipboard.writeText(data.diff || '')}
              >
                <IconCopy className="h-3.5 w-3.5" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>Copy diff</TooltipContent>
          </Tooltip>

          {/* Revert button (non-functional placeholder) */}
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="ghost"
                size="sm"
                className="h-6 w-6 p-0 cursor-pointer opacity-60 hover:opacity-100"
                onClick={() => {
                  // TODO: Implement revert functionality
                }}
              >
                <IconArrowBackUp className="h-3.5 w-3.5" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>Revert changes</TooltipContent>
          </Tooltip>

          {/* Word wrap toggle */}
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="ghost"
                size="sm"
                className={cn(
                  'h-6 w-6 p-0 cursor-pointer opacity-60 hover:opacity-100',
                  wordWrap && 'opacity-100 bg-muted'
                )}
                onClick={() => setWordWrap(!wordWrap)}
              >
                <IconTextWrap className="h-3.5 w-3.5" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>Toggle word wrap</TooltipContent>
          </Tooltip>

          {/* Split/Unified toggle (global) */}
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="ghost"
                size="sm"
                className="h-6 w-6 p-0 cursor-pointer opacity-60 hover:opacity-100"
                onClick={() => setGlobalViewMode(globalViewMode === 'split' ? 'unified' : 'split')}
              >
                {globalViewMode === 'split' ? (
                  <IconLayoutRows className="h-3.5 w-3.5" />
                ) : (
                  <IconLayoutColumns className="h-3.5 w-3.5" />
                )}
              </Button>
            </TooltipTrigger>
            <TooltipContent>
              {globalViewMode === 'split' ? 'Switch to unified view' : 'Switch to split view'}
            </TooltipContent>
          </Tooltip>

          {/* Open in editor button */}
          {onOpenFile && (
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="ghost"
                  size="sm"
                  className="h-6 w-6 p-0 cursor-pointer opacity-60 hover:opacity-100"
                  onClick={() => onOpenFile(filePath)}
                >
                  <IconExternalLink className="h-3.5 w-3.5" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>Open in editor</TooltipContent>
            </Tooltip>
          )}
        </div>
      );
    },
    [data.filePath, data.diff, wordWrap, globalViewMode, setGlobalViewMode, onOpenFile]
  );

  // Render hover utility (+) icon for adding comments
  const renderHoverUtility = useCallback(
    (): ReactNode => {
      // Only show when comments are enabled
      if (!enableComments) return null;

      return (
        <div
          className="flex h-5 w-5 cursor-pointer items-center justify-center rounded border border-border bg-background text-muted-foreground hover:bg-accent hover:text-foreground"
          title="Add comment"
        >
          <IconPlus className="h-3 w-3" />
        </div>
      );
    },
    [enableComments]
  );

  // Determine if header/toolbar should be shown
  const showHeader = !hideHeader && !compact;

  // Stable options - only include values that truly need to change the diff render
  const options = useMemo<FileDiffOptions<AnnotationMetadata>>(
    () => ({
      diffStyle: globalViewMode,
      themeType: resolvedTheme === 'dark' ? 'dark' : 'light',
      theme: {
        dark: 'github-dark-high-contrast',
        light: 'github-light',
      },
      enableLineSelection: enableComments,
      hunkSeparators: 'simple',
      enableHoverUtility: enableComments,
      diffIndicators: 'none',
      onLineSelectionEnd: handleLineSelectionEnd,
      disableFileHeader: !showHeader,
      lineDiffType: 'word',
      overflow: wordWrap ? 'wrap' : 'scroll',
    }),
    [globalViewMode, resolvedTheme, enableComments, showHeader, handleLineSelectionEnd, wordWrap]
  );

  // Compute FileDiffMetadata from either diff string or content
  const fileDiffMetadata = useMemo<FileDiffMetadata | null>(() => {
    if (data.diff) {
      // Has diff string - parse it
      const parsed = parsePatchFiles(data.diff);
      return parsed[0]?.files[0] ?? null;
    } else if (data.oldContent || data.newContent) {
      // Has content - generate diff using library
      return parseDiffFromFile(
        { name: data.filePath, contents: data.oldContent },
        { name: data.filePath, contents: data.newContent }
      );
    }
    return null;
  }, [data.diff, data.oldContent, data.newContent, data.filePath]);

  // Only pass selectedLines when we have a completed selection (form shown)
  // During drag, let FileDiff manage its own internal state
  const controlledSelection = showCommentForm ? selectedLines : null;

  if (!fileDiffMetadata) {
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
    <div className={cn('diff-viewer relative', className)}>
      <FileDiff
        fileDiff={fileDiffMetadata}
        options={options}
        selectedLines={controlledSelection}
        lineAnnotations={annotations}
        renderAnnotation={renderAnnotation}
        renderHeaderMetadata={showHeader ? renderHeaderMetadata : undefined}
        renderHoverUtility={renderHoverUtility}
        className={cn(
          'rounded-md border border-border/50',
          compact && 'text-xs'
        )}
      />
    </div>
  );
}, arePropsEqual);

/**
 * Compact inline diff viewer for chat messages.
 * This is a convenience wrapper around DiffViewer with compact defaults.
 * @deprecated Use DiffViewer with compact={true} instead
 */
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
