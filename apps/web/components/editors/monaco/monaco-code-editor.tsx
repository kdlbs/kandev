'use client';

import { useCallback, useEffect, useState, useRef } from 'react';
import Editor, { type OnMount, type OnChange } from '@monaco-editor/react';
import type { editor as monacoEditor, IDisposable } from 'monaco-editor';
import { useTheme } from 'next-themes';
import { Button } from '@kandev/ui/button';
import { IconDeviceFloppy, IconLoader2, IconTrash, IconTextWrap, IconTextWrapDisabled, IconMessagePlus, IconArrowsDiff } from '@tabler/icons-react';
import { Tooltip, TooltipContent, TooltipTrigger } from '@kandev/ui/tooltip';
import { formatDiffStats } from '@/lib/utils/file-diff';
import { toRelativePath } from '@/lib/utils';
import { getMonacoLanguage } from '@/lib/editor/language-map';
import { EDITOR_FONT_FAMILY, EDITOR_FONT_SIZE } from '@/lib/theme/editor-theme';
import { useDiffCommentsStore, useFileComments } from '@/lib/state/slices/diff-comments/diff-comments-slice';
import type { DiffComment } from '@/lib/diff/types';
import { buildDiffComment, useCommentedLines } from '@/lib/diff/comment-utils';
import { computeLineDiffStats } from '@/lib/diff';
import { useToast } from '@/components/toast-provider';
import { CommentForm } from '@/components/diff/comment-form';
import { CommentDisplay } from '@/components/diff/comment-display';
import { diffLines } from 'diff';
import { FileActionsDropdown } from '@/components/editors/file-actions-dropdown';
import { PanelHeaderBarSplit } from '@/components/task/panel-primitives';
import { useAppStore } from '@/components/state-provider';
import { useLsp } from '@/hooks/use-lsp';
import { lspClientManager } from '@/lib/lsp/lsp-client-manager';
import { useDockviewStore } from '@/lib/state/dockview-store';
import { consumePendingCursorPosition } from '@/hooks/use-file-editors';
import { useCommandPanelOpen } from '@/lib/commands/command-registry';
import { useGutterComments } from '@/hooks/use-gutter-comments';
import { useEditorViewZoneComments } from '@/hooks/use-editor-view-zone-comments';
import { LspStatusButton } from '@/components/editors/lsp-status-button';
import { initMonacoThemes } from './monaco-init';

initMonacoThemes();

