'use client';

import { useEffect, useRef, useState } from 'react';
import { useTheme } from 'next-themes';
import { useEditor, EditorContent } from '@tiptap/react';
import StarterKit from '@tiptap/starter-kit';
import Placeholder from '@tiptap/extension-placeholder';
import Link from '@tiptap/extension-link';
import TaskList from '@tiptap/extension-task-list';
import TaskItem from '@tiptap/extension-task-item';
import { Table } from '@tiptap/extension-table';
import { TableRow } from '@tiptap/extension-table-row';
import { TableCell } from '@tiptap/extension-table-cell';
import { TableHeader } from '@tiptap/extension-table-header';
import CodeBlockLowlight from '@tiptap/extension-code-block-lowlight';
import { Markdown } from 'tiptap-markdown';
import { common, createLowlight } from 'lowlight';
import { CommentHighlightExtension, updateCommentHighlights, type CommentHighlight } from './tiptap-comment-highlight';

export type { CommentHighlight };

export type TextSelection = {
  text: string;
  position: { x: number; y: number };
};

type TipTapPlanEditorProps = {
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  onSelectionChange?: (selection: TextSelection | null) => void;
  onSelectionEnd?: (selection: TextSelection | null) => void;
  comments?: CommentHighlight[];
  onCommentClick?: (id: string, position: { x: number; y: number }) => void;
};

const MIN_COMMENT_TEXT_LENGTH = 3;

const lowlight = createLowlight(common);

export function TipTapPlanEditor({
  value,
  onChange,
  placeholder = 'Start typing...',
  onSelectionChange,
  onSelectionEnd,
  comments = [],
  onCommentClick,
}: TipTapPlanEditorProps) {
  const { resolvedTheme } = useTheme();
  const wrapperRef = useRef<HTMLDivElement>(null);
  const onChangeRef = useRef(onChange);
  const onSelectionChangeRef = useRef(onSelectionChange);
  const onSelectionEndRef = useRef(onSelectionEnd);
  const onCommentClickRef = useRef(onCommentClick);
  const [isReady, setIsReady] = useState(false);

  // Keep refs updated
  useEffect(() => { onChangeRef.current = onChange; }, [onChange]);
  useEffect(() => { onSelectionChangeRef.current = onSelectionChange; }, [onSelectionChange]);
  useEffect(() => { onSelectionEndRef.current = onSelectionEnd; }, [onSelectionEnd]);
  useEffect(() => { onCommentClickRef.current = onCommentClick; }, [onCommentClick]);

  const editor = useEditor({
    extensions: [
      StarterKit.configure({
        codeBlock: false, // We use CodeBlockLowlight instead
      }),
      CodeBlockLowlight.configure({ lowlight }),
      Markdown.configure({
        html: true,
        transformPastedText: true,
        transformCopiedText: true,
      }),
      Placeholder.configure({ placeholder }),
      Link.configure({ openOnClick: false }),
      TaskList,
      TaskItem.configure({ nested: true }),
      Table.configure({ resizable: false }),
      TableRow,
      TableCell,
      TableHeader,
      CommentHighlightExtension.configure({ comments }),
    ],
    content: value,
    editorProps: {
      attributes: {
        class: 'tiptap-plan-editor',
      },
    },
    onUpdate: ({ editor: ed }) => {
      // tiptap-markdown adds getMarkdown() to the editor storage
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      const md = (ed.storage as any).markdown?.getMarkdown?.() as string | undefined;
      onChangeRef.current(md ?? ed.getText());
    },
    onCreate: () => {
      setIsReady(true);
    },
  });

  // Update comment decorations when comments change
  useEffect(() => {
    if (!editor || !isReady) return;
    try {
      updateCommentHighlights(editor as Parameters<typeof updateCommentHighlights>[0], comments);
    } catch {
      // Editor might not be ready yet
    }
  }, [comments, editor, isReady]);

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
      const mouseX = e.clientX;
      const mouseY = e.clientY;

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
        onSelectionEndRef.current?.({
          text: selectedText,
          position: { x: mouseX, y: mouseY },
        });
      }, 10);
    };

    const handleMouseDown = (e: MouseEvent) => {
      if ((e.target as HTMLElement).closest('.floating-comment-btn')) return;
      onSelectionEndRef.current?.(null);
    };

    wrapper.addEventListener('mouseup', handleMouseUp);
    wrapper.addEventListener('mousedown', handleMouseDown);
    return () => {
      wrapper.removeEventListener('mouseup', handleMouseUp);
      wrapper.removeEventListener('mousedown', handleMouseDown);
    };
  }, []);

  // Handle Cmd+I to trigger selection popover
  useEffect(() => {
    const wrapper = wrapperRef.current;
    if (!wrapper) return;

    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === 'i') {
        if (!onSelectionChangeRef.current) return;
        const selection = window.getSelection();
        if (!selection) return;

        e.preventDefault();
        e.stopImmediatePropagation();

        if (selection.isCollapsed || !selection.toString().trim()) return;
        const selectedText = selection.toString().trim();
        if (selectedText.length < MIN_COMMENT_TEXT_LENGTH) return;

        const focusNode = selection.focusNode;
        const focusOffset = selection.focusOffset;

        if (focusNode) {
          const focusRange = document.createRange();
          focusRange.setStart(focusNode, focusOffset);
          focusRange.setEnd(focusNode, focusOffset);
          const focusRect = focusRange.getBoundingClientRect();

          let x = focusRect.right || focusRect.left;
          let y = focusRect.bottom || focusRect.top;

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

    wrapper.addEventListener('keydown', handleKeyDown, true);
    return () => wrapper.removeEventListener('keydown', handleKeyDown, true);
  }, []);

  return (
    <div
      ref={wrapperRef}
      className={`tiptap-plan-wrapper h-full relative ${resolvedTheme === 'dark' ? 'dark' : ''}`}
    >
      <EditorContent editor={editor} className="h-full" />
      {!isReady && (
        <div className="absolute inset-0 flex items-center justify-center text-muted-foreground text-sm bg-background/80">
          Loading editor...
        </div>
      )}
    </div>
  );
}
