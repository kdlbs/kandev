'use client';

import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState, useSyncExternalStore } from 'react';
import { DiffEditor, type DiffOnMount } from '@monaco-editor/react';
import type { editor as monacoEditor } from 'monaco-editor';
import { useTheme } from 'next-themes';
import { cn } from '@kandev/ui/lib/utils';
import type { FileDiffData, DiffComment } from '@/lib/diff/types';
import { buildDiffComment, useCommentedLines, useCommentActions } from '@/lib/diff/comment-utils';
import { getMonacoLanguage } from '@/lib/editor/language-map';
import { EDITOR_FONT_FAMILY, EDITOR_FONT_SIZE } from '@/lib/theme/editor-theme';
import { useDiffComments } from '@/components/diff/use-diff-comments';
import { useCommandPanelOpen } from '@/lib/commands/command-registry';
import { useGutterComments } from '@/hooks/use-gutter-comments';
import { useDiffEditorHeight } from '@/hooks/use-diff-editor-height';
import { useGlobalViewMode } from '@/hooks/use-global-view-mode';
import { DiffViewerToolbar } from './diff-viewer-toolbar';
import { DiffViewerContextMenu, type ContextMenuState } from './diff-viewer-context-menu';
import { useViewZones } from './use-diff-view-zones';
import { initMonacoThemes } from './monaco-init';

initMonacoThemes();

// Global folding toggle sync
const DIFF_FOLD_KEY = 'diff-fold-unchanged';
const DEFAULT_FOLD = true;
const FOLD_CHANGE_EVENT = 'diff-fold-change';

function getStoredFolding(): boolean {
  if (typeof window === 'undefined') return DEFAULT_FOLD;
  const stored = localStorage.getItem(DIFF_FOLD_KEY);
  if (stored === null) return DEFAULT_FOLD;
  return stored === 'true';
}

function setStoredFolding(fold: boolean): void {
  localStorage.setItem(DIFF_FOLD_KEY, String(fold));
  window.dispatchEvent(new CustomEvent(FOLD_CHANGE_EVENT, { detail: fold }));
}

function useGlobalFolding(): [boolean, (fold: boolean) => void] {
  const subscribe = useCallback((callback: () => void) => {
    window.addEventListener(FOLD_CHANGE_EVENT, callback);
    window.addEventListener('storage', callback);
    return () => {
      window.removeEventListener(FOLD_CHANGE_EVENT, callback);
      window.removeEventListener('storage', callback);
    };
  }, []);
  const getSnapshot = useCallback(() => getStoredFolding(), []);
  const getServerSnapshot = useCallback(() => DEFAULT_FOLD, []);
  const foldUnchanged = useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot);
  return [foldUnchanged, setStoredFolding];
}

interface MonacoDiffViewerProps {
  data: FileDiffData;
  sessionId?: string;
  onCommentAdd?: (comment: DiffComment) => void;
  onCommentDelete?: (commentId: string) => void;
  comments?: DiffComment[];
  className?: string;
  compact?: boolean;
  hideHeader?: boolean;
  onOpenFile?: (filePath: string) => void;
  onRevert?: (filePath: string) => void;
  wordWrap?: boolean;
  editable?: boolean;
  onModifiedContentChange?: (filePath: string, content: string) => void;
}

