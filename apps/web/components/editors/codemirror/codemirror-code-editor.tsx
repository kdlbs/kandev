'use client';

import { useCallback, useEffect, useMemo, useState, useRef } from 'react';
import CodeMirror, { type ReactCodeMirrorRef } from '@uiw/react-codemirror';
import { EditorView, gutter, GutterMarker, ViewPlugin, type ViewUpdate } from '@codemirror/view';
import type { Extension } from '@codemirror/state';
import { Decoration, type DecorationSet } from '@codemirror/view';
import { getCodeMirrorExtensionFromPath } from '@/lib/languages';
import { Button } from '@kandev/ui/button';
import { IconDeviceFloppy, IconLoader2, IconTrash, IconTextWrap, IconTextWrapDisabled, IconMessagePlus } from '@tabler/icons-react';
import { Tooltip, TooltipContent, TooltipTrigger } from '@kandev/ui/tooltip';
import { formatDiffStats } from '@/lib/utils/file-diff';
import { toRelativePath } from '@/lib/utils';
import { vscodeDark } from '@uiw/codemirror-theme-vscode';
import { useDiffCommentsStore, useFileComments } from '@/lib/state/slices/diff-comments/diff-comments-slice';
import type { DiffComment } from '@/lib/diff/types';
import { useToast } from '@/components/toast-provider';
import { EditorCommentPopover } from '@/components/task/editor-comment-popover';
import { CommentViewPopover } from '@/components/task/comment-view-popover';

