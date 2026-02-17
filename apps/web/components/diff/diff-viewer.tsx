'use client';

import { useState, useCallback, useEffect, useMemo, useRef, memo, type ReactNode } from 'react';
import { useTheme } from 'next-themes';
import { FileDiff } from '@pierre/diffs/react';
import { parsePatchFiles, parseDiffFromFile } from '@pierre/diffs';
import type { FileDiffOptions, FileDiffMetadata, DiffLineAnnotation, AnnotationSide, SelectedLineRange } from '@pierre/diffs';
import { cn } from '@kandev/ui/lib/utils';
import { IconPlus } from '@tabler/icons-react';
import type { FileDiffData, DiffComment } from '@/lib/diff/types';
import { buildDiffComment, useCommentActions } from '@/lib/diff/comment-utils';
import { FONT } from '@/lib/theme/colors';
import { useGlobalViewMode } from '@/hooks/use-global-view-mode';
import { useDiffComments } from './use-diff-comments';
import { CommentForm } from './comment-form';
import { CommentDisplay } from './comment-display';
import { useDiffHeaderToolbar } from './diff-header-toolbar';
import { HunkActionBar } from './hunk-action-bar';

/**
 * Check if Go code contains patterns that trigger catastrophic regex backtracking
 * in shiki's JavaScript regex engine.
 */
