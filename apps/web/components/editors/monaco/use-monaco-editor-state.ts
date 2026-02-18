import { useCallback, useEffect, useState, useRef, type RefObject } from 'react';
import type { OnMount, OnChange } from '@monaco-editor/react';
import type { editor as monacoEditor, IDisposable } from 'monaco-editor';
import { useDiffCommentsStore, useFileComments } from '@/lib/state/slices/diff-comments/diff-comments-slice';
import { buildDiffComment, useCommentedLines } from '@/lib/diff/comment-utils';
import { useToast } from '@/components/toast-provider';
import { useCommandPanelOpen } from '@/lib/commands/command-registry';
import { useGutterComments } from '@/hooks/use-gutter-comments';
import { consumePendingCursorPosition } from '@/hooks/use-file-editors';

export type FormZoneRange = {
  startLine: number;
  endLine: number;
  codeContent: string;
} | null;

export type FloatingButtonPosition = {
  x: number;
  y: number;
} | null;

// ---------------------------------------------------------------------------
// Hook options
// ---------------------------------------------------------------------------

type UseMonacoEditorStateOpts = {
  path: string;
  enableComments: boolean;
  sessionId?: string;
  wrapperRef: RefObject<HTMLDivElement | null>;
  onChange: (newContent: string) => void;
  onSave: () => void;
  contentRef: RefObject<string>;
};

// ---------------------------------------------------------------------------
// useMonacoEditorComments — comment state, gutter, keyboard, ViewZone refs
// ---------------------------------------------------------------------------