type FileEditorContentProps = {
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

// Create comment line decorations
function createCommentDecorations(
  view: EditorView,
  comments: DiffComment[]
): DecorationSet {
  if (comments.length === 0) {
    return Decoration.none;
  }

  const decorations: Array<{ from: number; decoration: Decoration }> = [];

  // Get all lines that have comments
  const linesWithComments = new Set<number>();
  for (const comment of comments) {
    for (let line = comment.startLine; line <= comment.endLine; line++) {
      linesWithComments.add(line);
    }
  }

  // Create line decorations for commented lines
  for (const lineNum of linesWithComments) {
    if (lineNum > view.state.doc.lines) continue;
    const line = view.state.doc.line(lineNum);
    decorations.push({
      from: line.from,
      decoration: Decoration.line({
        class: 'cm-comment-line',
      }),
    });
  }

  decorations.sort((a, b) => a.from - b.from);
  return Decoration.set(decorations.map(d => d.decoration.range(d.from)));
}

// Gutter marker for lines with comments - defined outside component to avoid re-creation
class CommentGutterMarker extends GutterMarker {
  constructor(
    readonly lineComments: DiffComment[],
    readonly isFirstLine: boolean
  ) {
    super();
  }

  toDOM() {
    const marker = document.createElement('div');
    marker.className = 'cm-comment-gutter-marker';
    marker.title = `${this.lineComments.length} comment${this.lineComments.length > 1 ? 's' : ''} - click to view`;

    // Add icon only on the first line of a comment
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

export function CodeMirrorCodeEditor({
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
}: FileEditorContentProps) {
  const [wrapEnabled, setWrapEnabled] = useState(true);
  const [textSelection, setTextSelection] = useState<TextSelection>(null);
  const [floatingButtonPos, setFloatingButtonPos] = useState<FloatingButtonPosition>(null);
  const [currentSelection, setCurrentSelection] = useState<{ text: string; startLine: number; endLine: number } | null>(null);
  const [commentView, setCommentView] = useState<CommentViewState>(null);
  const editorRef = useRef<ReactCodeMirrorRef>(null);
  const wrapperRef = useRef<HTMLDivElement>(null);
  const mousePositionRef = useRef<{ x: number; y: number }>({ x: 0, y: 0 });
  const { toast } = useToast();

  // Comment store
  const addComment = useDiffCommentsStore((state) => state.addComment);
  const removeComment = useDiffCommentsStore((state) => state.removeComment);
  const comments = useFileComments(sessionId ?? '', path);

  const langExt = getCodeMirrorExtensionFromPath(path);

  // Create the comment decoration plugin
  const commentPlugin = useMemo(() => {
    return ViewPlugin.fromClass(
      class {
        decorations: DecorationSet;

        constructor(view: EditorView) {
          this.decorations = createCommentDecorations(view, comments);
        }

        update(update: ViewUpdate) {
          if (update.docChanged || update.viewportChanged) {
            this.decorations = createCommentDecorations(update.view, comments);
          }
        }
      },
      { decorations: (v) => v.decorations }
    );
  }, [comments]);

  // Create comment gutter
  const commentGutter = useMemo(() => {
    // Build a map of line number to comments that include that line
    const commentsByLine = new Map<number, DiffComment[]>();
    // Track which lines are the first line of at least one comment
    const firstLines = new Set<number>();

    for (const comment of comments) {
      firstLines.add(comment.startLine);
      // Add to all lines in the range
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
  }, [comments]);

  // Track mouse position
  useEffect(() => {
    const handleMouseMove = (e: MouseEvent) => {
      mousePositionRef.current = { x: e.clientX, y: e.clientY };
    };
    document.addEventListener('mousemove', handleMouseMove);
    return () => document.removeEventListener('mousemove', handleMouseMove);
  }, []);

  // Show floating button after selection ends (on mouseup)
  const handleSelectionEnd = useCallback(() => {
    if (!enableComments || !sessionId) return;

    const view = editorRef.current?.view;
    if (!view) return;

    const selection = view.state.selection.main;
    if (selection.empty) {
      setFloatingButtonPos(null);
      setCurrentSelection(null);
      return;
    }

    const selectedText = view.state.sliceDoc(selection.from, selection.to);
    if (!selectedText.trim()) {
      setFloatingButtonPos(null);
      setCurrentSelection(null);
      return;
    }

    // Get line numbers
    const startLine = view.state.doc.lineAt(selection.from).number;
    const endLine = view.state.doc.lineAt(selection.to).number;

    // Use mouse position for the floating button (where cursor is)
    setCurrentSelection({ text: selectedText, startLine, endLine });
    setFloatingButtonPos({ x: mousePositionRef.current.x, y: mousePositionRef.current.y });
  }, [enableComments, sessionId]);

  // Listen for mouseup to show floating button after selection ends
  useEffect(() => {
    const wrapper = wrapperRef.current;
    if (!wrapper || !enableComments || !sessionId) return;

    const handleMouseUp = (e: MouseEvent) => {
      // Don't show if clicking on the floating button itself
      if ((e.target as HTMLElement).closest('.floating-comment-btn')) {
        return;
      }
      // Small delay to let selection finalize
      setTimeout(handleSelectionEnd, 10);
    };

    // Clear selection state when clicking (new selection starts)
    const handleMouseDown = (e: MouseEvent) => {
      // Don't clear if clicking on the floating button
      if ((e.target as HTMLElement).closest('.floating-comment-btn')) {
        return;
      }
      setFloatingButtonPos(null);
    };

    wrapper.addEventListener('mouseup', handleMouseUp);
    wrapper.addEventListener('mousedown', handleMouseDown);
    return () => {
      wrapper.removeEventListener('mouseup', handleMouseUp);
      wrapper.removeEventListener('mousedown', handleMouseDown);
    };
  }, [enableComments, sessionId, handleSelectionEnd]);

  // Clear floating button when selection is cleared via keyboard
  const selectionUpdateExtension = useMemo(() => {
    return EditorView.updateListener.of((update) => {
      if (update.selectionSet) {
        const selection = update.state.selection.main;
        if (selection.empty) {
          setFloatingButtonPos(null);
          setCurrentSelection(null);
        }
      }
    });
  }, []);

  const extensions: Extension[] = useMemo(() => {
    const exts: Extension[] = [
      EditorView.editable.of(true),
      EditorView.theme({
        '&': { backgroundColor: 'hsl(var(--background)) !important' },
        '.cm-gutters': { backgroundColor: 'hsl(var(--background)) !important', borderRight: 'none' },
        // Comment gutter
        '.cm-comment-gutter': {
          width: '22px',
          cursor: 'pointer',
        },
        '.cm-comment-gutter .cm-gutterElement': {
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          padding: '0',
        },
        // Marker styles - background color on the marker itself (indigo)
        '.cm-comment-gutter-marker': {
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          width: '100%',
          height: '100%',
          cursor: 'pointer',
          backgroundColor: 'rgba(99, 102, 241, 0.2)',
        },
        '.cm-comment-gutter-marker:hover': {
          backgroundColor: 'rgba(99, 102, 241, 0.35)',
        },
        // Comment line highlight with left border (indigo)
        '.cm-comment-line': {
          backgroundColor: 'rgba(99, 102, 241, 0.15) !important',
          borderLeft: '3px solid rgba(99, 102, 241, 0.6)',
          marginLeft: '-3px',
          paddingLeft: '3px',
        },
      }),
      commentGutter,
      commentPlugin,
      selectionUpdateExtension,
    ];
    if (wrapEnabled) {
      exts.push(EditorView.lineWrapping);
    }
    if (langExt) {
      exts.push(langExt);
    }
    return exts;
  }, [langExt, wrapEnabled, commentGutter, commentPlugin, selectionUpdateExtension]);

  // Calculate diff stats when dirty
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

      if (origLine === undefined && currLine !== undefined) {
        additions++;
      } else if (origLine !== undefined && currLine === undefined) {
        deletions++;
      } else if (origLine !== currLine) {
        additions++;
        deletions++;
      }
    }

    return { additions, deletions };
  }, [isDirty, content, originalContent]);

  // Handle Cmd+I to open comment popover
  useEffect(() => {
    const wrapper = wrapperRef.current;
    if (!wrapper || !enableComments || !sessionId) return;

    const handleKeyDown = (e: KeyboardEvent) => {
      // Check for Cmd+I (Mac) or Ctrl+I (Windows/Linux)
      if ((e.metaKey || e.ctrlKey) && e.key === 'i') {
        if (!currentSelection || !floatingButtonPos) return;

        e.preventDefault();
        e.stopPropagation();

        setTextSelection({
          ...currentSelection,
          position: floatingButtonPos,
        });
        setFloatingButtonPos(null);
      }
    };

    wrapper.addEventListener('keydown', handleKeyDown, true);
    return () => wrapper.removeEventListener('keydown', handleKeyDown, true);
  }, [enableComments, sessionId, currentSelection, floatingButtonPos]);

  // Handle Cmd/Ctrl+S keyboard shortcut and Escape
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === 's') {
        e.preventDefault();
        if (isDirty && !isSaving) {
          onSave();
        }
      }
      // Escape to close popovers
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
      onChange(value);
    },
    [onChange]
  );

  // Open comment popover from floating button
  const handleFloatingButtonClick = useCallback((e: React.MouseEvent) => {
    e.preventDefault();
    e.stopPropagation();

    if (!currentSelection || !floatingButtonPos) return;

    setTextSelection({
      ...currentSelection,
      position: floatingButtonPos,
    });
    setFloatingButtonPos(null);
  }, [currentSelection, floatingButtonPos]);

  // Submit comment
  const handleCommentSubmit = useCallback((annotation: string) => {
    if (!textSelection || !sessionId) return;

    const { text, startLine, endLine } = textSelection;

    const comment: DiffComment = {
      id: `${path}-${Date.now()}-${Math.random().toString(36).substr(2, 9)}`,
      sessionId,
      filePath: path,
      startLine,
      endLine,
      side: 'additions',
      codeContent: text,
      annotation,
      createdAt: new Date().toISOString(),
      status: 'pending',
    };

    addComment(comment);
    setTextSelection(null);

    // Clear selection in editor
    const view = editorRef.current?.view;
    if (view) {
      view.dispatch({
        selection: { anchor: view.state.selection.main.head },
      });
    }

    toast({
      title: 'Comment added',
      description: 'Your comment will be sent with your next message.',
    });
  }, [textSelection, sessionId, path, addComment, toast]);

  // Close popover
  const handlePopoverClose = useCallback(() => {
    setTextSelection(null);
  }, []);

  // Delete comment
  const handleDeleteComment = useCallback((commentId: string) => {
    if (!sessionId) return;
    removeComment(sessionId, path, commentId);

    // Close the view if no more comments
    if (commentView && commentView.comments.length <= 1) {
      setCommentView(null);
    } else if (commentView) {
      setCommentView({
        ...commentView,
        comments: commentView.comments.filter(c => c.id !== commentId),
      });
    }

    toast({
      title: 'Comment deleted',
    });
  }, [sessionId, path, removeComment, commentView, toast]);

  // Close comment view
  const handleCommentViewClose = useCallback(() => {
    setCommentView(null);
  }, []);

  return (
    <div ref={wrapperRef} className="flex h-full flex-col rounded-lg">
      {/* Editor header with save button */}
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
                  ({navigator.platform.includes('Mac') ? '⌘' : 'Ctrl'}+S)
                </span>
              </>
            )}
          </Button>
        </div>
      </div>

      {/* CodeMirror editor */}
      <div className="flex-1 overflow-hidden relative">
        <CodeMirror
          ref={editorRef}
          value={content}
          height="100%"
          theme={vscodeDark}
          extensions={extensions}
          onChange={handleChange}
          basicSetup={{
            lineNumbers: true,
            foldGutter: true,
            highlightActiveLine: true,
            highlightSelectionMatches: true,
            searchKeymap: true,
          }}
          className="h-full overflow-auto text-xs"
        />

        {/* Floating "Add comment" button when text is selected */}
        {floatingButtonPos && !textSelection && (
          <Button
            size="sm"
            variant="secondary"
            className="floating-comment-btn fixed z-50 gap-1.5 shadow-lg animate-in fade-in-0 zoom-in-95 duration-100 cursor-pointer"
            style={{
              left: floatingButtonPos.x + 8,
              top: floatingButtonPos.y + 8,
            }}
            onMouseDown={(e) => e.stopPropagation()}
            onClick={handleFloatingButtonClick}
          >
            <IconMessagePlus className="h-3.5 w-3.5" />
            Comment
          </Button>
        )}

        {/* Comment popover for adding new comment */}
        {textSelection && (
          <EditorCommentPopover
            selectedText={textSelection.text}
            lineRange={{ start: textSelection.startLine, end: textSelection.endLine }}
            position={textSelection.position}
            onSubmit={handleCommentSubmit}
            onClose={handlePopoverClose}
          />
        )}

        {/* Comment view popover for viewing/deleting existing comments */}
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
