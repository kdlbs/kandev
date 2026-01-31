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

  // Calculate position - flip above if would overflow bottom
  const popoverHeight = 200; // approximate height of popover
  const gap = 10;
  const wouldOverflowBottom = position.y + gap + popoverHeight > window.innerHeight;
  const top = wouldOverflowBottom
    ? position.y - popoverHeight - gap // Position above selection
    : position.y + gap; // Position below selection

  return (
    <div
      ref={popoverRef}
      className={cn(
        'fixed z-50 w-96 rounded-xl border border-border/50 bg-popover/95 backdrop-blur-sm p-4 shadow-xl',
        'animate-in fade-in-0 zoom-in-95 duration-150'
      )}
      style={{
        left: Math.min(position.x - 192, window.innerWidth - 400),
        top: Math.max(10, top), // Ensure at least 10px from top
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

