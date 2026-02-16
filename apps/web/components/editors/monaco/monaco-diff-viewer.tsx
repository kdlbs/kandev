'use client';

import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState, useSyncExternalStore } from 'react';
import { DiffEditor, type DiffOnMount } from '@monaco-editor/react';
import type { editor as monacoEditor } from 'monaco-editor';
import { useTheme } from 'next-themes';
import { cn } from '@kandev/ui/lib/utils';
import { Button } from '@kandev/ui/button';
import { Tooltip, TooltipTrigger, TooltipContent } from '@kandev/ui/tooltip';
import { IconCopy, IconTextWrap, IconLayoutRows, IconLayoutColumns, IconPencil, IconArrowBackUp, IconFoldDown, IconFold } from '@tabler/icons-react';
import type { FileDiffData, DiffComment } from '@/lib/diff/types';
import { getMonacoLanguage } from '@/lib/editor/language-map';
import { EDITOR_FONT_FAMILY, EDITOR_FONT_SIZE } from '@/lib/theme/editor-theme';
import { useDiffComments } from '@/components/diff/use-diff-comments';
import { CommentForm } from '@/components/diff/comment-form';
import { CommentDisplay } from '@/components/diff/comment-display';
import { initMonacoThemes } from './monaco-init';

initMonacoThemes();

// Global view mode sync (same as pierre diff-viewer)
const DIFF_VIEW_MODE_KEY = 'diff-view-mode';
const DEFAULT_VIEW_MODE = 'unified' as const;
const VIEW_MODE_CHANGE_EVENT = 'diff-view-mode-change';
type ViewMode = 'split' | 'unified';

function getStoredViewMode(): ViewMode {
  if (typeof window === 'undefined') return DEFAULT_VIEW_MODE;
  const stored = localStorage.getItem(DIFF_VIEW_MODE_KEY);
  return stored === 'split' || stored === 'unified' ? stored : DEFAULT_VIEW_MODE;
}

function setStoredViewMode(mode: ViewMode): void {
  localStorage.setItem(DIFF_VIEW_MODE_KEY, mode);
  window.dispatchEvent(new CustomEvent(VIEW_MODE_CHANGE_EVENT, { detail: mode }));
}

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

