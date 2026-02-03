'use client';

import { useEffect, useRef, useState, useCallback } from 'react';
import { useTheme } from 'next-themes';
import { Editor, rootCtx, defaultValueCtx, editorViewCtx } from '@milkdown/core';
import { nord } from '@milkdown/theme-nord';
import { commonmark, codeBlockSchema } from '@milkdown/preset-commonmark';
import { gfm } from '@milkdown/preset-gfm';
import { listener, listenerCtx } from '@milkdown/plugin-listener';
import { Milkdown, MilkdownProvider, useEditor } from '@milkdown/react';
import { Plugin, PluginKey } from '@milkdown/prose/state';
import { Decoration, DecorationSet } from '@milkdown/prose/view';
import { $prose, $view } from '@milkdown/utils';
import type { Node as ProseMirrorNode } from '@milkdown/prose/model';
import mermaid from 'mermaid';

export type TextSelection = {
  text: string;
  position: { x: number; y: number };
};

export type CommentHighlight = {
  id: string;
  selectedText: string;
  comment: string;
};

type MilkdownEditorProps = {
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  onSelectionChange?: (selection: TextSelection | null) => void;
  /** Called when selection ends (mouseup) with selected text - for showing floating button */
  onSelectionEnd?: (selection: TextSelection | null) => void;
  comments?: CommentHighlight[];
  onCommentClick?: (id: string, position: { x: number; y: number }) => void;
};

// Minimum text length required for comment highlighting
const MIN_COMMENT_TEXT_LENGTH = 3;

// Initialize mermaid with default config
let mermaidInitialized = false;
function initMermaid(theme: 'dark' | 'light' = 'dark') {
  mermaid.initialize({
    startOnLoad: false,
    theme: theme === 'dark' ? 'dark' : 'default',
    securityLevel: 'loose',
  });
  mermaidInitialized = true;
}

// Unique ID counter for mermaid diagrams
let mermaidIdCounter = 0;

// Create mermaid node view for code blocks using $view
const mermaidCodeBlockView = $view(codeBlockSchema.node, () => {
  return (node: ProseMirrorNode) => {
    const language = node.attrs.language as string;

    // If not mermaid, return null to use default rendering
    if (language !== 'mermaid') {
      // Create default code block rendering
      const pre = document.createElement('pre');
      const code = document.createElement('code');
      code.textContent = node.textContent;
      if (language) {
        code.className = `language-${language}`;
      }
      pre.appendChild(code);
      return {
        dom: pre,
        contentDOM: code,
      };
    }

    // Track current content for comparison in update
    let currentContent = node.textContent;

    // Create container for mermaid diagram
    const dom = document.createElement('div');
    dom.className = 'mermaid-container';

    // Initialize mermaid if not already done
    if (!mermaidInitialized) {
      initMermaid();
    }

    // Render the mermaid diagram
    const id = `mermaid-${++mermaidIdCounter}`;
    mermaid.render(id, currentContent).then(({ svg }: { svg: string }) => {
      dom.innerHTML = svg;
    }).catch((err: Error) => {
      dom.innerHTML = `<pre class="mermaid-error">Error rendering diagram: ${err.message}</pre>`;
    });

    return {
      dom,
      contentDOM: null,
      update: (updatedNode: ProseMirrorNode) => {
        if (updatedNode.type.name !== 'code_block') return false;
        const updatedLang = updatedNode.attrs.language as string;
        if (updatedLang !== 'mermaid') return false;
        const newContent = updatedNode.textContent;
        if (newContent !== currentContent) {
          currentContent = newContent; // Update tracked content
          const newId = `mermaid-${++mermaidIdCounter}`;
          mermaid.render(newId, newContent).then(({ svg }: { svg: string }) => {
            dom.innerHTML = svg;
          }).catch((err: Error) => {
            dom.innerHTML = `<pre class="mermaid-error">Error rendering diagram: ${err.message}</pre>`;
          });
        }
        return true;
      },
    };
  };
});

// Plugin key for managing comment highlights
const commentHighlightPluginKey = new PluginKey<{
  deco: DecorationSet;
  comments: CommentHighlight[];
}>('comment-highlight');

// Plugin key for placeholder
const placeholderPluginKey = new PluginKey<DecorationSet>('placeholder');

// Create placeholder plugin that shows text when editor is empty
function createPlaceholderPlugin(placeholder: string) {
  return $prose(() => {
    return new Plugin({
      key: placeholderPluginKey,
      props: {
        decorations(state) {
          const doc = state.doc;
          // Check if document is empty (only has one empty paragraph)
          if (doc.childCount === 1 && doc.firstChild?.type.name === 'paragraph' && doc.firstChild.content.size === 0) {
            return DecorationSet.create(doc, [
              Decoration.widget(1, () => {
                const span = document.createElement('span');
                span.className = 'editor-placeholder';
                span.textContent = placeholder;
                return span;
              }, { side: 0 }),
            ]);
          }
          return DecorationSet.empty;
        },
      },
    });
  });
}

