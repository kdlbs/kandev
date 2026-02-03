'use client';

import { useRef, useEffect } from 'react';
import { IconTrash } from '@tabler/icons-react';
import { Button } from '@kandev/ui/button';
import { cn } from '@/lib/utils';
import type { DiffComment } from '@/lib/diff/types';

type CommentViewPopoverProps = {
  comments: DiffComment[];
  position: { x: number; y: number };
  onDelete: (commentId: string) => void;
  onClose: () => void;
};

export function CommentViewPopover({
  comments,
  position,
  onDelete,
  onClose,
}: CommentViewPopoverProps) {
  const popoverRef = useRef<HTMLDivElement>(null);

  // Close on click outside
  useEffect(() => {
    const handleClickOutside = (e: MouseEvent) => {
      if (popoverRef.current && !popoverRef.current.contains(e.target as Node)) {
        onClose();
      }
    };
    // Delay adding listener to avoid immediate close
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

  // Calculate position
  const popoverWidth = 350;
  const popoverHeight = 200;
  const gap = 8;

  let left = position.x + gap;
  let top = position.y + gap;

  // If would overflow right, shift left
  if (left + popoverWidth > window.innerWidth - 16) {
    left = Math.max(16, window.innerWidth - popoverWidth - 16);
  }

  // If would overflow bottom, position above
  if (top + popoverHeight > window.innerHeight - 16) {
    top = Math.max(16, position.y - popoverHeight - gap);
  }

  // Format line range
  const formatLineRange = (start: number, end: number) => {
    return start === end ? `L${start}` : `L${start}-${end}`;
  };

  return (
    <div
      ref={popoverRef}
      className={cn(
        'fixed z-50 w-[350px] max-h-[350px] overflow-auto rounded-xl border border-border/50 bg-popover/95 backdrop-blur-sm shadow-xl',
        'animate-in fade-in-0 zoom-in-95 duration-150'
      )}
      style={{
        left,
        top,
      }}
    >
      {/* Comments list */}
      <div className="divide-y divide-border/30">
        {comments.map((comment) => (
          <div key={comment.id} className="p-3">
            {/* Line range and delete */}
            <div className="flex items-center justify-between mb-2">
              <span className="px-2 py-0.5 rounded-md bg-muted text-[10px] font-mono text-muted-foreground">
                {formatLineRange(comment.startLine, comment.endLine)}
              </span>
              <Button
                size="sm"
                variant="ghost"
                className="h-6 w-6 p-0 cursor-pointer text-muted-foreground hover:text-destructive"
                onClick={() => onDelete(comment.id)}
              >
                <IconTrash className="h-3.5 w-3.5" />
              </Button>
            </div>

            {/* Code snippet with scroll */}
            {comment.codeContent && (
              <pre className="mb-2 p-2 rounded-md bg-muted/50 text-[10px] text-muted-foreground font-mono max-h-[80px] overflow-auto whitespace-pre-wrap">
                {comment.codeContent}
              </pre>
            )}

            {/* Annotation */}
            <p className="text-xs text-foreground leading-relaxed">
              {comment.annotation}
            </p>
          </div>
        ))}
      </div>
    </div>
  );
}