// Global folding toggle sync (same pattern as view mode)
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
}: MonacoDiffViewerProps) {
  const { resolvedTheme } = useTheme();
  const [globalViewMode, setGlobalViewMode] = useGlobalViewMode();
  const [foldUnchanged, setFoldUnchanged] = useGlobalFolding();
  const [wordWrapLocal, setWordWrap] = useState(false);
  const wordWrap = wordWrapProp ?? wordWrapLocal;
  const diffEditorRef = useRef<monacoEditor.IStandaloneDiffEditor | null>(null);
  const wrapperRef = useRef<HTMLDivElement>(null);

  // Fix: reset the DiffEditor model BEFORE @monaco-editor/react's useEffect cleanup
  // disposes the models. useLayoutEffect cleanup runs before useEffect cleanup,
  // so the DiffEditorWidget releases model references before models are disposed.
  // This prevents "TextModel got disposed before DiffEditorWidget model got reset".
  useLayoutEffect(() => {
    return () => {
      try { diffEditorRef.current?.setModel(null); } catch { /* already disposed */ }
    };
  }, []);

  // Context menu state
  type ContextMenuState = {
    x: number;
    y: number;
    lineNumber: number;
    side: 'original' | 'modified';
    isChangedLine: boolean;
    lineContent: string;
  } | null;
  const [contextMenu, setContextMenu] = useState<ContextMenuState>(null);

  // Comment form state
  const [showCommentForm, setShowCommentForm] = useState(false);
  const [selectedLineRange, setSelectedLineRange] = useState<{ start: number; end: number; side: string } | null>(null);

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

  const language = getMonacoLanguage(data.filePath);
  const showHeader = !hideHeader && !compact;

  // Parse original/modified content from diff data
  // When only a unified diff string is provided (e.g. git diffs), reconstruct
  // the original and modified content by parsing the patch hunks.
  const { original, modified } = useMemo(() => {
    if (data.oldContent || data.newContent) {
      return {
        original: data.oldContent ?? '',
        modified: data.newContent ?? '',
      };
    }

    // Reconstruct from unified diff patch
    if (data.diff) {
      const originalLines: string[] = [];
      const modifiedLines: string[] = [];
      const lines = data.diff.split('\n');

      for (const line of lines) {
        // Skip diff headers
        if (line.startsWith('diff ') || line.startsWith('index ') ||
            line.startsWith('--- ') || line.startsWith('+++ ') ||
            line.startsWith('@@')) {
          continue;
        }

        if (line.startsWith('-')) {
          originalLines.push(line.slice(1));
        } else if (line.startsWith('+')) {
          modifiedLines.push(line.slice(1));
        } else if (line.startsWith(' ') || line === '') {
          // Context line (or empty line between hunks)
          const content = line.startsWith(' ') ? line.slice(1) : line;
          originalLines.push(content);
          modifiedLines.push(content);
        } else if (line.startsWith('\\')) {
          // "\ No newline at end of file" — skip
          continue;
        }
      }

      return {
        original: originalLines.join('\n'),
        modified: modifiedLines.join('\n'),
      };
    }

    return { original: '', modified: '' };
  }, [data.oldContent, data.newContent, data.diff]);

  const handleDiffEditorMount: DiffOnMount = useCallback((editor) => {
    diffEditorRef.current = editor;

    // Register context menu on both sides
    const modifiedEditor = editor.getModifiedEditor();
    const originalEditor = editor.getOriginalEditor();

    const handleContextMenu = (side: 'original' | 'modified') => (e: monacoEditor.IEditorMouseEvent) => {
      if (e.target.position) {
        e.event.preventDefault();
        e.event.stopPropagation();
        const lineNumber = e.target.position.lineNumber;
        const targetEditor = side === 'modified' ? modifiedEditor : originalEditor;
        const model = targetEditor.getModel();
        const lineContent = model ? model.getLineContent(lineNumber) : '';
        const lineChanges = editor.getLineChanges();
        let isChangedLine = false;
        if (lineChanges) {
          for (const change of lineChanges) {
            if (side === 'modified') {
              if (lineNumber >= change.modifiedStartLineNumber && lineNumber <= change.modifiedEndLineNumber) {
                isChangedLine = true;
                break;
              }
            } else {
              if (lineNumber >= change.originalStartLineNumber && lineNumber <= change.originalEndLineNumber) {
                isChangedLine = true;
                break;
              }
            }
          }
        }
        setContextMenu({
          x: e.event.posx,
          y: e.event.posy,
          lineNumber,
          side,
          isChangedLine,
          lineContent,
        });
      }
    };

    modifiedEditor.onContextMenu(handleContextMenu('modified'));
    originalEditor.onContextMenu(handleContextMenu('original'));
  }, []);

  // Handle comment submission
  const handleCommentSubmit = useCallback(
    (content: string) => {
      if (!selectedLineRange) return;
      if (onCommentAdd && externalComments !== undefined) {
        const comment: DiffComment = {
          id: `${data.filePath}-${Date.now()}`,
          sessionId: sessionId || '',
          filePath: data.filePath,
          startLine: Math.min(selectedLineRange.start, selectedLineRange.end),
          endLine: Math.max(selectedLineRange.start, selectedLineRange.end),
          side: (selectedLineRange.side || 'additions') as DiffComment['side'],
          codeContent: '',
          annotation: content,
          createdAt: new Date().toISOString(),
          status: 'pending',
        };
        onCommentAdd(comment);
      } else if (sessionId) {
        addComment(
          {
            start: selectedLineRange.start,
            end: selectedLineRange.end,
            side: selectedLineRange.side as 'additions' | 'deletions',
          },
          content
        );
      }
      setShowCommentForm(false);
      setSelectedLineRange(null);
    },
    [selectedLineRange, sessionId, data.filePath, addComment, onCommentAdd, externalComments]
  );

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

  const handleCommentUpdate = useCallback(
    (commentId: string, content: string) => {
      updateComment(commentId, { annotation: content });
      setEditingComment(null);
    },
    [updateComment, setEditingComment]
  );

  // Close context menu on click or Escape
  useEffect(() => {
    if (!contextMenu) return;
    const handleClose = () => setContextMenu(null);
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setContextMenu(null);
    };
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

  return (
    <div ref={wrapperRef} className={cn('monaco-diff-viewer relative', className)}>
      {/* Toolbar */}
      {showHeader && (
        <div className="flex items-center justify-between px-3 py-1.5 border-b border-border/50 bg-card/50 rounded-t-md text-xs text-muted-foreground">
          <span className="font-mono truncate">{data.filePath}</span>
          <div className="flex items-center gap-1">
            <Tooltip>
              <TooltipTrigger asChild>
                <Button variant="ghost" size="sm" className="h-6 w-6 p-0 cursor-pointer opacity-60 hover:opacity-100" onClick={() => navigator.clipboard.writeText(data.diff || '')}>
                  <IconCopy className="h-3.5 w-3.5" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>Copy diff</TooltipContent>
            </Tooltip>
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="ghost"
                  size="sm"
                  className={cn('h-6 w-6 p-0 cursor-pointer opacity-60 hover:opacity-100', foldUnchanged && 'opacity-100 bg-muted')}
                  onClick={() => setFoldUnchanged(!foldUnchanged)}
                >
                  {foldUnchanged ? <IconFoldDown className="h-3.5 w-3.5" /> : <IconFold className="h-3.5 w-3.5" />}
                </Button>
              </TooltipTrigger>
              <TooltipContent>{foldUnchanged ? 'Show all lines' : 'Fold unchanged lines'}</TooltipContent>
            </Tooltip>
            {onRevert && (
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button variant="ghost" size="sm" className="h-6 w-6 p-0 cursor-pointer opacity-60 hover:opacity-100" onClick={() => onRevert(data.filePath)}>
                    <IconArrowBackUp className="h-3.5 w-3.5" />
                  </Button>
                </TooltipTrigger>
                <TooltipContent>Revert changes</TooltipContent>
              </Tooltip>
            )}
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="ghost"
                  size="sm"
                  className={cn('h-6 w-6 p-0 cursor-pointer opacity-60 hover:opacity-100', wordWrap && 'opacity-100 bg-muted')}
                  onClick={() => setWordWrap(!wordWrap)}
                >
                  <IconTextWrap className="h-3.5 w-3.5" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>Toggle word wrap</TooltipContent>
            </Tooltip>
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="ghost"
                  size="sm"
                  className="h-6 w-6 p-0 cursor-pointer opacity-60 hover:opacity-100"
                  onClick={() => setGlobalViewMode(globalViewMode === 'split' ? 'unified' : 'split')}
                >
                  {globalViewMode === 'split' ? <IconLayoutRows className="h-3.5 w-3.5" /> : <IconLayoutColumns className="h-3.5 w-3.5" />}
                </Button>
              </TooltipTrigger>
              <TooltipContent>{globalViewMode === 'split' ? 'Switch to unified view' : 'Switch to split view'}</TooltipContent>
            </Tooltip>
            {onOpenFile && (
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button variant="ghost" size="sm" className="h-6 w-6 p-0 cursor-pointer opacity-60 hover:opacity-100" onClick={() => onOpenFile(data.filePath)}>
                    <IconPencil className="h-3.5 w-3.5" />
                  </Button>
                </TooltipTrigger>
                <TooltipContent>Edit</TooltipContent>
              </Tooltip>
            )}
          </div>
        </div>
      )}

      {/* Diff editor */}
      <div className={cn('overflow-hidden', showHeader ? 'rounded-b-md' : 'rounded-md', 'border border-border/50')}>
        <DiffEditor
          height={compact ? '200px' : '400px'}
          language={language}
          original={original}
          modified={modified}
          theme={resolvedTheme === 'dark' ? 'kandev-dark' : 'kandev-light'}
          onMount={handleDiffEditorMount}
          options={{
            fontSize: compact ? 11 : EDITOR_FONT_SIZE,
            fontFamily: EDITOR_FONT_FAMILY,
            lineHeight: compact ? 16 : 18,
            minimap: { enabled: false },
            wordWrap: wordWrap ? 'on' : 'off',
            readOnly: true,
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

      {/* Comment annotations below the diff */}
      {comments.length > 0 && (
        <div className="mt-2 space-y-2">
          {comments.map((comment) => (
            <div key={comment.id} className="px-2">
              {editingCommentId === comment.id ? (
                <CommentForm
                  initialContent={comment.annotation}
                  onSubmit={(content) => handleCommentUpdate(comment.id, content)}
                  onCancel={() => setEditingComment(null)}
                  isEditing
                />
              ) : (
                <CommentDisplay
                  comment={comment}
                  onDelete={() => handleCommentDelete(comment.id)}
                  onEdit={() => setEditingComment(comment.id)}
                  showCode={false}
                />
              )}
            </div>
          ))}
        </div>
      )}

      {/* New comment form */}
      {showCommentForm && selectedLineRange && (
        <div className="mt-2 px-2">
          <CommentForm
            onSubmit={handleCommentSubmit}
            onCancel={() => {
              setShowCommentForm(false);
              setSelectedLineRange(null);
            }}
          />
        </div>
      )}

      {/* Context menu */}
      {contextMenu && (
        <div
          className="fixed z-50 min-w-[180px] rounded-md border border-border bg-popover text-popover-foreground shadow-md ring-1 ring-border/10 py-1 text-xs"
          style={{ left: contextMenu.x, top: contextMenu.y }}
          onMouseDown={(e) => e.stopPropagation()}
        >
          <button
            className="flex w-full items-center px-3 py-1.5 hover:bg-accent hover:text-accent-foreground cursor-pointer"
            onClick={copyAllChangedLines}
          >
            Copy all changed lines
          </button>
          {contextMenu.isChangedLine && (
            <button
              className="flex w-full items-center px-3 py-1.5 hover:bg-accent hover:text-accent-foreground cursor-pointer"
              onClick={() => {
                navigator.clipboard.writeText(contextMenu.lineContent);
                setContextMenu(null);
              }}
            >
              Copy line {contextMenu.lineNumber}
            </button>
          )}
          {onRevert && (
            <>
              <div className="my-1 border-t border-border" />
              <button
                className="flex w-full items-center px-3 py-1.5 hover:bg-accent hover:text-accent-foreground cursor-pointer text-destructive"
                onClick={() => {
                  onRevert(data.filePath);
                  setContextMenu(null);
                }}
              >
                Revert all changes
              </button>
            </>
          )}
        </div>
      )}
    </div>
  );
}