export function MonacoDiffViewer({
  data,
  sessionId,
  onCommentAdd,
  onCommentDelete,
  comments: externalComments,
  className,
  compact = false,
  hideHeader = false,
  onOpenFile,
  onRevert,
  wordWrap: wordWrapProp,
  editable,
  onModifiedContentChange,
}: MonacoDiffViewerProps) {
  const { resolvedTheme } = useTheme();
  const [globalViewMode, setGlobalViewMode] = useGlobalViewMode();
  const [foldUnchanged, setFoldUnchanged] = useGlobalFolding();
  const [wordWrapLocal, setWordWrap] = useState(false);
  const wordWrap = wordWrapProp ?? wordWrapLocal;
  const diffEditorRef = useRef<monacoEditor.IStandaloneDiffEditor | null>(null);
  const wrapperRef = useRef<HTMLDivElement>(null);
  const { setOpen: setCommandPanelOpen } = useCommandPanelOpen();
  const [modifiedEditor, setModifiedEditor] = useState<monacoEditor.ICodeEditor | null>(null);
  const [originalEditor, setOriginalEditor] = useState<monacoEditor.ICodeEditor | null>(null);

  // Cmd+K for command panel — capture phase prevents Monaco from swallowing the shortcut
  useEffect(() => {
    const wrapper = wrapperRef.current;
    if (!wrapper) return;
    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
        e.preventDefault();
        e.stopPropagation();
        setCommandPanelOpen(true);
      }
    };
    wrapper.addEventListener('keydown', handleKeyDown, true);
    return () => wrapper.removeEventListener('keydown', handleKeyDown, true);
  }, [setCommandPanelOpen]);

  // Fix: reset the DiffEditor model BEFORE @monaco-editor/react's useEffect cleanup
  useLayoutEffect(() => {
    return () => {
      try { diffEditorRef.current?.setModel(null); } catch { /* already disposed */ }
    };
  }, []);

  // Context menu state
  const [contextMenu, setContextMenu] = useState<ContextMenuState>(null);

  // Comment form state
  const [showCommentForm, setShowCommentForm] = useState(false);
  const [selectedLineRange, setSelectedLineRange] = useState<{
    start: number; end: number; side: string;
  } | null>(null);

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

  // Gutter comment interactions for both sides of the diff
  const gutterEnabled = !!sessionId && !compact;
  const commentedLines = useCommentedLines(comments);

  const handleGutterSelect = useCallback(
    (side: 'additions' | 'deletions') =>
      (params: { range: { start: number; end: number }; code: string; position: { x: number; y: number } }) => {
        setSelectedLineRange({ start: params.range.start, end: params.range.end, side });
        setShowCommentForm(true);
      },
    []
  );

  const { clearGutterSelection: clearModifiedGutter } = useGutterComments(modifiedEditor, {
    enabled: gutterEnabled,
    commentedLines,
    onSelectionComplete: handleGutterSelect('additions'),
  });

  const { clearGutterSelection: clearOriginalGutter } = useGutterComments(originalEditor, {
    enabled: gutterEnabled,
    commentedLines,
    onSelectionComplete: handleGutterSelect('deletions'),
  });

  const language = getMonacoLanguage(data.filePath);
  const showHeader = !hideHeader && !compact;

  // Parse original/modified content from diff data
  const { original, modified } = useMemo(() => {
    if (data.oldContent || data.newContent) {
      return { original: data.oldContent ?? '', modified: data.newContent ?? '' };
    }
    if (data.diff) {
      return parseDiffContent(data.diff);
    }
    return { original: '', modified: '' };
  }, [data.oldContent, data.newContent, data.diff]);

  // Auto-fit height
  const lineHeight = compact ? 16 : 18;
  const editorHeight = useDiffEditorHeight({
    modifiedEditor,
    originalEditor,
    compact,
    lineHeight,
    originalContent: original,
    modifiedContent: modified,
  });

  // Stable ref for onModifiedContentChange to avoid stale closures
  const onModifiedContentChangeRef = useRef(onModifiedContentChange);
  useEffect(() => { onModifiedContentChangeRef.current = onModifiedContentChange; }, [onModifiedContentChange]);

  const handleDiffEditorMount: DiffOnMount = useCallback((editor) => {
    diffEditorRef.current = editor;
    const modEditor = editor.getModifiedEditor();
    const origEditor = editor.getOriginalEditor();
    setModifiedEditor(modEditor);
    setOriginalEditor(origEditor);

    // Listen for content changes on modified side (edits + block reverts)
    if (!compact) {
      modEditor.onDidChangeModelContent(() => {
        onModifiedContentChangeRef.current?.(data.filePath, modEditor.getValue());
      });
    }

    // Context menu on both sides
    modEditor.onContextMenu(handleContextMenuEvent(editor, modEditor, 'modified'));
    origEditor.onContextMenu(handleContextMenuEvent(editor, origEditor, 'original'));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [compact, data.filePath]);

  const handleContextMenuEvent = useCallback(
    (
      diffEditor: monacoEditor.IStandaloneDiffEditor,
      targetEditor: monacoEditor.ICodeEditor,
      side: 'original' | 'modified',
    ) =>
      (e: monacoEditor.IEditorMouseEvent) => {
        if (!e.target.position) return;
        e.event.preventDefault();
        e.event.stopPropagation();
        const lineNumber = e.target.position.lineNumber;
        const model = targetEditor.getModel();
        const lineContent = model ? model.getLineContent(lineNumber) : '';
        const lineChanges = diffEditor.getLineChanges();
        let isChangedLine = false;
        if (lineChanges) {
          for (const change of lineChanges) {
            const [start, end] = side === 'modified'
              ? [change.modifiedStartLineNumber, change.modifiedEndLineNumber]
              : [change.originalStartLineNumber, change.originalEndLineNumber];
            if (lineNumber >= start && lineNumber <= end) {
              isChangedLine = true;
              break;
            }
          }
        }
        setContextMenu({ x: e.event.posx, y: e.event.posy, lineNumber, side, isChangedLine, lineContent });
      },
    []
  );

  // Handle comment submission
  const handleCommentSubmit = useCallback(
    (content: string) => {
      if (!selectedLineRange) return;
      if (onCommentAdd && externalComments !== undefined) {
        onCommentAdd(buildDiffComment({
          filePath: data.filePath,
          sessionId: sessionId || '',
          startLine: selectedLineRange.start,
          endLine: selectedLineRange.end,
          side: (selectedLineRange.side || 'additions') as DiffComment['side'],
          annotation: content,
        }));
      } else if (sessionId) {
        addComment(
          { start: selectedLineRange.start, end: selectedLineRange.end, side: selectedLineRange.side as 'additions' | 'deletions' },
          content
        );
      }
      setShowCommentForm(false);
      setSelectedLineRange(null);
      clearModifiedGutter();
      clearOriginalGutter();
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [selectedLineRange, sessionId, data.filePath, addComment, onCommentAdd, externalComments, clearModifiedGutter, clearOriginalGutter]
  );

  const { handleCommentDelete, handleCommentUpdate } = useCommentActions({
    removeComment, updateComment, setEditingComment,
    onCommentDelete, externalComments,
  });

  // Stable refs for callbacks used inside ViewZone renders
  const handleCommentSubmitRef = useRef(handleCommentSubmit);
  useEffect(() => { handleCommentSubmitRef.current = handleCommentSubmit; }, [handleCommentSubmit]);
  const handleCommentDeleteRef = useRef(handleCommentDelete);
  useEffect(() => { handleCommentDeleteRef.current = handleCommentDelete; }, [handleCommentDelete]);
  const handleCommentUpdateRef = useRef(handleCommentUpdate);
  useEffect(() => { handleCommentUpdateRef.current = handleCommentUpdate; }, [handleCommentUpdate]);

  useViewZones({
    modifiedEditor, originalEditor, comments, showCommentForm,
    selectedLineRange, editingCommentId, setEditingComment,
    handleCommentSubmitRef, handleCommentDeleteRef, handleCommentUpdateRef,
    clearModifiedGutter, clearOriginalGutter,
    setShowCommentForm, setSelectedLineRange,
  });

  // Close context menu on click or Escape
  useEffect(() => {
    if (!contextMenu) return;
    const handleClose = () => setContextMenu(null);
    const handleKeyDown = (e: KeyboardEvent) => { if (e.key === 'Escape') setContextMenu(null); };
    window.addEventListener('mousedown', handleClose);
    window.addEventListener('keydown', handleKeyDown);
    return () => {
      window.removeEventListener('mousedown', handleClose);
      window.removeEventListener('keydown', handleKeyDown);
    };
  }, [contextMenu]);

  // Copy all changed lines from the diff
  const copyAllChangedLines = useCallback(() => {
    const editor = diffEditorRef.current;
    if (!editor) return;
    const lineChanges = editor.getLineChanges();
    if (!lineChanges) return;
    const modifiedModel = editor.getModifiedEditor().getModel();
    if (!modifiedModel) return;
    const changedLines: string[] = [];
    for (const change of lineChanges) {
      if (change.modifiedStartLineNumber <= change.modifiedEndLineNumber) {
        for (let i = change.modifiedStartLineNumber; i <= change.modifiedEndLineNumber; i++) {
          changedLines.push(modifiedModel.getLineContent(i));
        }
      }
    }
    navigator.clipboard.writeText(changedLines.join('\n'));
    setContextMenu(null);
  }, []);

  if (!data.oldContent && !data.newContent && !data.diff) {
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

  // Modified side editable when prop is true or revert is available; compact always read-only
  const modifiedReadOnly = compact || !(editable || !!onRevert);

  return (
    <div ref={wrapperRef} className={cn('monaco-diff-viewer relative', className)}>
      {showHeader && (
        <DiffViewerToolbar
          data={data}
          foldUnchanged={foldUnchanged}
          setFoldUnchanged={setFoldUnchanged}
          wordWrap={wordWrap}
          setWordWrap={setWordWrap}
          globalViewMode={globalViewMode}
          setGlobalViewMode={setGlobalViewMode}
          onCopyDiff={() => navigator.clipboard.writeText(data.diff || '')}
          onOpenFile={onOpenFile}
          onRevert={onRevert}
        />
      )}

      <div className={cn('overflow-hidden', showHeader ? 'rounded-b-md' : 'rounded-md', 'border border-border/50')}>
        <DiffEditor
          height={editorHeight}
          language={language}
          original={original}
          modified={modified}
          theme={resolvedTheme === 'dark' ? 'kandev-dark' : 'kandev-light'}
          onMount={handleDiffEditorMount}
          options={{
            fontSize: compact ? 11 : EDITOR_FONT_SIZE,
            fontFamily: EDITOR_FONT_FAMILY,
            lineHeight,
            minimap: { enabled: false },
            wordWrap: wordWrap ? 'on' : 'off',
            readOnly: modifiedReadOnly,
            originalEditable: false,
            renderMarginRevertIcon: !compact && !!onRevert,
            contextmenu: false,
            renderSideBySide: globalViewMode === 'split',
            scrollBeyondLastLine: false,
            smoothScrolling: true,
            automaticLayout: true,
            renderOverviewRuler: false,
            hideUnchangedRegions: {
              enabled: foldUnchanged,
              contextLineCount: 3,
              minimumLineCount: 3,
              revealLineCount: 20,
            },
            folding: !compact,
            lineNumbers: compact ? 'off' : 'on',
            glyphMargin: false,
            lineDecorationsWidth: 10,
            scrollbar: {
              verticalScrollbarSize: 8,
              horizontalScrollbarSize: 8,
              alwaysConsumeMouseWheel: false,
            },
            padding: { top: 2 },
          }}
          loading={
            <div className="flex h-full items-center justify-center text-muted-foreground text-sm">
              Loading diff...
            </div>
          }
        />
      </div>

      {contextMenu && (
        <DiffViewerContextMenu
          contextMenu={contextMenu}
          onCopyAllChanged={copyAllChangedLines}
          onClose={() => setContextMenu(null)}
          onRevert={onRevert}
          filePath={data.filePath}
        />
      )}
    </div>
  );
}

// --- Helpers extracted to keep the main component under lint limits ---

function parseDiffContent(diff: string): { original: string; modified: string } {
  const originalLines: string[] = [];
  const modifiedLines: string[] = [];
  const lines = diff.split('\n');

  for (const line of lines) {
    if (line.startsWith('diff ') || line.startsWith('index ') ||
        line.startsWith('--- ') || line.startsWith('+++ ') ||
        line.startsWith('@@') || line.startsWith('\\')) {
      continue;
    }
    if (line.startsWith('-')) {
      originalLines.push(line.slice(1));
    } else if (line.startsWith('+')) {
      modifiedLines.push(line.slice(1));
    } else {
      const content = line.startsWith(' ') ? line.slice(1) : line;
      originalLines.push(content);
      modifiedLines.push(content);
    }
  }

  return { original: originalLines.join('\n'), modified: modifiedLines.join('\n') };
}