type MonacoCodeEditorProps = {
  path: string;
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

type FormZoneRange = {
  startLine: number;
  endLine: number;
  codeContent: string;
} | null;

type FloatingButtonPosition = {
  x: number;
  y: number;
} | null;

export function MonacoCodeEditor({
  path,
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

  // Read initial content from the store once (not reactive — Monaco manages content internally)
  const [initialContent] = useState(
    () => useDockviewStore.getState().openFiles.get(path)?.content ?? ''
  );
  // Track current content via ref for non-React uses (diff decorations, LSP sync, diff stats)
  const contentRef = useRef(initialContent);

  const [wrapEnabled, setWrapEnabled] = useState(true);
  const [formZoneRange, setFormZoneRange] = useState<FormZoneRange>(null);
  const [floatingButtonPos, setFloatingButtonPos] = useState<FloatingButtonPosition>(null);
  const [currentSelection, setCurrentSelection] = useState<{ text: string; startLine: number; endLine: number } | null>(null);
  const [showDiffIndicators, setShowDiffIndicators] = useState(true);
  const [editorInstance, setEditorInstance] = useState<monacoEditor.IStandaloneCodeEditor | null>(null);
  const editorRef = useRef<monacoEditor.IStandaloneCodeEditor | null>(null);
  const wrapperRef = useRef<HTMLDivElement>(null);
  const mousePositionRef = useRef<{ x: number; y: number }>({ x: 0, y: 0 });
  const onSaveRef = useRef(onSave);
  useEffect(() => { onSaveRef.current = onSave; }, [onSave]);
  const decorationsRef = useRef<monacoEditor.IEditorDecorationsCollection | null>(null);
  const diffDecorationsRef = useRef<monacoEditor.IEditorDecorationsCollection | null>(null);
  const disposablesRef = useRef<IDisposable[]>([]);
  const { toast } = useToast();
  const { setOpen: setCommandPanelOpen } = useCommandPanelOpen();

  const addComment = useDiffCommentsStore((state) => state.addComment);
  const removeComment = useDiffCommentsStore((state) => state.removeComment);
  const updateComment = useDiffCommentsStore((state) => state.updateComment);
  const editingCommentId = useDiffCommentsStore((state) => state.editingCommentId);
  const setEditingComment = useDiffCommentsStore((state) => state.setEditingComment);
  const comments = useFileComments(sessionId ?? '', path);

  // Gutter comment interactions (hover "+", click-to-select)
  const commentedLines = useCommentedLines(comments);

  const handleGutterSelectionComplete = useCallback(
    (params: { range: { start: number; end: number }; code: string }) => {
      setFormZoneRange({
        startLine: params.range.start,
        endLine: params.range.end,
        codeContent: params.code,
      });
    },
    []
  );

  const { clearGutterSelection } = useGutterComments(
    editorInstance,
    {
      enabled: enableComments && !!sessionId,
      commentedLines,
      onSelectionComplete: handleGutterSelectionComplete,
    }
  );

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
    lspClientManager.openDocument(lspSessionId, lspLanguage, documentUri, language, contentRef.current);
    return () => {
      lspClientManager.closeDocument(lspSessionId, lspLanguage, documentUri);
    };
    // Only run on mount/unmount and when LSP readiness changes — NOT on content changes
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [hasLspActive, lspSessionId, lspLanguage, documentUri, language]);

  // LSP document change: notify server of content changes via Monaco model events (not React props)
  const changeTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  useEffect(() => {
    const editor = editorRef.current;
    if (!editor || !hasLspActive || !lspSessionId || !lspLanguage) return;

    const model = editor.getModel();
    if (!model) return;

    const disposable = model.onDidChangeContent(() => {
      if (changeTimerRef.current) clearTimeout(changeTimerRef.current);
      changeTimerRef.current = setTimeout(() => {
        lspClientManager.changeDocument(lspSessionId, lspLanguage, documentUri, contentRef.current);
      }, 300);
    });

    return () => {
      if (changeTimerRef.current) clearTimeout(changeTimerRef.current);
      disposable.dispose();
    };
    // Re-run when LSP readiness or editor instance changes
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [hasLspActive, lspSessionId, lspLanguage, documentUri]);

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

  // Cmd/Ctrl+S to save — uses DOM keydown on wrapper instead of Monaco's addCommand
  // because addCommand registers globally and conflicts with multiple editor instances.
  useEffect(() => {
    const wrapper = wrapperRef.current;
    if (!wrapper) return;
    const handler = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === 's') {
        e.preventDefault();
        e.stopPropagation();
        onSaveRef.current();
      }
    };
    wrapper.addEventListener('keydown', handler);
    return () => wrapper.removeEventListener('keydown', handler);
  }, []);

  // Stable callback refs for ViewZone renders (avoids stale closures)
  const handleCommentSubmitRef = useRef((_annotation: string) => {});
  const handleCommentDeleteRef = useRef((_commentId: string) => {});
  const handleCommentUpdateRef = useRef((_commentId: string, _annotation: string) => {});

  // Handle editor mount
  const handleEditorDidMount: OnMount = useCallback(
    (editor, monaco) => {
      editorRef.current = editor;
      setEditorInstance(editor);
      decorationsRef.current = editor.createDecorationsCollection([]);
      diffDecorationsRef.current = editor.createDecorationsCollection([]);

      // Jump to pending cursor position (e.g. from LSP Go-to-Definition)
      const pendingPos = consumePendingCursorPosition(path);
      if (pendingPos) {
        editor.setPosition({ lineNumber: pendingPos.line, column: pendingPos.column });
        editor.revealLineInCenter(pendingPos.line);
      }

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

      // Gutter click on commented lines → toggle editing
      const glyphDisposable = editor.onMouseDown((e) => {
        if (e.target.type === 3 || e.target.type === 4) {
          const lineNumber = e.target.position?.lineNumber;
          if (!lineNumber) return;

          const { editingCommentId: currentEditing } = useDiffCommentsStore.getState();
          const fileComments = useDiffCommentsStore.getState().getCommentsForFile(sessionId ?? '', path);
          const lineComments = fileComments.filter(
            (c) => lineNumber >= c.startLine && lineNumber <= c.endLine
          );
          if (lineComments.length > 0) {
            const firstComment = lineComments[0];
            // Toggle: if already editing this comment, stop editing
            if (currentEditing === firstComment.id) {
              useDiffCommentsStore.getState().setEditingComment(null);
            } else {
              useDiffCommentsStore.getState().setEditingComment(firstComment.id);
            }
          }
        }
      });
      disposablesRef.current.push(glyphDisposable);

      // Override Monaco's built-in Cmd+K chord to open command panel
      editor.addCommand(
        monaco.KeyMod.CtrlCmd | monaco.KeyCode.KeyK,
        () => { setCommandPanelOpen(true); }
      );

      // Alt+Z to toggle word wrap
      editor.addCommand(
        monaco.KeyMod.Alt | monaco.KeyCode.KeyZ,
        () => { setWrapEnabled((prev) => !prev); }
      );
    },
    [path, enableComments, sessionId, setCommandPanelOpen, setWrapEnabled]
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
          lineNumberClassName: 'monaco-comment-line-number',
          linesDecorationsClassName: firstLines.has(lineNum)
            ? 'monaco-comment-bar-icon'
            : 'monaco-comment-bar',
        },
      });
    }

    decorationsRef.current.set(decorations);
  }, [comments, editorInstance]);

  // Diff gutter indicators — driven by Monaco model change events, not React props.
  const updateDiffDecorations = useCallback(() => {
    if (!diffDecorationsRef.current || !editorRef.current) return;

    if (!showDiffIndicators || !isDirty || !originalContent) {
      diffDecorationsRef.current.set([]);
      return;
    }

    const currentContent = contentRef.current;
    const changes = diffLines(originalContent, currentContent);
    const decorations: monacoEditor.IModelDeltaDecoration[] = [];
    let currentLine = 1;

    for (let i = 0; i < changes.length; i++) {
      const change = changes[i];
      const lineCount = change.count ?? 0;

      if (change.removed) {
        const next = changes[i + 1];
        if (next && next.added) {
          const addedLineCount = next.count ?? 0;
          for (let j = 0; j < addedLineCount; j++) {
            decorations.push({
              range: { startLineNumber: currentLine + j, startColumn: 1, endLineNumber: currentLine + j, endColumn: 1 },
              options: { isWholeLine: true, linesDecorationsClassName: 'monaco-diff-modified-gutter' },
            });
          }
          currentLine += addedLineCount;
          i++;
        } else {
          const indicatorLine = Math.max(1, currentLine - 1);
          decorations.push({
            range: { startLineNumber: indicatorLine, startColumn: 1, endLineNumber: indicatorLine, endColumn: 1 },
            options: { isWholeLine: true, linesDecorationsClassName: 'monaco-diff-deleted-gutter' },
          });
        }
      } else if (change.added) {
        for (let j = 0; j < lineCount; j++) {
          decorations.push({
            range: { startLineNumber: currentLine + j, startColumn: 1, endLineNumber: currentLine + j, endColumn: 1 },
            options: { isWholeLine: true, linesDecorationsClassName: 'monaco-diff-added-gutter' },
          });
        }
        currentLine += lineCount;
      } else {
        currentLine += lineCount;
      }
    }

    diffDecorationsRef.current.set(decorations);
  }, [originalContent, showDiffIndicators, isDirty]);

  // Update diff decorations on model content changes (debounced) and when settings change
  const diffTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  useEffect(() => {
    // Update immediately for settings changes (toggle, save)
    updateDiffDecorations();

    const editor = editorRef.current;
    if (!editor) return;
    const model = editor.getModel();
    if (!model) return;

    const disposable = model.onDidChangeContent(() => {
      if (diffTimerRef.current) clearTimeout(diffTimerRef.current);
      diffTimerRef.current = setTimeout(updateDiffDecorations, 150);
    });

    return () => {
      if (diffTimerRef.current) clearTimeout(diffTimerRef.current);
      disposable.dispose();
    };
  }, [updateDiffDecorations]);

  // Show floating button at end of selection
  useEffect(() => {
    const wrapper = wrapperRef.current;
    const editor = editorRef.current;
    if (!wrapper || !editor || !enableComments || !sessionId) return;

    const handleMouseUp = (e: MouseEvent) => {
      if ((e.target as HTMLElement).closest('.floating-comment-btn')) return;
      setTimeout(() => {
        if (!currentSelection) return;
        const sel = editor.getSelection();
        if (!sel || sel.isEmpty()) return;
        // Position at end of selection using editor coordinates
        // endPos is relative to the editor's DOM container
        const endPos = editor.getScrolledVisiblePosition({
          lineNumber: sel.endLineNumber,
          column: sel.endColumn,
        });
        if (!endPos) return;
        // getScrolledVisiblePosition returns coords relative to the editor DOM node,
        // which is inside the "relative" container where the button lives.
        setFloatingButtonPos({
          x: endPos.left,
          y: endPos.top + endPos.height,
        });
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

  // Cmd+I to open inline comment form
  useEffect(() => {
    const wrapper = wrapperRef.current;
    if (!wrapper || !enableComments || !sessionId) return;

    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === 'i') {
        if (!currentSelection) return;
        e.preventDefault();
        e.stopPropagation();
        setFormZoneRange({
          startLine: currentSelection.startLine,
          endLine: currentSelection.endLine,
          codeContent: currentSelection.text,
        });
        setFloatingButtonPos(null);
      }
    };

    wrapper.addEventListener('keydown', handleKeyDown, true);
    return () => wrapper.removeEventListener('keydown', handleKeyDown, true);
  }, [enableComments, sessionId, currentSelection]);

  // Escape to close inline forms
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        if (formZoneRange) {
          setFormZoneRange(null);
          clearGutterSelection();
        }
        if (editingCommentId) setEditingComment(null);
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [formZoneRange, editingCommentId, setEditingComment, clearGutterSelection]);

  // Click outside to close editing mode / new comment form
  useEffect(() => {
    if (!editingCommentId && !formZoneRange) return;
    const handleMouseDown = (e: MouseEvent) => {
      const target = e.target as HTMLElement;
      if (target.closest('[data-comment-zone]')) return;
      if (editingCommentId) setEditingComment(null);
      if (formZoneRange) {
        setFormZoneRange(null);
        clearGutterSelection();
      }
    };
    // Delay to avoid closing immediately from the click that opened it
    const timer = setTimeout(() => {
      document.addEventListener('mousedown', handleMouseDown);
    }, 0);
    return () => {
      clearTimeout(timer);
      document.removeEventListener('mousedown', handleMouseDown);
    };
  }, [editingCommentId, formZoneRange, setEditingComment, clearGutterSelection]);

  const handleChange: OnChange = useCallback(
    (value) => {
      if (value !== undefined) {
        contentRef.current = value;
        onChange(value);
      }
    },
    [onChange]
  );

  const handleFloatingButtonClick = useCallback(
    (e: React.MouseEvent) => {
      e.preventDefault();
      e.stopPropagation();
      if (!currentSelection) return;
      setFormZoneRange({
        startLine: currentSelection.startLine,
        endLine: currentSelection.endLine,
        codeContent: currentSelection.text,
      });
      setFloatingButtonPos(null);
    },
    [currentSelection]
  );

  // Comment submit for new comments
  const handleCommentSubmit = useCallback(
    (annotation: string) => {
      if (!formZoneRange || !sessionId) return;
      addComment(buildDiffComment({
        filePath: path,
        sessionId,
        startLine: formZoneRange.startLine,
        endLine: formZoneRange.endLine,
        side: 'additions',
        annotation,
        codeContent: formZoneRange.codeContent,
      }));
      setFormZoneRange(null);
      clearGutterSelection();

      // Clear editor selection
      const editor = editorRef.current;
      if (editor) {
        const pos = editor.getPosition();
        if (pos) editor.setSelection({ startLineNumber: pos.lineNumber, startColumn: pos.column, endLineNumber: pos.lineNumber, endColumn: pos.column });
      }

      toast({ title: 'Comment added', description: 'Your comment will be sent with your next message.' });
    },
    [formZoneRange, sessionId, path, addComment, clearGutterSelection, toast]
  );

  const handleDeleteComment = useCallback(
    (commentId: string) => {
      if (!sessionId) return;
      removeComment(sessionId, path, commentId);
      toast({ title: 'Comment deleted' });
    },
    [sessionId, path, removeComment, toast]
  );

  const handleUpdateComment = useCallback(
    (commentId: string, annotation: string) => {
      updateComment(commentId, { annotation });
      setEditingComment(null);
      toast({ title: 'Comment updated' });
    },
    [updateComment, setEditingComment, toast]
  );

  // Keep stable refs updated for ViewZone renders
  useEffect(() => { handleCommentSubmitRef.current = handleCommentSubmit; }, [handleCommentSubmit]);
  useEffect(() => { handleCommentDeleteRef.current = handleDeleteComment; }, [handleDeleteComment]);
  useEffect(() => { handleCommentUpdateRef.current = handleUpdateComment; }, [handleUpdateComment]);

  // Inline ViewZones for comments and new comment form
  useEditorViewZoneComments(
    editorInstance,
    [comments, formZoneRange, editingCommentId],
    (addZone) => {
      // Existing comments → compact inline display or edit form
      for (const comment of comments) {
        const isEditing = editingCommentId === comment.id;
        const node = isEditing ? (
          <div className="px-2 py-0.5" data-comment-zone>
            <CommentForm
              initialContent={comment.annotation}
              onSubmit={(c) => handleCommentUpdateRef.current(comment.id, c)}
              onCancel={() => setEditingComment(null)}
              isEditing
            />
          </div>
        ) : (
          <div className="px-2 py-0.5" data-comment-zone>
            <CommentDisplay
              comment={comment}
              onDelete={() => handleCommentDeleteRef.current(comment.id)}
              onEdit={() => setEditingComment(comment.id)}
              showCode={false}
              compact
            />
          </div>
        );
        addZone(comment.endLine, isEditing ? 120 : 32, node);
      }

      // New comment form
      if (formZoneRange) {
        addZone(
          formZoneRange.endLine, 120,
          <div className="px-2 py-1" data-comment-zone>
            <CommentForm
              onSubmit={(c) => handleCommentSubmitRef.current(c)}
              onCancel={() => {
                setFormZoneRange(null);
                clearGutterSelection();
              }}
            />
          </div>,
        );
      }
    },
  );

  // Diff stats — updated via Monaco model events, not React props
  const [diffStats, setDiffStats] = useState<{ additions: number; deletions: number } | null>(null);
  const computeDiffStats = useCallback(() => {
    if (!isDirty) { setDiffStats(null); return; }
    setDiffStats(computeLineDiffStats(originalContent, contentRef.current));
  }, [isDirty, originalContent]);

  const statsTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  useEffect(() => {
    computeDiffStats();

    const editor = editorRef.current;
    if (!editor) return;
    const model = editor.getModel();
    if (!model) return;

    const disposable = model.onDidChangeContent(() => {
      if (statsTimerRef.current) clearTimeout(statsTimerRef.current);
      statsTimerRef.current = setTimeout(computeDiffStats, 300);
    });

    return () => {
      if (statsTimerRef.current) clearTimeout(statsTimerRef.current);
      disposable.dispose();
    };
  }, [computeDiffStats]);

  return (
    <div ref={wrapperRef} className="flex h-full flex-col rounded-lg">
      {/* Editor header */}
      <PanelHeaderBarSplit
        left={
          <div className="flex items-center gap-2 text-xs text-muted-foreground">
            <span className="font-mono">{toRelativePath(path, worktreePath)}</span>
            {isDirty && diffStats && (
              <span className="text-xs text-yellow-500">
                {formatDiffStats(diffStats.additions, diffStats.deletions)}
              </span>
            )}
          </div>
        }
        right={
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
        }
      />

      {/* Monaco editor */}
      <div className="flex-1 overflow-hidden relative">
        <Editor
          height="100%"
          language={language}
          path={monacoPath}
          defaultValue={initialContent}
          theme={resolvedTheme === 'dark' ? 'kandev-dark' : 'kandev-light'}
          onChange={handleChange}
          onMount={handleEditorDidMount}
          keepCurrentModel
          options={{
            fontSize: EDITOR_FONT_SIZE,
            fontFamily: EDITOR_FONT_FAMILY,
            lineHeight: 18,
            minimap: { enabled: false },
            wordWrap: wrapEnabled ? 'on' : 'off',
            scrollBeyondLastLine: false,
            smoothScrolling: true,
            cursorSmoothCaretAnimation: 'on',
            glyphMargin: false,
            lineDecorationsWidth: 10,
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
        {floatingButtonPos && !formZoneRange && (
          <Button
            size="sm"
            variant="secondary"
            className="floating-comment-btn absolute z-50 gap-1.5 shadow-lg animate-in fade-in-0 zoom-in-95 duration-100 cursor-pointer"
            style={{ left: floatingButtonPos.x + 4, top: floatingButtonPos.y + 2 }}
            onMouseDown={(e) => e.stopPropagation()}
            onClick={handleFloatingButtonClick}
          >
            <IconMessagePlus className="h-3.5 w-3.5" />
            Comment
          </Button>
        )}
      </div>
    </div>
  );
}