// Helper to get plain text content and position mapping from a document range
function getTextWithPositions(doc: Parameters<typeof DecorationSet.create>[0]): { text: string; positions: number[] } {
  let text = '';
  const positions: number[] = []; // Maps text index to document position

  doc.descendants((node, pos) => {
    if (node.isText && node.text) {
      for (let i = 0; i < node.text.length; i++) {
        positions.push(pos + i);
        text += node.text[i];
      }
    } else if (node.type.name === 'hardbreak') {
      // Handle hardbreak as a space/newline
      positions.push(-1);
      text += ' ';
    } else if (node.isBlock && text.length > 0 && !text.endsWith('\n')) {
      // Add newline between blocks
      positions.push(-1); // -1 indicates a synthetic newline
      text += '\n';
    }
  });

  return { text, positions };
}

// Normalize text for comparison (collapse whitespace, normalize newlines)
function normalizeForSearch(text: string): string {
  return text
    .replace(/\r\n/g, '\n')
    .replace(/[\t ]+/g, ' ')
    .replace(/\n+/g, '\n')
    .trim()
    .toLowerCase();
}

// Create decorations for comment highlights
function createCommentDecorations(
  doc: Parameters<typeof DecorationSet.create>[0],
  comments: CommentHighlight[]
): DecorationSet {
  const decorations: Decoration[] = [];

  if (comments.length === 0) {
    return DecorationSet.empty;
  }

  // Get the full text content with position mapping
  const { text: fullText, positions } = getTextWithPositions(doc);
  const normalizedFullText = normalizeForSearch(fullText);

  comments.forEach((comment) => {
    const searchText = normalizeForSearch(comment.selectedText);

    if (!searchText || searchText.length < MIN_COMMENT_TEXT_LENGTH) {
      return;
    }

    // Find the text in the normalized full document text
    const normalizedIndex = normalizedFullText.indexOf(searchText);

    if (normalizedIndex === -1) {
      // Try with just the first line as fallback
      const firstLine = normalizeForSearch(comment.selectedText.split('\n')[0]);
      if (firstLine.length >= MIN_COMMENT_TEXT_LENGTH) {
        const fallbackIndex = normalizedFullText.indexOf(firstLine);
        if (fallbackIndex !== -1) {
          createDecorationForRange(fallbackIndex, firstLine.length, comment, fullText, positions, decorations);
        }
      }
      return;
    }

    createDecorationForRange(normalizedIndex, searchText.length, comment, fullText, positions, decorations);
  });

  return DecorationSet.create(doc, decorations);
}

// Helper to create decoration(s) for a text range, handling multi-line selections
function createDecorationForRange(
  normalizedIndex: number,
  length: number,
  comment: CommentHighlight,
  fullText: string,
  positions: number[],
  decorations: Decoration[]
) {
  // Map normalized index back to original text index
  const normalizedFullText = normalizeForSearch(fullText);

  const normalizedChars = normalizedFullText.split('');
  const origChars = fullText.split('');

  // Map from normalized position to original position
  const normToOrig: number[] = [];
  let oi = 0;
  for (let ni = 0; ni < normalizedChars.length; ni++) {
    // Skip whitespace differences in original
    while (oi < origChars.length &&
           origChars[oi].toLowerCase() !== normalizedChars[ni] &&
           /\s/.test(origChars[oi])) {
      oi++;
    }
    normToOrig[ni] = oi;
    oi++;
  }

  const startOrig = normToOrig[normalizedIndex] ?? 0;
  const endOrig = normToOrig[normalizedIndex + length - 1] ?? startOrig;

  // Now create decorations for each contiguous range (split by newlines/block boundaries)
  let rangeStart = -1;

  for (let i = startOrig; i <= endOrig && i < positions.length; i++) {
    const pos = positions[i];

    if (pos === -1) {
      // Hit a block boundary - close current range if open
      if (rangeStart !== -1) {
        const fromPos = positions[rangeStart];
        const toPos = positions[i - 1] + 1;
        if (fromPos >= 0 && toPos > fromPos) {
          decorations.push(Decoration.inline(fromPos, toPos, {
            class: 'comment-highlight',
            'data-comment-id': comment.id,
            title: comment.comment,
          }));
        }
        rangeStart = -1;
      }
    } else {
      // Valid position - start range if not started
      if (rangeStart === -1) {
        rangeStart = i;
      }
    }
  }

  // Close final range
  if (rangeStart !== -1 && rangeStart < positions.length) {
    const fromPos = positions[rangeStart];
    const lastValidIndex = Math.min(endOrig, positions.length - 1);
    const toPos = positions[lastValidIndex] + 1;
    if (fromPos >= 0 && toPos > fromPos) {
      decorations.push(Decoration.inline(fromPos, toPos, {
        class: 'comment-highlight',
        'data-comment-id': comment.id,
        title: comment.comment,
      }));
    }
  }
}

