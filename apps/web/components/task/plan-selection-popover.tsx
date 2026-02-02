'use client';

import { useState, useCallback, useRef, useEffect } from 'react';
import { IconPlus } from '@tabler/icons-react';
import { Button } from '@kandev/ui/button';
import { Textarea } from '@kandev/ui/textarea';
import { cn } from '@/lib/utils';

type SelectionPosition = {
  x: number;
  y: number;
};

type PlanSelectionPopoverProps = {
  selectedText: string;
  position: SelectionPosition;
  onAdd: (comment: string, selectedText: string) => void;
  onClose: () => void;
  editingComment?: string; // Pre-fill if editing existing comment
};

export function PlanSelectionPopover({
  selectedText,
  position,
  onAdd,
  onClose,
  editingComment,
}: PlanSelectionPopoverProps) {
  const [comment, setComment] = useState(editingComment || '');
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const popoverRef = useRef<HTMLDivElement>(null);

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
        onClose();
      }
    };
    document.addEventListener('keydown', handleKeyDown);
    return () => document.removeEventListener('keydown', handleKeyDown);
  }, [onClose]);

  const handleSubmit = useCallback(() => {
    if (!comment.trim()) return;
    onAdd(comment.trim(), selectedText);
    onClose();
  }, [comment, onAdd, selectedText, onClose]);

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

  // Truncate text for preview
  const previewText = selectedText.length > 80
    ? selectedText.slice(0, 80).trim() + '…'
    : selectedText;

  // Calculate position - anchor at bottom-right of selection, adjust if would overflow
  const popoverWidth = 384; // w-96 = 24rem = 384px
  const popoverHeight = 200; // approximate height of popover
  const gap = 8;

  // Start at bottom-right of selection
  let left = position.x + gap;
  let top = position.y + gap;

  // If would overflow right, shift left
  if (left + popoverWidth > window.innerWidth - 16) {
    left = Math.max(16, window.innerWidth - popoverWidth - 16);
  }

  // If would overflow bottom, position above the selection instead
  if (top + popoverHeight > window.innerHeight - 16) {
    top = Math.max(16, position.y - popoverHeight - gap);
  }

  return (
    <div
      ref={popoverRef}
      className={cn(
        'fixed z-50 w-96 rounded-xl border border-border/50 bg-popover/95 backdrop-blur-sm p-4 shadow-xl',
        'animate-in fade-in-0 zoom-in-95 duration-150'
      )}
      style={{
        left,
        top,
      }}
    >
      {/* Text preview */}
      <p className="mb-3 text-xs text-muted-foreground line-clamp-2 leading-relaxed italic">
        &ldquo;{previewText}&rdquo;
      </p>

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
          ⌘+Enter to add
        </span>
        <Button
          size="sm"
          onClick={handleSubmit}
          disabled={isDisabled}
          className="gap-1.5"
        >
          <IconPlus className="h-3.5 w-3.5" />
          {editingComment ? 'Update' : 'Add comment'}
        </Button>
      </div>
    </div>
  );
}