function hasProblematicGoPattern(content: string | undefined): boolean {
  if (!content) return false;
  const problematicPattern = /interface\{\}\s*`[^`]*`/;
  return problematicPattern.test(content);
}

function isGoFile(filePath: string): boolean {
  return filePath.endsWith('.go');
}

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

type AnnotationMetadata = {
  type: 'comment' | 'new-comment-form' | 'hunk-actions';
  comment?: DiffComment;
  isEditing?: boolean;
  changeBlockId?: string;
};

function arePropsEqual(prevProps: DiffViewerProps, nextProps: DiffViewerProps): boolean {
  if (prevProps.data.filePath !== nextProps.data.filePath) return false;
  if (prevProps.data.diff !== nextProps.data.diff) return false;
  if (prevProps.data.oldContent !== nextProps.data.oldContent) return false;
  if (prevProps.data.newContent !== nextProps.data.newContent) return false;
  if (prevProps.enableComments !== nextProps.enableComments) return false;
  if (prevProps.sessionId !== nextProps.sessionId) return false;
  if (prevProps.compact !== nextProps.compact) return false;
  if (prevProps.hideHeader !== nextProps.hideHeader) return false;
  if (prevProps.className !== nextProps.className) return false;
  if (prevProps.onOpenFile !== nextProps.onOpenFile) return false;
  if (prevProps.onRevert !== nextProps.onRevert) return false;
  if (prevProps.enableAcceptReject !== nextProps.enableAcceptReject) return false;
  if (prevProps.onRevertBlock !== nextProps.onRevertBlock) return false;
  if (prevProps.wordWrap !== nextProps.wordWrap) return false;

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
  onRevert,
  enableAcceptReject = false,
  onRevertBlock,
  wordWrap: wordWrapProp,
}: DiffViewerProps) {
  const { resolvedTheme } = useTheme();
  const [selectedLines, setSelectedLines] = useState<SelectedLineRange | null>(null);
  const [showCommentForm, setShowCommentForm] = useState(false);

  const [globalViewMode, setGlobalViewMode] = useGlobalViewMode();

  const [wordWrapLocal, setWordWrap] = useState(false);
  const wordWrap = wordWrapProp ?? wordWrapLocal;

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

  // Parse initial diff metadata
  const initialDiffMetadata = useMemo<FileDiffMetadata | null>(() => {
    let result: FileDiffMetadata | null = null;
    if (data.diff) {
      const parsed = parsePatchFiles(data.diff);
      result = parsed[0]?.files[0] ?? null;
    } else if (data.oldContent || data.newContent) {
      result = parseDiffFromFile(
        { name: data.filePath, contents: data.oldContent },
        { name: data.filePath, contents: data.newContent }
      );
    }

    if (result && isGoFile(data.filePath)) {
      const contentToCheck = data.newContent || data.oldContent || data.diff;
      if (hasProblematicGoPattern(contentToCheck)) {
        result = { ...result, lang: 'text' };
      }
    }

    return result;
  }, [data.diff, data.oldContent, data.newContent, data.filePath]);

  const fileDiffMetadata = initialDiffMetadata;

  // Per-change-block revert info: changeBlockId → { addStart, addCount, oldLines }
  const revertInfoRef = useRef<Map<string, RevertBlockInfo>>(new Map());

  const handleRevertBlock = useCallback(
    async (changeBlockId: string) => {
      const info = revertInfoRef.current.get(changeBlockId);
      if (!info) return;
      await onRevertBlock?.(data.filePath, info);
    },
    [data.filePath, onRevertBlock]
  );

  // Map from "side:lineNumber" → changeBlockId for hover detection
  const changeLineMapRef = useRef<Map<string, string>>(new Map());
  const hideTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Annotations: comments, new comment form, and hunk accept/reject
  const { annotations, lineMap, revertMap } = useMemo(() => {
    const result: DiffLineAnnotation<AnnotationMetadata>[] = comments.map((comment) => ({
      side: comment.side,
      lineNumber: comment.endLine,
      metadata: {
        type: 'comment' as const,
        comment,
        isEditing: editingCommentId === comment.id,
      },
    }));

    if (showCommentForm && selectedLines) {
      result.push({
        side: (selectedLines.side || 'additions') as AnnotationSide,
        lineNumber: Math.max(selectedLines.start, selectedLines.end),
        metadata: { type: 'new-comment-form' as const },
      });
    }

    // Add undo annotations per change block within each hunk.
    // Also build a line → changeBlockId mapping for hover detection,
    // and a changeBlockId → RevertBlockInfo mapping for workspace revert.
    const newLineMap = new Map<string, string>();
    const newRevertMap = new Map<string, RevertBlockInfo>();
    if (enableAcceptReject && fileDiffMetadata) {
      let blockIdx = 0;
      for (let hi = 0; hi < fileDiffMetadata.hunks.length; hi++) {
        const hunk = fileDiffMetadata.hunks[hi];
        if (hunk.additionCount === 0 && hunk.deletionCount === 0) continue;
        let addLine = hunk.additionStart;
        let delLine = hunk.deletionStart;
        let lastCtxAdd = addLine > 1 ? addLine - 1 : addLine;
        let lastCtxDel = delLine > 1 ? delLine - 1 : delLine;
        for (const content of hunk.hunkContent) {
          if (content.type === 'context') {
            const len = content.lines.length;
            lastCtxAdd = addLine + len - 1;
            lastCtxDel = delLine + len - 1;
            addLine += len;
            delLine += len;
          } else {
            const aLen = content.additions.length;
            const dLen = content.deletions.length;
            if (aLen > 0 || dLen > 0) {
              const cbId = `cb-${blockIdx++}`;
              const side: AnnotationSide = aLen > 0 ? 'additions' : 'deletions';
              const lineNumber = side === 'additions' ? lastCtxAdd : lastCtxDel;
              result.push({
                side,
                lineNumber,
                metadata: { type: 'hunk-actions', changeBlockId: cbId },
              });
              for (let l = 0; l < aLen; l++) newLineMap.set(`additions:${addLine + l}`, cbId);
              for (let l = 0; l < dLen; l++) newLineMap.set(`deletions:${delLine + l}`, cbId);
              newRevertMap.set(cbId, {
                addStart: addLine,
                addCount: aLen,
                // Pierre stores lines with trailing \n — strip them since
                // we splice into an array produced by split('\n')
                oldLines: content.deletions.map(l => l.replace(/\r?\n$/, '')),
              });
            }
            addLine += aLen;
            delLine += dLen;
          }
        }
      }
    }

    return { annotations: result, lineMap: newLineMap, revertMap: newRevertMap };
  }, [comments, editingCommentId, showCommentForm, selectedLines, enableAcceptReject, fileDiffMetadata]);

  // Sync maps to refs outside of render (avoids react-hooks/refs violation)
  useEffect(() => {
    changeLineMapRef.current = lineMap;
    revertInfoRef.current = revertMap;
  }, [lineMap, revertMap]);

  const handleLineSelectionEnd = useCallback(
    (range: SelectedLineRange | null) => {
      setSelectedLines(range);
      if (range && enableComments) {
        setShowCommentForm(true);
      }
    },
    [enableComments]
  );

  const handleCommentSubmit = useCallback(
    (content: string) => {
      if (!selectedLines) return;
      if (onCommentAdd && externalComments !== undefined) {
        onCommentAdd(buildDiffComment({
          filePath: data.filePath,
          sessionId: sessionId || '',
          startLine: selectedLines.start,
          endLine: selectedLines.end,
          side: (selectedLines.side || 'additions') as DiffComment['side'],
          annotation: content,
        }));
      } else if (sessionId) {
        addComment(selectedLines, content);
      }
      setShowCommentForm(false);
      setSelectedLines(null);
    },
    [selectedLines, sessionId, data.filePath, addComment, onCommentAdd, externalComments]
  );

  const { handleCommentDelete, handleCommentUpdate } = useCommentActions({
    removeComment, updateComment, setEditingComment,
    onCommentDelete, externalComments,
  });

  // Show/hide undo buttons when hovering change lines (direct DOM, no re-renders)
  const wrapperRef = useRef<HTMLDivElement>(null);
  const activeBlockRef = useRef<string | null>(null);
  const isHoveringButtonRef = useRef(false);

  const setBlockVisible = useCallback((cbId: string | null, visible: boolean) => {
    if (!cbId) return;
    const btn = wrapperRef.current?.querySelector(`[data-cb="${cbId}"] [data-undo-btn]`);
    if (btn instanceof HTMLElement) {
      btn.style.opacity = visible ? '1' : '0';
      btn.style.pointerEvents = visible ? 'auto' : 'none';
    }
  }, []);

  const showBlock = useCallback((cbId: string) => {
    if (hideTimeoutRef.current) { clearTimeout(hideTimeoutRef.current); hideTimeoutRef.current = null; }
    if (activeBlockRef.current === cbId) return;
    setBlockVisible(activeBlockRef.current, false);
    activeBlockRef.current = cbId;
    setBlockVisible(cbId, true);
  }, [setBlockVisible]);

  const hideBlock = useCallback(() => {
    if (hideTimeoutRef.current) clearTimeout(hideTimeoutRef.current);
    hideTimeoutRef.current = setTimeout(() => {
      if (isHoveringButtonRef.current) return; // Don't hide while mouse is on the button
      setBlockVisible(activeBlockRef.current, false);
      activeBlockRef.current = null;
    }, 200);
  }, [setBlockVisible]);

  const onButtonEnter = useCallback(() => {
    isHoveringButtonRef.current = true;
    if (hideTimeoutRef.current) { clearTimeout(hideTimeoutRef.current); hideTimeoutRef.current = null; }
  }, []);

  const onButtonLeave = useCallback(() => {
    isHoveringButtonRef.current = false;
    hideBlock();
  }, [hideBlock]);

  const showBlockRef = useRef(showBlock);
  const hideBlockRef = useRef(hideBlock);
  useEffect(() => { showBlockRef.current = showBlock; }, [showBlock]);
  useEffect(() => { hideBlockRef.current = hideBlock; }, [hideBlock]);

  const onLineEnter = useCallback((props: { lineType?: string; lineNumber?: number; annotationSide?: string }) => {
    const { lineType, lineNumber, annotationSide } = props;
    if (!lineType?.startsWith('change-') || lineNumber == null) { hideBlockRef.current(); return; }
    const side = lineType === 'change-deletion' ? 'deletions' : 'additions';
    const key = `${annotationSide ?? side}:${lineNumber}`;
    const cbId = changeLineMapRef.current.get(key);
    if (cbId) showBlockRef.current(cbId);
    else hideBlockRef.current();
  }, []);

  const onLineLeave = useCallback(() => { hideBlockRef.current(); }, []);

  const renderAnnotation = useCallback(
    (annotation: DiffLineAnnotation<AnnotationMetadata>): ReactNode => {
      const { type, comment, isEditing, changeBlockId } = annotation.metadata;

      if (type === 'hunk-actions' && changeBlockId) {
        return (
          <HunkActionBar
            key={changeBlockId}
            changeBlockId={changeBlockId}
            onRevert={() => handleRevertBlock(changeBlockId)}
            onMouseEnter={onButtonEnter}
            onMouseLeave={onButtonLeave}
          />
        );
      }

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
    [setEditingComment, handleCommentDelete, handleCommentUpdate, handleCommentSubmit, handleRevertBlock, onButtonEnter, onButtonLeave]
  );

  const toggleViewMode = useCallback(
    () => setGlobalViewMode(globalViewMode === 'split' ? 'unified' : 'split'),
    [globalViewMode, setGlobalViewMode]
  );

  const toggleWordWrap = useCallback(
    () => setWordWrap((v) => !v),
    []
  );

  const renderHeaderMetadata = useDiffHeaderToolbar({
    filePath: data.filePath,
    diff: data.diff,
    wordWrap,
    onToggleWordWrap: toggleWordWrap,
    viewMode: globalViewMode,
    onToggleViewMode: toggleViewMode,
    onOpenFile,
    onRevert,
  });

  const renderHoverUtility = useCallback(
    (): ReactNode => {
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

  const showHeader = !hideHeader && !compact;

  const options = useMemo<FileDiffOptions<AnnotationMetadata>>(
    () => ({
      diffStyle: globalViewMode,
      themeType: resolvedTheme === 'dark' ? 'dark' : 'light',
      enableLineSelection: enableComments,
      hunkSeparators: 'simple',
      enableHoverUtility: enableComments,
      diffIndicators: 'none',
      onLineSelectionEnd: handleLineSelectionEnd,
      onLineEnter,
      onLineLeave,
      disableFileHeader: !showHeader,
      overflow: wordWrap ? 'wrap' : 'scroll',
      unsafeCSS: `
        /* Override Pierre's inline theme — both CSS vars AND direct background-color */
        pre[data-diffs] {
          background-color: var(--background) !important;
          --diffs-bg: var(--background) !important;
          --diffs-bg-context: var(--background) !important;
          --diffs-bg-buffer: var(--background) !important;
          --diffs-bg-separator: var(--card) !important;
          --diffs-bg-hover: var(--muted) !important;
          --diffs-fg: var(--foreground) !important;
          --diffs-fg-number: var(--muted-foreground) !important;
          --diffs-addition-color-override: rgb(var(--git-addition)) !important;
          --diffs-deletion-color-override: rgb(var(--git-deletion)) !important;
          --diffs-font-size: ${FONT.size}px !important;
          --diffs-font-family: ${FONT.mono} !important;
        }
        [data-change-icon] {
          width: 12px !important;
          height: 12px !important;
        }
        [data-diffs-header] {
          padding-inline: 12px !important;
          background: var(--card) !important;
        }
      `,
    }),
    [globalViewMode, resolvedTheme, enableComments, showHeader, handleLineSelectionEnd, wordWrap, onLineEnter, onLineLeave]
  );

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
    <div ref={wrapperRef} className={cn('diff-viewer', className)}>
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
