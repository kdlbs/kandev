'use client';

import React, { useState, useCallback, useRef, useEffect } from 'react';
import { IconPlus, IconTrash } from '@tabler/icons-react';
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
  onDelete?: () => void; // Delete callback when editing existing comment
};

function computePopoverPosition(position: SelectionPosition): { left: number; top: number } {
  const popoverWidth = 384;
  const popoverHeight = 200;
  const gap = 8;
  let left = position.x + gap;
  let top = position.y + gap;
  if (left + popoverWidth > window.innerWidth - 16) left = Math.max(16, window.innerWidth - popoverWidth - 16);
  if (top + popoverHeight > window.innerHeight - 16) top = Math.max(16, position.y - popoverHeight - gap);
  return { left, top };
}

function usePopoverDismiss(onClose: () => void, popoverRef: React.RefObject<HTMLDivElement | null>) {
  useEffect(() => {
    const handleClickOutside = (e: MouseEvent) => {
      if (popoverRef.current && !popoverRef.current.contains(e.target as Node)) onClose();
    };
    const timer = setTimeout(() => { document.addEventListener('mousedown', handleClickOutside); }, 100);
    return () => { clearTimeout(timer); document.removeEventListener('mousedown', handleClickOutside); };
  }, [onClose, popoverRef]);

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose(); };
    document.addEventListener('keydown', handleKeyDown);
    return () => document.removeEventListener('keydown', handleKeyDown);
  }, [onClose]);
}

export function PlanSelectionPopover({
  selectedText,
  position,
  onAdd,
  onClose,
  editingComment,
  onDelete,
}: PlanSelectionPopoverProps) {
  const [comment, setComment] = useState(editingComment || '');
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const popoverRef = useRef<HTMLDivElement>(null);

  useEffect(() => { textareaRef.current?.focus(); }, []);
  usePopoverDismiss(onClose, popoverRef);

  const handleSubmit = useCallback(() => {
    if (!comment.trim()) return;
    onAdd(comment.trim(), selectedText);
    onClose();
  }, [comment, onAdd, selectedText, onClose]);

  const handleKeyDown = useCallback((e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) { e.preventDefault(); handleSubmit(); }
  }, [handleSubmit]);

  const isDisabled = !comment.trim();
  const previewText = selectedText.length > 80 ? selectedText.slice(0, 80).trim() + '…' : selectedText;
  const { left, top } = computePopoverPosition(position);

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
        <div className="flex items-center gap-2">
          <span className="text-xs text-muted-foreground/70">
            ⌘+Enter to {editingComment ? 'update' : 'add'}
          </span>
          {editingComment && onDelete && (
            <Button
              size="sm"
              variant="ghost"
              onClick={() => {
                onDelete();
                onClose();
              }}
              className="h-7 px-2 text-muted-foreground hover:text-destructive cursor-pointer"
            >
              <IconTrash className="h-3.5 w-3.5" />
            </Button>
          )}
        </div>
        <Button
          size="sm"
          onClick={handleSubmit}
          disabled={isDisabled}
          className="gap-1.5 cursor-pointer"
        >
          <IconPlus className="h-3.5 w-3.5" />
          {editingComment ? 'Update' : 'Add comment'}
        </Button>
      </div>
    </div>
  );
}

