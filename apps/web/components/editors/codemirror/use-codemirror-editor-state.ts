import { useCallback, useEffect, useMemo, useState, useRef, type RefObject } from 'react';
import type { ReactCodeMirrorRef } from '@uiw/react-codemirror';
import { EditorView, gutter, GutterMarker, ViewPlugin, type ViewUpdate } from '@codemirror/view';
import type { Extension } from '@codemirror/state';
import { Decoration, type DecorationSet } from '@codemirror/view';
import { getCodeMirrorExtensionFromPath } from '@/lib/languages';
import { useCommentsStore } from '@/lib/state/slices/comments';
import { useDiffFileComments } from '@/hooks/domains/comments/use-diff-comments';
import type { DiffComment } from '@/lib/diff/types';
import { computeLineDiffStats } from '@/lib/diff';
import { useToast } from '@/components/toast-provider';
import { useCommandPanelOpen } from '@/lib/commands/command-registry';

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export type TextSelection = {
  text: string;
  startLine: number;
  endLine: number;
  position: { x: number; y: number };
} | null;

export type FloatingButtonPosition = {
  x: number;
  y: number;
} | null;

export type CommentViewState = {
  comments: DiffComment[];
  position: { x: number; y: number };
} | null;

// ---------------------------------------------------------------------------
// Comment decorations
// ---------------------------------------------------------------------------

function createCommentDecorations(view: EditorView, comments: DiffComment[]): DecorationSet {
  if (comments.length === 0) return Decoration.none;
  const decorations: Array<{ from: number; decoration: Decoration }> = [];
  const linesWithComments = new Set<number>();
  for (const comment of comments) {
    for (let line = comment.startLine; line <= comment.endLine; line++) {
      linesWithComments.add(line);
    }
  }
  for (const lineNum of linesWithComments) {
    if (lineNum > view.state.doc.lines) continue;
    const line = view.state.doc.line(lineNum);
    decorations.push({ from: line.from, decoration: Decoration.line({ class: 'cm-comment-line' }) });
  }
  decorations.sort((a, b) => a.from - b.from);
  return Decoration.set(decorations.map(d => d.decoration.range(d.from)));
}

class CommentGutterMarker extends GutterMarker {
  constructor(readonly lineComments: DiffComment[], readonly isFirstLine: boolean) {
    super();
  }

  toDOM() {
    const marker = document.createElement('div');
    marker.className = 'cm-comment-gutter-marker';
    marker.title = `${this.lineComments.length} comment${this.lineComments.length > 1 ? 's' : ''} - click to view`;
    if (this.isFirstLine) {
      const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
      svg.setAttribute('width', '12');
      svg.setAttribute('height', '12');
      svg.setAttribute('viewBox', '0 0 24 24');
      svg.setAttribute('fill', 'none');
      svg.setAttribute('stroke', 'rgba(99, 102, 241, 0.9)');
      svg.setAttribute('stroke-width', '2');
      svg.setAttribute('stroke-linecap', 'round');
      svg.setAttribute('stroke-linejoin', 'round');
      const path = document.createElementNS('http://www.w3.org/2000/svg', 'path');
      path.setAttribute('d', 'M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z');
      svg.appendChild(path);
      marker.appendChild(svg);
    }
    return marker;
  }

  eq(other: CommentGutterMarker) {
    return this.lineComments.length === other.lineComments.length && this.isFirstLine === other.isFirstLine;
  }
}

// ---------------------------------------------------------------------------
// Comment gutter extension builder
// ---------------------------------------------------------------------------

