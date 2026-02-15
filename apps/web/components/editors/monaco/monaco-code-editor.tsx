'use client';

import { useCallback, useEffect, useMemo, useState, useRef } from 'react';
import Editor, { type OnMount, type OnChange } from '@monaco-editor/react';
import type { editor as monacoEditor, IDisposable } from 'monaco-editor';
import { useTheme } from 'next-themes';
import { Button } from '@kandev/ui/button';
import { IconDeviceFloppy, IconLoader2, IconTrash, IconTextWrap, IconTextWrapDisabled, IconMessagePlus, IconArrowsDiff, IconPlugConnected, IconPlugOff, IconAlertTriangle } from '@tabler/icons-react';
import { Tooltip, TooltipContent, TooltipTrigger } from '@kandev/ui/tooltip';
import { formatDiffStats } from '@/lib/utils/file-diff';
import { toRelativePath } from '@/lib/utils';
import { getMonacoLanguage } from '@/lib/editor/language-map';
import { EDITOR_FONT_FAMILY, EDITOR_FONT_SIZE } from '@/lib/theme/editor-theme';
import { useDiffCommentsStore, useFileComments } from '@/lib/state/slices/diff-comments/diff-comments-slice';
import type { DiffComment } from '@/lib/diff/types';
import { useToast } from '@/components/toast-provider';
import { EditorCommentPopover } from '@/components/task/editor-comment-popover';
import { CommentViewPopover } from '@/components/task/comment-view-popover';
import { diffLines } from 'diff';
import { FileActionsDropdown } from '@/components/editors/file-actions-dropdown';
import { useAppStore } from '@/components/state-provider';
import { useLsp } from '@/hooks/use-lsp';
import type { LspStatus } from '@/lib/lsp/lsp-client-manager';
import { lspClientManager } from '@/lib/lsp/lsp-client-manager';
import { consumePendingCursorPosition } from '@/hooks/use-file-editors';
import { initMonacoThemes } from './monaco-init';

initMonacoThemes();

type MonacoCodeEditorProps = {
  path: string;
  content: string;
  originalContent: string;
  isDirty: boolean;
  isSaving: boolean;
  sessionId?: string;
  worktreePath?: string;
  enableComments?: boolean;
  onChange: (newContent: string) => void;
  onSave: () => void;
  onDelete?: () => void;
};

type TextSelection = {
  text: string;
  startLine: number;
  endLine: number;
  position: { x: number; y: number };
} | null;

type FloatingButtonPosition = {
  x: number;
  y: number;
} | null;

type CommentViewState = {
  comments: DiffComment[];
  position: { x: number; y: number };
} | null;

function LspStatusButton({
  status,
  lspLanguage,
  onToggle,
}: {
  status: LspStatus;
  lspLanguage: string | null;
  onToggle: () => void;
}) {
  if (!lspLanguage) return null;

  type StateConfig = { icon: React.ReactNode; tooltip: string; clickable: boolean };
  const configs: Record<string, StateConfig> = {
    disabled: {
      icon: <IconPlugOff className="h-3.5 w-3.5 text-muted-foreground/50" />,
      tooltip: 'LSP: Off \u2014 click to start',
      clickable: true,
    },
    connecting: {
      icon: <IconLoader2 className="h-3.5 w-3.5 animate-spin text-muted-foreground" />,
      tooltip: 'LSP: Connecting...',
      clickable: true,
    },
    installing: {
      icon: <IconLoader2 className="h-3.5 w-3.5 animate-spin text-amber-500" />,
      tooltip: 'LSP: Installing language server...',
      clickable: false,
    },
    starting: {
      icon: <IconLoader2 className="h-3.5 w-3.5 animate-spin text-blue-500" />,
      tooltip: 'LSP: Starting language server...',
      clickable: true,
    },
    ready: {
      icon: <IconPlugConnected className="h-3.5 w-3.5 text-emerald-500" />,
      tooltip: 'LSP: Connected \u2014 click to stop',
      clickable: true,
    },
    stopping: {
      icon: <IconLoader2 className="h-3.5 w-3.5 animate-spin text-muted-foreground" />,
      tooltip: 'LSP: Stopping...',
      clickable: false,
    },
    unavailable: {
      icon: <IconPlugOff className="h-3.5 w-3.5 text-muted-foreground" />,
      tooltip: `LSP: ${'reason' in status ? status.reason : 'Unavailable'}`,
      clickable: true,
    },
    error: {
      icon: <IconAlertTriangle className="h-3.5 w-3.5 text-yellow-500" />,
      tooltip: `LSP: ${'reason' in status ? status.reason : 'Error'}`,
      clickable: true,
    },
  };

  const c = configs[status.state];
  if (!c) return null;

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          variant="ghost"
          size="sm"
          className="h-6 w-6 p-0 cursor-pointer"
          onClick={c.clickable ? onToggle : undefined}
          disabled={!c.clickable}
        >
          {c.icon}
        </Button>
      </TooltipTrigger>
      <TooltipContent>{c.tooltip}</TooltipContent>
    </Tooltip>
  );
}

