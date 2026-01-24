'use client';

import { useState, useRef, useCallback, useMemo, useEffect } from 'react';
import { DiffView, DiffModeEnum, SplitSide } from '@git-diff-view/react';
import { generateDiffFile } from '@git-diff-view/file';
import { useTheme } from 'next-themes';
import { Button } from '@kandev/ui/button';
import { Badge } from '@kandev/ui/badge';
import { IconLayoutColumns, IconLayoutRows } from '@tabler/icons-react';
import { cn } from '@/lib/utils';

import { useDiffComments } from './use-diff-comments';
import { InlineCommentWidget } from './comment-widget';
import { CommentRenderer } from './comment-renderer';
import { DragSelectionOverlay } from './selection-overlay';
import { CommentRangeIndicator } from './comment-indicator';
import { PositionedCommentWidget } from './multi-line-comment';
import { getLineInfoFromElement } from './dom-utils';
import type { GitDiffViewerProps, ExtendLineData, DragSelectionState } from './types';

import '@git-diff-view/react/styles/diff-view.css';
import './git-diff-viewer.css';

export function GitDiffViewer({
  oldContent,
  newContent,
  filePath,
  language = 'typescript',
  defaultViewMode = 'split',
  showToolbar = true,
  enableComments = true,
  comments: externalComments,
  onCommentAdd,
  onCommentDelete,
  className,
}: GitDiffViewerProps) {
  const { resolvedTheme } = useTheme();
  const [viewMode, setViewMode] = useState<'split' | 'unified'>(defaultViewMode);
  const [widgetStartLines, setWidgetStartLines] = useState<Record<string, number>>({});
  const widgetStartLinesRef = useRef(widgetStartLines);

  useEffect(() => {
    widgetStartLinesRef.current = widgetStartLines;
  }, [widgetStartLines]);

  // Drag selection state for multi-line comments
  const [dragSelection, setDragSelection] = useState<DragSelectionState | null>(null);
  const [showDragWidget, setShowDragWidget] = useState(false);
  const justCompletedMultiLineRef = useRef(false);

  const wrapperRef = useRef<HTMLDivElement>(null);

  // Comment management
  const { comments, addComment, deleteComment } = useDiffComments({
    filePath,
    externalComments,
    onCommentAdd,
    onCommentDelete,
  });

  const diffModeEnum = viewMode === 'split' ? DiffModeEnum.Split : DiffModeEnum.Unified;
  const diffTheme = resolvedTheme === 'dark' ? 'dark' : 'light';

  // Generate diff file
  const diffFile = useMemo(() => {
    try {
      const file = generateDiffFile(filePath, oldContent, filePath, newContent, language, language);
      file.initRaw();
      return file;
    } catch (e) {
      console.error('Failed to generate diff:', e);
      return null;
    }
  }, [filePath, oldContent, newContent, language]);

  // Build extendData for displaying existing comments
  const extendData = useMemo(() => {
    const oldFileData: Record<string, { data: ExtendLineData }> = {};
    const newFileData: Record<string, { data: ExtendLineData }> = {};

    comments.forEach((comment) => {
      const lineKey = String(comment.endLine);
      const entry: ExtendLineData = { type: 'comment', comment };

      if (comment.side === SplitSide.old) {
        oldFileData[lineKey] = { data: entry };
      } else {
        newFileData[lineKey] = { data: entry };
      }
    });

    return { oldFile: oldFileData, newFile: newFileData };
  }, [comments]);

  // Mouse handlers for drag selection
  const handleMouseDown = useCallback(
    (e: React.MouseEvent) => {
      if (!enableComments || e.button !== 0) return;

      const target = e.target as HTMLElement;
      const lineInfo = getLineInfoFromElement(target, wrapperRef.current, viewMode);

      if (lineInfo) {
        setDragSelection({
          startLine: lineInfo.lineNumber,
          endLine: lineInfo.lineNumber,
          side: lineInfo.side,
          isActive: true,
        });
        setShowDragWidget(false);
        justCompletedMultiLineRef.current = false;
      }
    },
    [enableComments, viewMode]
  );

  const handleMouseMove = useCallback(
    (e: React.MouseEvent) => {
      if (!dragSelection?.isActive) return;

      const target = e.target as HTMLElement;
      const lineInfo = getLineInfoFromElement(target, wrapperRef.current, viewMode);

      if (lineInfo && lineInfo.side === dragSelection.side && lineInfo.lineNumber !== dragSelection.endLine) {
        setDragSelection((prev) => (prev ? { ...prev, endLine: lineInfo.lineNumber } : null));
      }
    },
    [dragSelection, viewMode]
  );

  const handleMouseUp = useCallback(() => {
    if (!dragSelection?.isActive) return;

    const { startLine, endLine, side } = dragSelection;
    const rangeStart = Math.min(startLine, endLine);
    const rangeEnd = Math.max(startLine, endLine);

    if (rangeStart !== rangeEnd) {
      setDragSelection({ startLine: rangeStart, endLine: rangeEnd, side, isActive: false });
      setShowDragWidget(true);
      justCompletedMultiLineRef.current = true;
    } else {
      setDragSelection(null);
    }
  }, [dragSelection]);

  const handleMouseLeave = useCallback(() => {
    if (dragSelection?.isActive) {
      setDragSelection(null);
    }
  }, [dragSelection]);

  // Prevent library's single-line widget after multi-line selection
  const handleClick = useCallback((e: React.MouseEvent) => {
    if (justCompletedMultiLineRef.current) {
      e.stopPropagation();
      e.preventDefault();
      justCompletedMultiLineRef.current = false;
    }
  }, []);

  // Handle widget line rendering for single-line comments
  const renderWidgetLine = useCallback(
    (props: { side: SplitSide; lineNumber: number; onClose: () => void }) => {
      if (!diffFile || !enableComments) return null;

      const widgetKey = `${filePath}-${props.side}-${props.lineNumber}`;
      const startLine = widgetStartLinesRef.current[widgetKey] ?? props.lineNumber;

      return (
        <InlineCommentWidget
          key={widgetKey}
          lineNumber={props.lineNumber}
          side={props.side}
          startLine={startLine}
          diffFile={diffFile}
          wrapperRef={wrapperRef}
          viewMode={viewMode}
          filePath={filePath}
          onSave={(comment) => {
            addComment(comment.startLine, comment.endLine, comment.side, comment.content);
            setWidgetStartLines((prev) => {
              const next = { ...prev };
              delete next[widgetKey];
              return next;
            });
            props.onClose();
          }}
          onCancel={() => {
            setWidgetStartLines((prev) => {
              const next = { ...prev };
              delete next[widgetKey];
              return next;
            });
            props.onClose();
          }}
          onStartLineChange={(newStartLine) => {
            setWidgetStartLines((prev) => ({ ...prev, [widgetKey]: newStartLine }));
          }}
        />
      );
    },
    [filePath, diffFile, viewMode, enableComments, addComment]
  );

  // Render existing comments inline
  const renderExtendLine = useCallback(
    (lineData: { data: ExtendLineData }) => {
      if (!lineData.data) return null;
      return <CommentRenderer comment={lineData.data.comment} onDelete={(id) => deleteComment(id)} />;
    },
    [deleteComment]
  );

  if (!diffFile) {
    return (
      <div className={cn('flex items-center justify-center p-6', className)}>
        <div className="text-muted-foreground">Failed to generate diff</div>
      </div>
    );
  }

  return (
    <div className={cn('space-y-4', className)}>
      {showToolbar && (
        <div className="flex items-center justify-between">
          <Badge variant="secondary" className="rounded-full text-xs">
            {filePath}
          </Badge>
          <div className="inline-flex rounded-md border border-border overflow-hidden">
            <Button
              type="button"
              variant="ghost"
              size="sm"
              className={cn('h-7 px-2 text-xs rounded-none cursor-pointer', viewMode === 'unified' && 'bg-muted')}
              onClick={() => setViewMode('unified')}
            >
              <IconLayoutRows className="h-3.5 w-3.5" />
            </Button>
            <Button
              type="button"
              variant="ghost"
              size="sm"
              className={cn('h-7 px-2 text-xs rounded-none cursor-pointer', viewMode === 'split' && 'bg-muted')}
              onClick={() => setViewMode('split')}
            >
              <IconLayoutColumns className="h-3.5 w-3.5" />
            </Button>
          </div>
        </div>
      )}

      {/* Outer container for positioning widget outside overflow */}
      <div className="relative">
        <div
          ref={wrapperRef}
          className="relative border border-border rounded-lg overflow-x-auto select-none"
          onMouseDown={handleMouseDown}
          onMouseMove={handleMouseMove}
          onMouseUp={handleMouseUp}
          onMouseLeave={handleMouseLeave}
          onClickCapture={handleClick}
        >
          {/* Comment range indicators */}
          {enableComments &&
            comments.map((comment) => (
              <CommentRangeIndicator key={comment.id} comment={comment} wrapperRef={wrapperRef} viewMode={viewMode} />
            ))}

          {/* Drag selection overlay */}
          {enableComments && dragSelection && diffFile && (
            <DragSelectionOverlay selection={dragSelection} wrapperRef={wrapperRef} viewMode={viewMode} />
          )}

          <DiffView
            diffFile={diffFile}
            diffViewWrap={false}
            diffViewTheme={diffTheme}
            diffViewHighlight
            diffViewMode={diffModeEnum}
            diffViewFontSize={12}
            diffViewAddWidget={enableComments}
            onAddWidgetClick={() => {}}
            renderWidgetLine={renderWidgetLine}
            extendData={extendData}
            renderExtendLine={renderExtendLine}
          />
        </div>

        {/* Multi-line comment widget - outside overflow container */}
        {enableComments && showDragWidget && dragSelection && !dragSelection.isActive && diffFile && (
          <PositionedCommentWidget
            selection={dragSelection}
            diffFile={diffFile}
            wrapperRef={wrapperRef}
            viewMode={viewMode}
            onSave={(content) => {
              addComment(dragSelection.startLine, dragSelection.endLine, dragSelection.side, content);
              setDragSelection(null);
              setShowDragWidget(false);
            }}
            onCancel={() => {
              setDragSelection(null);
              setShowDragWidget(false);
            }}
          />
        )}
      </div>
    </div>
  );
}
