'use client';

import { useState, useCallback, useRef, useEffect } from 'react';
import { IconPlus, IconGripHorizontal } from '@tabler/icons-react';
import { Button } from '@kandev/ui/button';
import { Textarea } from '@kandev/ui/textarea';
import { cn } from '@/lib/utils';

type EditorCommentPopoverProps = {
  selectedText: string;
  lineRange: { start: number; end: number };
  position: { x: number; y: number };
  onSubmit: (comment: string) => void;
  onClose: () => void;
};

export function EditorCommentPopover({
  selectedText,
  lineRange,
  position,
  onSubmit,
  onClose,
}: EditorCommentPopoverProps) {
  const [comment, setComment] = useState('');
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const popoverRef = useRef<HTMLDivElement>(null);

  // Draggable position — starts at computed initial position
  const popoverWidth = 384;
  const popoverHeight = 220;

  const computeInitial = () => {
    let left = position.x;
    let top = position.y;
    if (left + popoverWidth > window.innerWidth - 16) {
      left = Math.max(16, window.innerWidth - popoverWidth - 16);
    }
    if (top + popoverHeight > window.innerHeight - 16) {
      top = Math.max(16, position.y - popoverHeight);
    }
    return { left, top };
  };

  const [pos, setPos] = useState(computeInitial);
  const dragRef = useRef<{ startX: number; startY: number; origLeft: number; origTop: number } | null>(null);

  // Drag handlers
  const onDragStart = useCallback((e: React.MouseEvent) => {
    e.preventDefault();
    dragRef.current = { startX: e.clientX, startY: e.clientY, origLeft: pos.left, origTop: pos.top };

    const onMove = (ev: MouseEvent) => {
      if (!dragRef.current) return;
      const dx = ev.clientX - dragRef.current.startX;
      const dy = ev.clientY - dragRef.current.startY;
      setPos({ left: dragRef.current.origLeft + dx, top: dragRef.current.origTop + dy });
    };
    const onUp = () => {
      dragRef.current = null;
      document.removeEventListener('mousemove', onMove);
      document.removeEventListener('mouseup', onUp);
    };
    document.addEventListener('mousemove', onMove);
    document.addEventListener('mouseup', onUp);
  }, [pos.left, pos.top]);

  // Focus textarea on mount
  useEffect(() => {
    textareaRef.current?.focus();
  }, []);

  // Close on click outside
  useEffect(() => {
    const handleClickOutside = (e: MouseEvent) => {
      if (popoverRef.current && !popoverRef.current.contains(e.target as Node)) {
        onClose();
      }
    };
    // Delay adding listener to avoid immediate close from selection click
    const timer = setTimeout(() => {
      document.addEventListener('mousedown', handleClickOutside);
    }, 100);
    return () => {
      clearTimeout(timer);
      document.removeEventListener('mousedown', handleClickOutside);
    };
  }, [onClose]);

  // Close on Escape
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.stopPropagation();
        onClose();
      }
    };
    document.addEventListener('keydown', handleKeyDown, true);
    return () => document.removeEventListener('keydown', handleKeyDown, true);
  }, [onClose]);

  const handleSubmit = useCallback(() => {
    if (!comment.trim()) return;
    onSubmit(comment.trim());
  }, [comment, onSubmit]);

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
        e.preventDefault();
        handleSubmit();
      }
    },
    [handleSubmit]
  );

  const isDisabled = !comment.trim();

  // Truncate code preview
  const previewText = selectedText.length > 100
    ? selectedText.slice(0, 100).trim() + '…'
    : selectedText;

  // Format line range
  const lineRangeText = lineRange.start === lineRange.end
    ? `Line ${lineRange.start}`
    : `Lines ${lineRange.start}-${lineRange.end}`;

  return (
    <div
      ref={popoverRef}
      className={cn(
        'fixed z-50 w-96 rounded-xl border border-border/50 bg-popover/95 backdrop-blur-sm shadow-xl',
        'animate-in fade-in-0 zoom-in-95 duration-150'
      )}
      style={{ left: pos.left, top: pos.top }}
    >
      {/* Drag handle */}
      <div
        className="flex items-center justify-between px-4 pt-3 pb-1 cursor-grab active:cursor-grabbing select-none"
        onMouseDown={onDragStart}
      >
        <span className="px-2 py-0.5 rounded-md bg-muted text-xs font-mono text-muted-foreground">
          {lineRangeText}
        </span>
        <IconGripHorizontal className="h-3.5 w-3.5 text-muted-foreground/40" />
      </div>

      <div className="px-4 pb-4">
        {/* Code preview */}
        <pre className="mb-3 p-2 rounded-md bg-muted/50 text-xs text-muted-foreground font-mono line-clamp-3 overflow-hidden whitespace-pre-wrap">
          {previewText}
        </pre>

        {/* Comment input */}
        <Textarea
          ref={textareaRef}
          value={comment}
          onChange={(e) => setComment(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder="Add your comment or instruction..."
          className="min-h-[72px] resize-none text-sm border-border/50 focus:border-primary/50"
        />

        {/* Actions */}
        <div className="mt-3 flex items-center justify-between">
          <span className="text-xs text-muted-foreground/70">
            {'\u2318'}+Enter to add
          </span>
          <Button
            size="sm"
            onClick={handleSubmit}
            disabled={isDisabled}
            className="gap-1.5 cursor-pointer"
          >
            <IconPlus className="h-3.5 w-3.5" />
            Add comment
          </Button>
        </div>
      </div>
    </div>
  );
}