export function MonacoCodeEditor({
  path,
  content,
  originalContent,
  isDirty,
  isSaving,
  sessionId,
  worktreePath,
  enableComments = false,
  onChange,
  onSave,
  onDelete,
}: MonacoCodeEditorProps) {
  const { resolvedTheme } = useTheme();
  const [wrapEnabled, setWrapEnabled] = useState(true);
  const [textSelection, setTextSelection] = useState<TextSelection>(null);
  const [floatingButtonPos, setFloatingButtonPos] = useState<FloatingButtonPosition>(null);
  const [currentSelection, setCurrentSelection] = useState<{ text: string; startLine: number; endLine: number } | null>(null);
  const [commentView, setCommentView] = useState<CommentViewState>(null);
  const [showDiffIndicators, setShowDiffIndicators] = useState(true);
  const editorRef = useRef<monacoEditor.IStandaloneCodeEditor | null>(null);
  const wrapperRef = useRef<HTMLDivElement>(null);
  const mousePositionRef = useRef<{ x: number; y: number }>({ x: 0, y: 0 });
  const decorationsRef = useRef<monacoEditor.IEditorDecorationsCollection | null>(null);
  const diffDecorationsRef = useRef<monacoEditor.IEditorDecorationsCollection | null>(null);
  const disposablesRef = useRef<IDisposable[]>([]);
  const { toast } = useToast();

  const addComment = useDiffCommentsStore((state) => state.addComment);
  const removeComment = useDiffCommentsStore((state) => state.removeComment);
  const comments = useFileComments(sessionId ?? '', path);

  const language = getMonacoLanguage(path);

  // LSP integration
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const lspSessionId = sessionId ?? activeSessionId ?? null;
  const { status: lspStatus, lspLanguage, toggle: toggleLsp } = useLsp(lspSessionId, language);

  const hasLspActive = lspStatus.state === 'ready';

  // Use absolute path for Monaco model URI so the LSP server receives correct file:// URIs
  const monacoPath = worktreePath ? `${worktreePath}/${path}` : path;

  // LSP document sync: open/close document when LSP becomes ready or component mounts/unmounts
  const documentUri = `file://${monacoPath}`;
  useEffect(() => {
    if (!hasLspActive || !lspSessionId || !lspLanguage) return;
    lspClientManager.openDocument(lspSessionId, lspLanguage, documentUri, language, content);
    return () => {
      lspClientManager.closeDocument(lspSessionId, lspLanguage, documentUri);
    };
    // Only run on mount/unmount and when LSP readiness changes — NOT on content changes
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [hasLspActive, lspSessionId, lspLanguage, documentUri, language]);

  // LSP document change: notify server of content changes (debounced)
  const changeTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  useEffect(() => {
    if (!hasLspActive || !lspSessionId || !lspLanguage) return;
    if (changeTimerRef.current) clearTimeout(changeTimerRef.current);
    changeTimerRef.current = setTimeout(() => {
      lspClientManager.changeDocument(lspSessionId, lspLanguage, documentUri, content);
    }, 300);
    return () => {
      if (changeTimerRef.current) clearTimeout(changeTimerRef.current);
    };
  }, [hasLspActive, lspSessionId, lspLanguage, documentUri, content]);

  // Toast on LSP status changes
  const lspStateForToast = lspStatus.state;
  const lspReasonForToast = 'reason' in lspStatus ? lspStatus.reason : null;
  useEffect(() => {
    if (lspStateForToast === 'installing') {
      toast({
        title: 'Installing language server',
        description: 'This may take a moment...',
      });
    } else if (lspStateForToast === 'unavailable' && lspReasonForToast) {
      toast({
        title: 'Language server not found',
        description: `${lspReasonForToast}. Enable auto-install in Settings → Editors.`,
      });
    } else if (lspStateForToast === 'error' && lspReasonForToast) {
      toast({
        title: 'LSP error',
        description: lspReasonForToast,
      });
    }
  }, [lspStateForToast, lspReasonForToast, toast]);

  // Track mouse position
  useEffect(() => {
    const handleMouseMove = (e: MouseEvent) => {
      mousePositionRef.current = { x: e.clientX, y: e.clientY };
    };
    document.addEventListener('mousemove', handleMouseMove);
    return () => document.removeEventListener('mousemove', handleMouseMove);
  }, []);

  // Handle editor mount
  const handleEditorDidMount: OnMount = useCallback(
    (editor) => {
      editorRef.current = editor;
      decorationsRef.current = editor.createDecorationsCollection([]);
      diffDecorationsRef.current = editor.createDecorationsCollection([]);

      // Jump to pending cursor position (e.g. from LSP Go-to-Definition)
      const pendingPos = consumePendingCursorPosition(path);
      if (pendingPos) {
        editor.setPosition({ lineNumber: pendingPos.line, column: pendingPos.column });
        editor.revealLineInCenter(pendingPos.line);
      }

      // Cmd/Ctrl+S to save
      editor.addCommand(
        (window as { monaco?: typeof import('monaco-editor') }).monaco!.KeyMod.CtrlCmd |
          (window as { monaco?: typeof import('monaco-editor') }).monaco!.KeyCode.KeyS,
        () => {
          onSave();
        }
      );

      // Selection change listener for comments
      if (enableComments && sessionId) {
        const disposable = editor.onDidChangeCursorSelection(() => {
          const selection = editor.getSelection();
          if (!selection || selection.isEmpty()) {
            setCurrentSelection(null);
            return;
          }
          const model = editor.getModel();
          if (!model) return;
          const text = model.getValueInRange(selection);
          if (!text.trim()) {
            setCurrentSelection(null);
            return;
          }
          setCurrentSelection({
            text,
            startLine: selection.startLineNumber,
            endLine: selection.endLineNumber,
          });
        });
        disposablesRef.current.push(disposable);
      }

      // Gutter click for viewing comments
      const glyphDisposable = editor.onMouseDown((e) => {
        if (e.target.type === 2 /* GUTTER_GLYPH_MARGIN */ || e.target.type === 3 /* GUTTER_LINE_NUMBERS */) {
          const lineNumber = e.target.position?.lineNumber;
          if (!lineNumber) return;

          const lineComments = comments.filter(
            (c) => lineNumber >= c.startLine && lineNumber <= c.endLine
          );
          if (lineComments.length > 0 && e.event.browserEvent) {
            setCommentView({
              comments: lineComments,
              position: { x: e.event.browserEvent.clientX, y: e.event.browserEvent.clientY },
            });
          }
        }
      });
      disposablesRef.current.push(glyphDisposable);
    },
    [path, onSave, enableComments, sessionId, comments]
  );

  // Cleanup disposables
  useEffect(() => {
    return () => {
      for (const d of disposablesRef.current) d.dispose();
      disposablesRef.current = [];
    };
  }, []);

  // Update decorations when comments change
  useEffect(() => {
    if (!decorationsRef.current || !editorRef.current) return;

    const decorations: monacoEditor.IModelDeltaDecoration[] = [];
    const linesWithComments = new Set<number>();
    const firstLines = new Set<number>();

    for (const comment of comments) {
      firstLines.add(comment.startLine);
      for (let line = comment.startLine; line <= comment.endLine; line++) {
        linesWithComments.add(line);
      }
    }

    for (const lineNum of linesWithComments) {
      decorations.push({
        range: {
          startLineNumber: lineNum,
          startColumn: 1,
          endLineNumber: lineNum,
          endColumn: 1,
        },
        options: {
          isWholeLine: true,
          className: 'monaco-comment-line',
          glyphMarginClassName: firstLines.has(lineNum)
            ? 'monaco-comment-glyph'
            : 'monaco-comment-glyph-bg',
          glyphMarginHoverMessage: {
            value: `${comments.filter((c) => lineNum >= c.startLine && lineNum <= c.endLine).length} comment(s) - click to view`,
          },
        },
      });
    }

    decorationsRef.current.set(decorations);
  }, [comments]);

  // Diff gutter indicators
  useEffect(() => {
    if (!diffDecorationsRef.current || !editorRef.current) return;

    if (!showDiffIndicators || !isDirty || !originalContent) {
      diffDecorationsRef.current.set([]);
      return;
    }

    const changes = diffLines(originalContent, content);
    const decorations: monacoEditor.IModelDeltaDecoration[] = [];
    let currentLine = 1;

    for (let i = 0; i < changes.length; i++) {
      const change = changes[i];
      const lineCount = change.count ?? 0;

      if (change.removed) {
        // Check if next change is an addition (modified lines)
        const next = changes[i + 1];
        if (next && next.added) {
          // Modified: removed + added pair
          const addedLineCount = next.count ?? 0;
          for (let j = 0; j < addedLineCount; j++) {
            decorations.push({
              range: { startLineNumber: currentLine + j, startColumn: 1, endLineNumber: currentLine + j, endColumn: 1 },
              options: { isWholeLine: true, linesDecorationsClassName: 'monaco-diff-modified-gutter' },
            });
          }
          currentLine += addedLineCount;
          i++; // skip the added part
        } else {
          // Standalone deletion: red indicator at previous line
          const indicatorLine = Math.max(1, currentLine - 1);
          decorations.push({
            range: { startLineNumber: indicatorLine, startColumn: 1, endLineNumber: indicatorLine, endColumn: 1 },
            options: { isWholeLine: true, linesDecorationsClassName: 'monaco-diff-deleted-gutter' },
          });
        }
      } else if (change.added) {
        // Standalone addition
        for (let j = 0; j < lineCount; j++) {
          decorations.push({
            range: { startLineNumber: currentLine + j, startColumn: 1, endLineNumber: currentLine + j, endColumn: 1 },
            options: { isWholeLine: true, linesDecorationsClassName: 'monaco-diff-added-gutter' },
          });
        }
        currentLine += lineCount;
      } else {
        // Unchanged
        currentLine += lineCount;
      }
    }

    diffDecorationsRef.current.set(decorations);
  }, [content, originalContent, showDiffIndicators, isDirty]);

  // Show floating button after mouse up
  useEffect(() => {
    const wrapper = wrapperRef.current;
    if (!wrapper || !enableComments || !sessionId) return;

    const handleMouseUp = (e: MouseEvent) => {
      if ((e.target as HTMLElement).closest('.floating-comment-btn')) return;
      setTimeout(() => {
        if (currentSelection) {
          setFloatingButtonPos({ x: mousePositionRef.current.x, y: mousePositionRef.current.y });
        }
      }, 10);
    };

    const handleMouseDown = (e: MouseEvent) => {
      if ((e.target as HTMLElement).closest('.floating-comment-btn')) return;
      setFloatingButtonPos(null);
    };

    wrapper.addEventListener('mouseup', handleMouseUp);
    wrapper.addEventListener('mousedown', handleMouseDown);
    return () => {
      wrapper.removeEventListener('mouseup', handleMouseUp);
      wrapper.removeEventListener('mousedown', handleMouseDown);
    };
  }, [enableComments, sessionId, currentSelection]);

  // Cmd+I for comment popover
  useEffect(() => {
    const wrapper = wrapperRef.current;
    if (!wrapper || !enableComments || !sessionId) return;

    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === 'i') {
        if (!currentSelection || !floatingButtonPos) return;
        e.preventDefault();
        e.stopPropagation();
        setTextSelection({ ...currentSelection, position: floatingButtonPos });
        setFloatingButtonPos(null);
      }
    };

    wrapper.addEventListener('keydown', handleKeyDown, true);
    return () => wrapper.removeEventListener('keydown', handleKeyDown, true);
  }, [enableComments, sessionId, currentSelection, floatingButtonPos]);

  // Escape to close popovers
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        if (textSelection) setTextSelection(null);
        if (commentView) setCommentView(null);
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [textSelection, commentView]);

  const handleChange: OnChange = useCallback(
    (value) => {
      if (value !== undefined) onChange(value);
    },
    [onChange]
  );

  const handleFloatingButtonClick = useCallback(
    (e: React.MouseEvent) => {
      e.preventDefault();
      e.stopPropagation();
      if (!currentSelection || !floatingButtonPos) return;
      setTextSelection({ ...currentSelection, position: floatingButtonPos });
      setFloatingButtonPos(null);
    },
    [currentSelection, floatingButtonPos]
  );

  const handleCommentSubmit = useCallback(
    (annotation: string) => {
      if (!textSelection || !sessionId) return;
      const comment: DiffComment = {
        id: `${path}-${Date.now()}-${Math.random().toString(36).substr(2, 9)}`,
        sessionId,
        filePath: path,
        startLine: textSelection.startLine,
        endLine: textSelection.endLine,
        side: 'additions',
        codeContent: textSelection.text,
        annotation,
        createdAt: new Date().toISOString(),
        status: 'pending',
      };
      addComment(comment);
      setTextSelection(null);

      // Clear editor selection
      const editor = editorRef.current;
      if (editor) {
        const pos = editor.getPosition();
        if (pos) editor.setSelection({ startLineNumber: pos.lineNumber, startColumn: pos.column, endLineNumber: pos.lineNumber, endColumn: pos.column });
      }

      toast({ title: 'Comment added', description: 'Your comment will be sent with your next message.' });
    },
    [textSelection, sessionId, path, addComment, toast]
  );

  const handlePopoverClose = useCallback(() => setTextSelection(null), []);

  const handleDeleteComment = useCallback(
    (commentId: string) => {
      if (!sessionId) return;
      removeComment(sessionId, path, commentId);
      if (commentView && commentView.comments.length <= 1) {
        setCommentView(null);
      } else if (commentView) {
        setCommentView({
          ...commentView,
          comments: commentView.comments.filter((c) => c.id !== commentId),
        });
      }
      toast({ title: 'Comment deleted' });
    },
    [sessionId, path, removeComment, commentView, toast]
  );

  const handleCommentViewClose = useCallback(() => setCommentView(null), []);

  // Diff stats
  const diffStats = useMemo(() => {
    if (!isDirty) return null;
    const originalLines = originalContent.split('\n');
    const currentLines = content.split('\n');
    let additions = 0;
    let deletions = 0;
    const maxLen = Math.max(originalLines.length, currentLines.length);
    for (let i = 0; i < maxLen; i++) {
      const origLine = originalLines[i];
      const currLine = currentLines[i];
      if (origLine === undefined && currLine !== undefined) additions++;
      else if (origLine !== undefined && currLine === undefined) deletions++;
      else if (origLine !== currLine) { additions++; deletions++; }
    }
    return { additions, deletions };
  }, [isDirty, content, originalContent]);

  return (
    <div ref={wrapperRef} className="flex h-full flex-col rounded-lg">
      {/* Editor header */}
      <div className="flex items-center justify-between px-2 border-foreground/10 border-b">
        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          <span className="font-mono">{toRelativePath(path, worktreePath)}</span>
          {isDirty && diffStats && (
            <span className="text-xs text-yellow-500">
              {formatDiffStats(diffStats.additions, diffStats.deletions)}
            </span>
          )}
        </div>
        <div className="flex items-center gap-1">
          {enableComments && sessionId && comments.length > 0 && (
            <div className="flex items-center gap-1 px-2 py-1 text-xs text-primary">
              <IconMessagePlus className="h-3.5 w-3.5" />
              <span>{comments.length} comment{comments.length > 1 ? 's' : ''}</span>
            </div>
          )}
          <LspStatusButton status={lspStatus} lspLanguage={lspLanguage} onToggle={toggleLsp} />
          {isDirty && (
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  size="sm"
                  variant="ghost"
                  onClick={() => setShowDiffIndicators(!showDiffIndicators)}
                  className={`h-8 w-8 p-0 cursor-pointer ${showDiffIndicators ? 'text-foreground' : 'text-muted-foreground'}`}
                >
                  <IconArrowsDiff className="h-4 w-4" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>{showDiffIndicators ? 'Hide diff indicators' : 'Show diff indicators'}</TooltipContent>
            </Tooltip>
          )}
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                size="sm"
                variant="ghost"
                onClick={() => setWrapEnabled(!wrapEnabled)}
                className={`h-8 w-8 p-0 cursor-pointer ${wrapEnabled ? 'text-foreground' : 'text-muted-foreground'}`}
              >
                {wrapEnabled ? <IconTextWrap className="h-4 w-4" /> : <IconTextWrapDisabled className="h-4 w-4" />}
              </Button>
            </TooltipTrigger>
            <TooltipContent>{wrapEnabled ? 'Disable word wrap' : 'Enable word wrap'}</TooltipContent>
          </Tooltip>
          <FileActionsDropdown filePath={path} sessionId={sessionId} size="sm" />
          {onDelete && (
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  size="sm"
                  variant="ghost"
                  onClick={onDelete}
                  className="h-8 w-8 p-0 cursor-pointer text-muted-foreground hover:text-destructive"
                >
                  <IconTrash className="h-4 w-4" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>Delete file</TooltipContent>
            </Tooltip>
          )}
          <Button
            size="sm"
            variant="default"
            onClick={onSave}
            disabled={!isDirty || isSaving}
            className="cursor-pointer gap-2"
          >
            {isSaving ? (
              <>
                <IconLoader2 className="h-4 w-4 animate-spin" />
                Saving...
              </>
            ) : (
              <>
                <IconDeviceFloppy className="h-4 w-4" />
                Save
                <span className="text-xs text-muted-foreground">
                  ({navigator.platform.includes('Mac') ? '\u2318' : 'Ctrl'}+S)
                </span>
              </>
            )}
          </Button>
        </div>
      </div>

      {/* Monaco editor */}
      <div className="flex-1 overflow-hidden relative">
        <Editor
          height="100%"
          language={language}
          path={monacoPath}
          value={content}
          theme={resolvedTheme === 'dark' ? 'kandev-dark' : 'kandev-light'}
          onChange={handleChange}
          onMount={handleEditorDidMount}
          options={{
            fontSize: EDITOR_FONT_SIZE,
            fontFamily: EDITOR_FONT_FAMILY,
            lineHeight: 18,
            minimap: { enabled: false },
            wordWrap: wrapEnabled ? 'on' : 'off',
            scrollBeyondLastLine: false,
            smoothScrolling: true,
            cursorSmoothCaretAnimation: 'on',
            glyphMargin: enableComments,
            folding: true,
            lineNumbers: 'on',
            renderLineHighlight: 'line',
            automaticLayout: true,
            scrollbar: {
              verticalScrollbarSize: 10,
              horizontalScrollbarSize: 10,
            },
            padding: { top: 4 },
            'semanticHighlighting.enabled': true,
          }}
          loading={
            <div className="flex h-full items-center justify-center text-muted-foreground text-sm">
              Loading editor...
            </div>
          }
        />

        {/* Floating comment button */}
        {floatingButtonPos && !textSelection && (
          <Button
            size="sm"
            variant="secondary"
            className="floating-comment-btn fixed z-50 gap-1.5 shadow-lg animate-in fade-in-0 zoom-in-95 duration-100 cursor-pointer"
            style={{ left: floatingButtonPos.x + 8, top: floatingButtonPos.y + 8 }}
            onMouseDown={(e) => e.stopPropagation()}
            onClick={handleFloatingButtonClick}
          >
            <IconMessagePlus className="h-3.5 w-3.5" />
            Comment
          </Button>
        )}

        {textSelection && (
          <EditorCommentPopover
            selectedText={textSelection.text}
            lineRange={{ start: textSelection.startLine, end: textSelection.endLine }}
            position={textSelection.position}
            onSubmit={handleCommentSubmit}
            onClose={handlePopoverClose}
          />
        )}

        {commentView && (
          <CommentViewPopover
            comments={commentView.comments}
            position={commentView.position}
            onDelete={handleDeleteComment}
            onClose={handleCommentViewClose}
          />
        )}
      </div>
    </div>
  );
}
