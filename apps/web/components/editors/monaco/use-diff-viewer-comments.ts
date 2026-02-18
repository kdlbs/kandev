import { useCallback, useEffect, useRef, useState } from 'react';
import type { editor as monacoEditor } from 'monaco-editor';
import type { DiffOnMount } from '@monaco-editor/react';
import type { DiffComment } from '@/lib/diff/types';
import { buildDiffComment, useCommentedLines, useCommentActions } from '@/lib/diff/comment-utils';
import { useDiffComments } from '@/components/diff/use-diff-comments';
import { useGutterComments } from '@/hooks/use-gutter-comments';
import type { ContextMenuState } from './diff-viewer-context-menu';
import { useViewZones } from './use-diff-view-zones';

/** Check if a line number falls within a changed region on the given side. */
function isLineInChanges(
  lineChanges: ReturnType<monacoEditor.IStandaloneDiffEditor['getLineChanges']>,
  lineNumber: number,
  side: 'original' | 'modified',
): boolean {
  if (!lineChanges) return false;
  for (const change of lineChanges) {
    const [start, end] = side === 'modified'
      ? [change.modifiedStartLineNumber, change.modifiedEndLineNumber]
      : [change.originalStartLineNumber, change.originalEndLineNumber];
    if (lineNumber >= start && lineNumber <= end) return true;
  }
  return false;
}

interface UseDiffViewerCommentsOpts {
  data: { filePath: string; diff?: string; newContent?: string; oldContent?: string };
  sessionId?: string;
  compact: boolean;
  onCommentAdd?: (comment: DiffComment) => void;
  onCommentDelete?: (commentId: string) => void;
  externalComments?: DiffComment[];
  onModifiedContentChange?: (filePath: string, content: string) => void;
}

// eslint-disable-next-line max-lines-per-function
export function useDiffViewerComments(opts: UseDiffViewerCommentsOpts) {
  const {
    data, sessionId, compact, onCommentAdd, onCommentDelete,
    externalComments, onModifiedContentChange,
  } = opts;

  const diffEditorRef = useRef<monacoEditor.IStandaloneDiffEditor | null>(null);
  const [modifiedEditor, setModifiedEditor] = useState<monacoEditor.ICodeEditor | null>(null);
  const [originalEditor, setOriginalEditor] = useState<monacoEditor.ICodeEditor | null>(null);
  const [contextMenu, setContextMenu] = useState<ContextMenuState>(null);
  const [showCommentForm, setShowCommentForm] = useState(false);
  const [selectedLineRange, setSelectedLineRange] = useState<{
    start: number; end: number; side: string;
  } | null>(null);

  const {
    comments: internalComments, addComment, removeComment,
    updateComment, editingCommentId, setEditingComment,
  } = useDiffComments({
    sessionId: sessionId || '',
    filePath: data.filePath,
    diff: data.diff,
    newContent: data.newContent,
    oldContent: data.oldContent,
  });

  const comments = externalComments || internalComments;

  // Gutter comment interactions
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
    enabled: gutterEnabled, commentedLines,
    onSelectionComplete: handleGutterSelect('additions'),
  });

  const { clearGutterSelection: clearOriginalGutter } = useGutterComments(originalEditor, {
    enabled: gutterEnabled, commentedLines,
    onSelectionComplete: handleGutterSelect('deletions'),
  });

  // Stable ref for onModifiedContentChange
  const onModifiedContentChangeRef = useRef(onModifiedContentChange);
  useEffect(() => { onModifiedContentChangeRef.current = onModifiedContentChange; }, [onModifiedContentChange]);

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
        const isChangedLine = isLineInChanges(diffEditor.getLineChanges(), lineNumber, side);
        setContextMenu({ x: e.event.posx, y: e.event.posy, lineNumber, side, isChangedLine, lineContent });
      },
    []
  );

  const handleDiffEditorMount: DiffOnMount = useCallback((editor) => {
    diffEditorRef.current = editor;
    const modEditor = editor.getModifiedEditor();
    const origEditor = editor.getOriginalEditor();
    setModifiedEditor(modEditor);
    setOriginalEditor(origEditor);
    if (!compact) {
      modEditor.onDidChangeModelContent(() => {
        onModifiedContentChangeRef.current?.(data.filePath, modEditor.getValue());
      });
    }
    modEditor.onContextMenu(handleContextMenuEvent(editor, modEditor, 'modified'));
    origEditor.onContextMenu(handleContextMenuEvent(editor, origEditor, 'original'));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [compact, data.filePath]);

  // Comment submission
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
          text: content,
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
    [selectedLineRange, sessionId, data.filePath, addComment, onCommentAdd, externalComments, clearModifiedGutter, clearOriginalGutter]
  );

  const { handleCommentDelete, handleCommentUpdate } = useCommentActions({
    removeComment, updateComment, setEditingComment,
    onCommentDelete, externalComments,
  });

  // Stable refs for ViewZone renders
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

  // Copy all changed lines
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

  return {
    diffEditorRef, modifiedEditor, originalEditor,
    contextMenu, setContextMenu, copyAllChangedLines,
    handleDiffEditorMount,
  };
}