function buildCommentGutter(
  comments: DiffComment[],
  setCommentView: (state: CommentViewState) => void,
) {
  const commentsByLine = new Map<number, DiffComment[]>();
  const firstLines = new Set<number>();
  for (const comment of comments) {
    firstLines.add(comment.startLine);
    for (let lineNum = comment.startLine; lineNum <= comment.endLine; lineNum++) {
      const existing = commentsByLine.get(lineNum) || [];
      existing.push(comment);
      commentsByLine.set(lineNum, existing);
    }
  }
  return gutter({
    class: 'cm-comment-gutter',
    lineMarker: (view, line) => {
      const lineNum = view.state.doc.lineAt(line.from).number;
      const lineComments = commentsByLine.get(lineNum);
      if (lineComments && lineComments.length > 0) {
        return new CommentGutterMarker(lineComments, firstLines.has(lineNum));
      }
      return null;
    },
    domEventHandlers: {
      click: (view, line, event) => {
        const lineNum = view.state.doc.lineAt(line.from).number;
        const lineComments = commentsByLine.get(lineNum);
        if (lineComments && lineComments.length > 0) {
          event.preventDefault();
          event.stopPropagation();
          setCommentView({
            comments: lineComments,
            position: { x: (event as MouseEvent).clientX, y: (event as MouseEvent).clientY },
          });
          return true;
        }
        return false;
      },
    },
  });
}

// ---------------------------------------------------------------------------
// Static theme (re-exported for the component)
// ---------------------------------------------------------------------------

export const cmEditorTheme = EditorView.theme({
  '&': { backgroundColor: 'hsl(var(--background)) !important' },
  '.cm-gutters': { backgroundColor: 'hsl(var(--background)) !important', borderRight: 'none' },
  '.cm-comment-gutter': { width: '22px', cursor: 'pointer' },
  '.cm-comment-gutter .cm-gutterElement': {
    display: 'flex', alignItems: 'center', justifyContent: 'center', padding: '0',
  },
  '.cm-comment-gutter-marker': {
    display: 'flex', alignItems: 'center', justifyContent: 'center',
    width: '100%', height: '100%', cursor: 'pointer',
    backgroundColor: 'rgba(99, 102, 241, 0.2)',
  },
  '.cm-comment-gutter-marker:hover': { backgroundColor: 'rgba(99, 102, 241, 0.35)' },
  '.cm-comment-line': {
    backgroundColor: 'rgba(99, 102, 241, 0.15) !important',
    borderLeft: '3px solid rgba(99, 102, 241, 0.6)',
    marginLeft: '-3px', paddingLeft: '3px',
  },
});

// ---------------------------------------------------------------------------
// Hook: useCodeMirrorEditorState
// ---------------------------------------------------------------------------

type UseCodeMirrorEditorStateOpts = {
  path: string;
  originalContent: string;
  content: string;
  isDirty: boolean;
  isSaving: boolean;
  sessionId?: string;
  enableComments: boolean;
  onChange: (newContent: string) => void;
  onSave: () => void;
  wrapperRef: RefObject<HTMLDivElement | null>;
  editorRef: RefObject<ReactCodeMirrorRef | null>;
};