// Create the comment highlight plugin
function createCommentHighlightPlugin(initialComments: CommentHighlight[]) {
  return $prose(() => {
    return new Plugin({
      key: commentHighlightPluginKey,
      state: {
        init(_, state) {
          return {
            deco: createCommentDecorations(state.doc, initialComments),
            comments: initialComments,
          };
        },
        apply(tr, value, _, newState) {
          // Check if comments were updated via meta
          const newComments = tr.getMeta(commentHighlightPluginKey);
          if (newComments !== undefined) {
            return {
              deco: createCommentDecorations(newState.doc, newComments),
              comments: newComments,
            };
          }
          // If document changed, remap decorations
          if (tr.docChanged) {
            return {
              deco: createCommentDecorations(newState.doc, value.comments),
              comments: value.comments,
            };
          }
          return value;
        },
      },
      props: {
        decorations(state) {
          const pluginState = commentHighlightPluginKey.getState(state);
          return pluginState?.deco ?? DecorationSet.empty;
        },
      },
    });
  });
}

function MilkdownEditorInner({
  value,
  onChange,
  placeholder = 'Start typing...',
  onSelectionChange,
  onSelectionEnd,
  comments = [],
  onCommentClick,
}: MilkdownEditorProps) {
  const { resolvedTheme } = useTheme();
  const wrapperRef = useRef<HTMLDivElement>(null);
  // Capture initial value at mount - editor manages its own state after that
  const initialValueRef = useRef(value);
  const placeholderRef = useRef(placeholder);
  const onChangeRef = useRef(onChange);
  const onSelectionChangeRef = useRef(onSelectionChange);
  const onSelectionEndRef = useRef(onSelectionEnd);
  const commentsRef = useRef(comments);
  const onCommentClickRef = useRef(onCommentClick);

  // Keep refs updated
  useEffect(() => {
    onChangeRef.current = onChange;
  }, [onChange]);

  useEffect(() => {
    onSelectionChangeRef.current = onSelectionChange;
  }, [onSelectionChange]);

  useEffect(() => {
    onSelectionEndRef.current = onSelectionEnd;
  }, [onSelectionEnd]);

  useEffect(() => {
    commentsRef.current = comments;
  }, [comments]);

  useEffect(() => {
    onCommentClickRef.current = onCommentClick;
  }, [onCommentClick]);

  // Set up click handler for comment highlights via event delegation
  useEffect(() => {
    const wrapper = wrapperRef.current;
    if (!wrapper) return;

    const handleClick = (e: MouseEvent) => {
      const target = e.target as HTMLElement;
      const commentHighlight = target.closest('.comment-highlight');
      if (commentHighlight && onCommentClickRef.current) {
        const commentId = commentHighlight.getAttribute('data-comment-id');
        if (commentId) {
          onCommentClickRef.current(commentId, { x: e.clientX, y: e.clientY });
        }
      }
    };

    wrapper.addEventListener('click', handleClick);
    return () => wrapper.removeEventListener('click', handleClick);
  }, []);

  // Handle mouseup to show floating button when selection ends
  useEffect(() => {
    const wrapper = wrapperRef.current;
    if (!wrapper) return;

    const handleMouseUp = (e: MouseEvent) => {
      if (!onSelectionEndRef.current) return;

      // Capture mouse position immediately
      const mouseX = e.clientX;
      const mouseY = e.clientY;

      // Small delay to let selection finalize
      setTimeout(() => {
        const selection = window.getSelection();
        if (!selection || selection.isCollapsed || !selection.toString().trim()) {
          onSelectionEndRef.current?.(null);
          return;
        }

        const selectedText = selection.toString().trim();
        if (selectedText.length < MIN_COMMENT_TEXT_LENGTH) {
          onSelectionEndRef.current?.(null);
          return;
        }

        // Use mouse position (where cursor is) instead of selection end
        onSelectionEndRef.current?.({
          text: selectedText,
          position: { x: mouseX, y: mouseY },
        });
      }, 10);
    };

    const handleMouseDown = (e: MouseEvent) => {
      // Don't clear if clicking on the floating button
      if ((e.target as HTMLElement).closest('.floating-comment-btn')) {
        return;
      }
      // Clear selection state when starting new selection
      onSelectionEndRef.current?.(null);
    };

    wrapper.addEventListener('mouseup', handleMouseUp);
    wrapper.addEventListener('mousedown', handleMouseDown);
    return () => {
      wrapper.removeEventListener('mouseup', handleMouseUp);
      wrapper.removeEventListener('mousedown', handleMouseDown);
    };
  }, []);

  // Handle Cmd+I to trigger selection popover (overrides italic formatting)
  useEffect(() => {
    const wrapper = wrapperRef.current;
    if (!wrapper) return;

    const handleKeyDown = (e: KeyboardEvent) => {
      // Check for Cmd+I (Mac) or Ctrl+I (Windows/Linux)
      if ((e.metaKey || e.ctrlKey) && e.key === 'i') {
        // Only handle if we have a selection callback
        if (!onSelectionChangeRef.current) return;

        const selection = window.getSelection();
        if (!selection) return;

        // Prevent Milkdown's italic formatting immediately
        e.preventDefault();
        e.stopImmediatePropagation();

        // Now check if we have valid selected text
        if (selection.isCollapsed || !selection.toString().trim()) {
          return;
        }

        const selectedText = selection.toString().trim();
        if (selectedText.length < MIN_COMMENT_TEXT_LENGTH) {
          return;
        }

        // Get position from the focus point (end of selection where cursor is)
        const focusNode = selection.focusNode;
        const focusOffset = selection.focusOffset;

        if (focusNode) {
          // Create a range at the focus point to get its position
          const focusRange = document.createRange();
          focusRange.setStart(focusNode, focusOffset);
          focusRange.setEnd(focusNode, focusOffset);
          const focusRect = focusRange.getBoundingClientRect();

          // If focusRect has no width (collapsed), use its position
          // Otherwise fall back to the selection's last rect
          let x = focusRect.right || focusRect.left;
          let y = focusRect.bottom || focusRect.top;

          // If position is 0,0 (can happen), fall back to last rect of selection
          if (x === 0 && y === 0) {
            const range = selection.getRangeAt(0);
            const rects = range.getClientRects();
            const lastRect = rects.length > 0 ? rects[rects.length - 1] : range.getBoundingClientRect();
            x = lastRect.right;
            y = lastRect.bottom;
          }

          onSelectionChangeRef.current({
            text: selectedText,
            position: { x, y },
          });
        }
      }
    };

    // Attach to wrapper element in capture phase to intercept before Milkdown
    wrapper.addEventListener('keydown', handleKeyDown, true);
    return () => wrapper.removeEventListener('keydown', handleKeyDown, true);
  }, []);

  // Create the highlight plugin once with initial comments
  const highlightPlugin = useCallback(
    () => createCommentHighlightPlugin(commentsRef.current),
    []
  );

  // Create the placeholder plugin once
  const placeholderPlugin = useCallback(
    () => createPlaceholderPlugin(placeholderRef.current),
    []
  );

  const { loading, get } = useEditor((root) =>
    Editor.make()
      .config((ctx) => {
        ctx.set(rootCtx, root);
        ctx.set(defaultValueCtx, initialValueRef.current);
        ctx.get(listenerCtx).markdownUpdated((_, markdown) => {
          onChangeRef.current(markdown);
        });
      })
      .config(nord)
      .use(commonmark)
      .use(gfm)
      .use(mermaidCodeBlockView)
      .use(listener)
      .use(highlightPlugin())
      .use(placeholderPlugin())
  );

  // Update decorations when comments change
  useEffect(() => {
    const editor = get();
    if (!editor || loading) return;

    try {
      // Get the editor view and dispatch a transaction to update comments
      editor.action((ctx) => {
        try {
          const view = ctx.get(editorViewCtx);
          if (view) {
            const tr = view.state.tr.setMeta(commentHighlightPluginKey, comments);
            view.dispatch(tr);
          }
        } catch {
          // editorViewCtx not available yet
        }
      });
    } catch {
      // Editor might not be ready
    }
  }, [comments, get, loading]);

  return (
    <div
      ref={wrapperRef}
      className={`milkdown-wrapper h-full relative ${resolvedTheme === 'dark' ? 'dark' : ''}`}
    >
      <Milkdown />
      {loading && (
        <div className="absolute inset-0 flex items-center justify-center text-muted-foreground text-sm bg-background/80">
          Loading editor...
        </div>
      )}
    </div>
  );
}

export function MarkdownEditor(props: MilkdownEditorProps) {
  // Generate a unique key on each mount to force fresh Milkdown instance
  const [instanceId] = useState(() => Math.random().toString(36).slice(2));

  return (
    <MilkdownProvider key={instanceId}>
      <MilkdownEditorInner {...props} />
    </MilkdownProvider>
  );
}