// eslint-disable-next-line max-lines-per-function
export function useMonacoEditorComments(opts: UseMonacoEditorStateOpts) {
  const { path, enableComments, sessionId, wrapperRef, onChange, onSave, contentRef } = opts;

  const [wrapEnabled, setWrapEnabled] = useState(true);
  const [formZoneRange, setFormZoneRange] = useState<FormZoneRange>(null);
  const [floatingButtonPos, setFloatingButtonPos] = useState<FloatingButtonPosition>(null);
  const [currentSelection, setCurrentSelection] = useState<{ text: string; startLine: number; endLine: number } | null>(null);
  const [showDiffIndicators, setShowDiffIndicators] = useState(true);
  const [editorInstance, setEditorInstance] = useState<monacoEditor.IStandaloneCodeEditor | null>(null);
  const editorRef = useRef<monacoEditor.IStandaloneCodeEditor | null>(null);
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
  const commentedLines = useCommentedLines(comments);

  const handleGutterSelectionComplete = useCallback(
    (params: { range: { start: number; end: number }; code: string }) => {
      setFormZoneRange({ startLine: params.range.start, endLine: params.range.end, codeContent: params.code });
    }, [],
  );

  const { clearGutterSelection } = useGutterComments(
    editorInstance,
    { enabled: enableComments && !!sessionId, commentedLines, onSelectionComplete: handleGutterSelectionComplete },
  );

  // Stable callback refs for ViewZone renders (avoids stale closures)
  const handleCommentSubmitRef = useRef((() => {}) as (annotation: string) => void);
  const handleCommentDeleteRef = useRef((() => {}) as (commentId: string) => void);
  const handleCommentUpdateRef = useRef((() => {}) as (commentId: string, annotation: string) => void);

  // Editor mount
  const handleEditorDidMount: OnMount = useCallback(
    (editor, monaco) => {
      editorRef.current = editor;
      setEditorInstance(editor);
      decorationsRef.current = editor.createDecorationsCollection([]);
      diffDecorationsRef.current = editor.createDecorationsCollection([]);
      const pendingPos = consumePendingCursorPosition(path);
      if (pendingPos) {
        editor.setPosition({ lineNumber: pendingPos.line, column: pendingPos.column });
        editor.revealLineInCenter(pendingPos.line);
      }
      if (enableComments && sessionId) {
        disposablesRef.current.push(
          editor.onDidChangeCursorSelection(() => {
            const selection = editor.getSelection();
            if (!selection || selection.isEmpty()) { setCurrentSelection(null); return; }
            const model = editor.getModel();
            if (!model) return;
            const text = model.getValueInRange(selection);
            if (!text.trim()) { setCurrentSelection(null); return; }
            setCurrentSelection({ text, startLine: selection.startLineNumber, endLine: selection.endLineNumber });
          }),
        );
      }
      disposablesRef.current.push(
        editor.onMouseDown((e) => {
          if (e.target.type !== 3 && e.target.type !== 4) return;
          const lineNumber = e.target.position?.lineNumber;
          if (!lineNumber) return;
          const storeState = useDiffCommentsStore.getState();
          const fileComments = storeState.getCommentsForFile(sessionId ?? '', path);
          const lineComments = fileComments.filter((c) => lineNumber >= c.startLine && lineNumber <= c.endLine);
          if (lineComments.length > 0) {
            const id = lineComments[0].id;
            storeState.setEditingComment(storeState.editingCommentId === id ? null : id);
          }
        }),
      );
      editor.addCommand(monaco.KeyMod.CtrlCmd | monaco.KeyCode.KeyK, () => { setCommandPanelOpen(true); });
      editor.addCommand(monaco.KeyMod.Alt | monaco.KeyCode.KeyZ, () => { setWrapEnabled((prev) => !prev); });
    },
    [path, enableComments, sessionId, setCommandPanelOpen, setWrapEnabled],
  );

  // Cleanup
  useEffect(() => {
    return () => { for (const d of disposablesRef.current) d.dispose(); disposablesRef.current = []; };
  }, []);

  // Track mouse position
  useEffect(() => {
    const handleMouseMove = (e: MouseEvent) => { mousePositionRef.current = { x: e.clientX, y: e.clientY }; };
    document.addEventListener('mousemove', handleMouseMove);
    return () => document.removeEventListener('mousemove', handleMouseMove);
  }, []);

  // Cmd/Ctrl+S to save
  useEffect(() => {
    const wrapper = wrapperRef.current;
    if (!wrapper) return;
    const handler = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === 's') { e.preventDefault(); e.stopPropagation(); onSaveRef.current(); }
    };
    wrapper.addEventListener('keydown', handler);
    return () => wrapper.removeEventListener('keydown', handler);
  }, [wrapperRef]);

  // Comment decorations
  useEffect(() => {
    if (!decorationsRef.current || !editorRef.current) return;
    const decorations: monacoEditor.IModelDeltaDecoration[] = [];
    const linesWithComments = new Set<number>();
    const firstLines = new Set<number>();
    for (const comment of comments) {
      firstLines.add(comment.startLine);
      for (let line = comment.startLine; line <= comment.endLine; line++) linesWithComments.add(line);
    }
    for (const lineNum of linesWithComments) {
      decorations.push({
        range: { startLineNumber: lineNum, startColumn: 1, endLineNumber: lineNum, endColumn: 1 },
        options: {
          isWholeLine: true, className: 'monaco-comment-line', lineNumberClassName: 'monaco-comment-line-number',
          linesDecorationsClassName: firstLines.has(lineNum) ? 'monaco-comment-bar-icon' : 'monaco-comment-bar',
        },
      });
    }
    decorationsRef.current.set(decorations);
  }, [comments, editorInstance]);

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
        const endPos = editor.getScrolledVisiblePosition({ lineNumber: sel.endLineNumber, column: sel.endColumn });
        if (!endPos) return;
        setFloatingButtonPos({ x: endPos.left, y: endPos.top + endPos.height });
      }, 10);
    };
    const handleMouseDown = (e: MouseEvent) => {
      if ((e.target as HTMLElement).closest('.floating-comment-btn')) return;
      setFloatingButtonPos(null);
    };
    wrapper.addEventListener('mouseup', handleMouseUp);
    wrapper.addEventListener('mousedown', handleMouseDown);
    return () => { wrapper.removeEventListener('mouseup', handleMouseUp); wrapper.removeEventListener('mousedown', handleMouseDown); };
  }, [enableComments, sessionId, currentSelection, wrapperRef]);

  // Cmd+I to open inline comment form
  useEffect(() => {
    const wrapper = wrapperRef.current;
    if (!wrapper || !enableComments || !sessionId) return;
    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === 'i') {
        if (!currentSelection) return;
        e.preventDefault(); e.stopPropagation();
        setFormZoneRange({ startLine: currentSelection.startLine, endLine: currentSelection.endLine, codeContent: currentSelection.text });
        setFloatingButtonPos(null);
      }
    };
    wrapper.addEventListener('keydown', handleKeyDown, true);
    return () => wrapper.removeEventListener('keydown', handleKeyDown, true);
  }, [enableComments, sessionId, currentSelection, wrapperRef]);

  // Escape to close inline forms
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        if (formZoneRange) { setFormZoneRange(null); clearGutterSelection(); }
        if (editingCommentId) setEditingComment(null);
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [formZoneRange, editingCommentId, setEditingComment, clearGutterSelection]);

  // Click outside to close editing / new comment form
  useEffect(() => {
    if (!editingCommentId && !formZoneRange) return;
    const handleMouseDown = (e: MouseEvent) => {
      if ((e.target as HTMLElement).closest('[data-comment-zone]')) return;
      if (editingCommentId) setEditingComment(null);
      if (formZoneRange) { setFormZoneRange(null); clearGutterSelection(); }
    };
    const timer = setTimeout(() => { document.addEventListener('mousedown', handleMouseDown); }, 0);
    return () => { clearTimeout(timer); document.removeEventListener('mousedown', handleMouseDown); };
  }, [editingCommentId, formZoneRange, setEditingComment, clearGutterSelection]);

  const handleChange: OnChange = useCallback(
    (value) => { if (value !== undefined) { contentRef.current = value; onChange(value); } },
    [onChange, contentRef],
  );

  const handleFloatingButtonClick = useCallback(
    (e: React.MouseEvent) => {
      e.preventDefault(); e.stopPropagation();
      if (!currentSelection) return;
      setFormZoneRange({ startLine: currentSelection.startLine, endLine: currentSelection.endLine, codeContent: currentSelection.text });
      setFloatingButtonPos(null);
    },
    [currentSelection],
  );

  const handleCommentSubmit = useCallback(
    (annotation: string) => {
      if (!formZoneRange || !sessionId) return;
      addComment(buildDiffComment({
        filePath: path, sessionId, startLine: formZoneRange.startLine, endLine: formZoneRange.endLine,
        side: 'additions', annotation, codeContent: formZoneRange.codeContent,
      }));
      setFormZoneRange(null); clearGutterSelection();
      const editor = editorRef.current;
      if (editor) {
        const pos = editor.getPosition();
        if (pos) editor.setSelection({ startLineNumber: pos.lineNumber, startColumn: pos.column, endLineNumber: pos.lineNumber, endColumn: pos.column });
      }
      toast({ title: 'Comment added', description: 'Your comment will be sent with your next message.' });
    },
    [formZoneRange, sessionId, path, addComment, clearGutterSelection, toast],
  );

  const handleDeleteComment = useCallback(
    (commentId: string) => {
      if (!sessionId) return;
      removeComment(sessionId, path, commentId);
      toast({ title: 'Comment deleted' });
    },
    [sessionId, path, removeComment, toast],
  );

  const handleUpdateComment = useCallback(
    (commentId: string, annotation: string) => {
      updateComment(commentId, { annotation }); setEditingComment(null);
      toast({ title: 'Comment updated' });
    },
    [updateComment, setEditingComment, toast],
  );

  // Keep stable refs updated for ViewZone renders
  useEffect(() => { handleCommentSubmitRef.current = handleCommentSubmit; }, [handleCommentSubmit]);
  useEffect(() => { handleCommentDeleteRef.current = handleDeleteComment; }, [handleDeleteComment]);
  useEffect(() => { handleCommentUpdateRef.current = handleUpdateComment; }, [handleUpdateComment]);

  return {
    wrapEnabled, setWrapEnabled, showDiffIndicators, setShowDiffIndicators,
    editorInstance, editorRef, decorationsRef, diffDecorationsRef,
    formZoneRange, setFormZoneRange, floatingButtonPos, editingCommentId, setEditingComment,
    comments, clearGutterSelection,
    handleEditorDidMount, handleChange, handleFloatingButtonClick,
    handleCommentSubmitRef, handleCommentDeleteRef, handleCommentUpdateRef,
  };
}