// eslint-disable-next-line max-lines-per-function
export function useCodeMirrorEditorState(opts: UseCodeMirrorEditorStateOpts) {
  const { path, originalContent, isDirty, isSaving, sessionId, enableComments, onChange, onSave, wrapperRef, editorRef } = opts;
  const [wrapEnabled, setWrapEnabled] = useState(true);
  const [textSelection, setTextSelection] = useState<TextSelection>(null);
  const [floatingButtonPos, setFloatingButtonPos] = useState<FloatingButtonPosition>(null);
  const [currentSelection, setCurrentSelection] = useState<{ text: string; startLine: number; endLine: number } | null>(null);
  const [commentView, setCommentView] = useState<CommentViewState>(null);
  const mousePositionRef = useRef<{ x: number; y: number }>({ x: 0, y: 0 });
  const contentRef = useRef(opts.content);
  const { toast } = useToast();
  const { setOpen: setCommandPanelOpen } = useCommandPanelOpen();

  const addComment = useCommentsStore((state) => state.addComment);
  const removeComment = useCommentsStore((state) => state.removeComment);
  const comments = useDiffFileComments(sessionId ?? '', path);
  const langExt = getCodeMirrorExtensionFromPath(path);

  // Comment decorations plugin
  const commentPlugin = useMemo(() => {
    return ViewPlugin.fromClass(
      class {
        decorations: DecorationSet;
        constructor(view: EditorView) { this.decorations = createCommentDecorations(view, comments); }
        update(update: ViewUpdate) {
          if (update.docChanged || update.viewportChanged) {
            this.decorations = createCommentDecorations(update.view, comments);
          }
        }
      },
      { decorations: (v) => v.decorations }
    );
  }, [comments]);

  const commentGutter = useMemo(() => buildCommentGutter(comments, setCommentView), [comments]);

  // Track mouse position
  useEffect(() => {
    const handleMouseMove = (e: MouseEvent) => { mousePositionRef.current = { x: e.clientX, y: e.clientY }; };
    document.addEventListener('mousemove', handleMouseMove);
    return () => document.removeEventListener('mousemove', handleMouseMove);
  }, []);

  // Show floating button after selection ends
  const handleSelectionEnd = useCallback(() => {
    if (!enableComments || !sessionId) return;
    const view = editorRef.current?.view;
    if (!view) return;
    const selection = view.state.selection.main;
    if (selection.empty) { setFloatingButtonPos(null); setCurrentSelection(null); return; }
    const selectedText = view.state.sliceDoc(selection.from, selection.to);
    if (!selectedText.trim()) { setFloatingButtonPos(null); setCurrentSelection(null); return; }
    const startLine = view.state.doc.lineAt(selection.from).number;
    const endLine = view.state.doc.lineAt(selection.to).number;
    setCurrentSelection({ text: selectedText, startLine, endLine });
    setFloatingButtonPos({ x: mousePositionRef.current.x, y: mousePositionRef.current.y });
  }, [enableComments, sessionId, editorRef]);

  // Mouse listeners for selection
  useEffect(() => {
    const wrapper = wrapperRef.current;
    if (!wrapper || !enableComments || !sessionId) return;
    const handleMouseUp = (e: MouseEvent) => {
      if ((e.target as HTMLElement).closest('.floating-comment-btn')) return;
      setTimeout(handleSelectionEnd, 10);
    };
    const handleMouseDown = (e: MouseEvent) => {
      if ((e.target as HTMLElement).closest('.floating-comment-btn')) return;
      setFloatingButtonPos(null);
    };
    wrapper.addEventListener('mouseup', handleMouseUp);
    wrapper.addEventListener('mousedown', handleMouseDown);
    return () => { wrapper.removeEventListener('mouseup', handleMouseUp); wrapper.removeEventListener('mousedown', handleMouseDown); };
  }, [enableComments, sessionId, handleSelectionEnd, wrapperRef]);

  // Clear floating button when selection cleared via keyboard
  const selectionUpdateExtension = useMemo(() => {
    return EditorView.updateListener.of((update) => {
      if (update.selectionSet) {
        const selection = update.state.selection.main;
        if (selection.empty) { setFloatingButtonPos(null); setCurrentSelection(null); }
      }
    });
  }, []);

  // Extensions
  const extensions: Extension[] = useMemo(() => {
    const exts: Extension[] = [EditorView.editable.of(true), cmEditorTheme, commentGutter, commentPlugin, selectionUpdateExtension];
    if (wrapEnabled) exts.push(EditorView.lineWrapping);
    if (langExt) exts.push(langExt);
    return exts;
  }, [langExt, wrapEnabled, commentGutter, commentPlugin, selectionUpdateExtension]);

  // Diff stats
  const [diffStats, setDiffStats] = useState<{ additions: number; deletions: number } | null>(null);
  const statsTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const computeDiffStats = useCallback(() => {
    if (!isDirty) { setDiffStats(null); return; }
    setDiffStats(computeLineDiffStats(originalContent, contentRef.current));
  }, [isDirty, originalContent]);
  useEffect(() => { computeDiffStats(); }, [computeDiffStats]);

  useEffect(() => {
    contentRef.current = opts.content;
  }, [opts.content]);

  // Cmd+I to open comment popover
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
  }, [enableComments, sessionId, currentSelection, floatingButtonPos, wrapperRef]);

  // Cmd+K for command panel
  useEffect(() => {
    const wrapper = wrapperRef.current;
    if (!wrapper) return;
    const handler = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
        e.preventDefault();
        e.stopPropagation();
        setCommandPanelOpen(true);
      }
    };
    wrapper.addEventListener('keydown', handler, true);
    return () => wrapper.removeEventListener('keydown', handler, true);
  }, [setCommandPanelOpen, wrapperRef]);

  // Alt+Z to toggle word wrap
  useEffect(() => {
    const wrapper = wrapperRef.current;
    if (!wrapper) return;
    const handler = (e: KeyboardEvent) => {
      if (e.altKey && !e.metaKey && !e.ctrlKey && e.code === 'KeyZ') {
        e.preventDefault();
        e.stopPropagation();
        setWrapEnabled((prev) => !prev);
      }
    };
    wrapper.addEventListener('keydown', handler, true);
    return () => wrapper.removeEventListener('keydown', handler, true);
  }, [wrapperRef]);

  // Cmd+S and Escape
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === 's') {
        e.preventDefault();
        if (isDirty && !isSaving) onSave();
      }
      if (e.key === 'Escape') {
        if (textSelection) setTextSelection(null);
        if (commentView) setCommentView(null);
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [isDirty, isSaving, onSave, textSelection, commentView]);

  const handleChange = useCallback(
    (value: string) => {
      contentRef.current = value;
      onChange(value);
      if (statsTimerRef.current) clearTimeout(statsTimerRef.current);
      statsTimerRef.current = setTimeout(computeDiffStats, 300);
    },
    [onChange, computeDiffStats]
  );

  const handleFloatingButtonClick = useCallback((e: React.MouseEvent) => {
    e.preventDefault();
    e.stopPropagation();
    if (!currentSelection || !floatingButtonPos) return;
    setTextSelection({ ...currentSelection, position: floatingButtonPos });
    setFloatingButtonPos(null);
  }, [currentSelection, floatingButtonPos]);

  const handleCommentSubmit = useCallback((annotation: string) => {
    if (!textSelection || !sessionId) return;
    const comment: DiffComment = {
      id: `${path}-${Date.now()}-${Math.random().toString(36).substr(2, 9)}`,
      source: 'diff', sessionId, filePath: path,
      startLine: textSelection.startLine, endLine: textSelection.endLine,
      side: 'additions', codeContent: textSelection.text, text: annotation,
      createdAt: new Date().toISOString(), status: 'pending',
    };
    addComment(comment);
    setTextSelection(null);
    const view = editorRef.current?.view;
    if (view) {
      view.dispatch({ selection: { anchor: view.state.selection.main.head } });
    }
    toast({ title: 'Comment added', description: 'Your comment will be sent with your next message.' });
  }, [textSelection, sessionId, path, addComment, toast, editorRef]);

  const handlePopoverClose = useCallback(() => { setTextSelection(null); }, []);

  const handleDeleteComment = useCallback((commentId: string) => {
    if (!sessionId) return;
    removeComment(commentId);
    if (commentView && commentView.comments.length <= 1) {
      setCommentView(null);
    } else if (commentView) {
      setCommentView({ ...commentView, comments: commentView.comments.filter(c => c.id !== commentId) });
    }
    toast({ title: 'Comment deleted' });
  }, [sessionId, removeComment, commentView, toast]);

  const handleCommentViewClose = useCallback(() => { setCommentView(null); }, []);

  return {
    wrapEnabled, setWrapEnabled, extensions, comments, diffStats,
    textSelection, floatingButtonPos, commentView,
    handleChange, handleFloatingButtonClick, handleCommentSubmit,
    handlePopoverClose, handleDeleteComment, handleCommentViewClose,
  };
}
