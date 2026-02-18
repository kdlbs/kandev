'use client';

import { useRef, useEffect, useState, useCallback } from 'react';
import { IconTrash, IconGripHorizontal } from '@tabler/icons-react';
import { Button } from '@kandev/ui/button';
import { cn } from '@/lib/utils';
import type { DiffComment } from '@/lib/diff/types';

type CommentViewPopoverProps = {
  comments: DiffComment[];
  position: { x: number; y: number };
  onDelete: (commentId: string) => void;
  onClose: () => void;
};

type DragState = { startX: number; startY: number; origLeft: number; origTop: number };

function computePopoverInitialPos(position: { x: number; y: number }, width: number, height: number) {
  let left = position.x;
  let top = position.y;
  if (left + width > window.innerWidth - 16) left = Math.max(16, window.innerWidth - width - 16);
  if (top + height > window.innerHeight - 16) top = Math.max(16, position.y - height);
  return { left, top };
}

function useDraggablePos(position: { x: number; y: number }, width: number, height: number) {
  const [pos, setPos] = useState(() => computePopoverInitialPos(position, width, height));
  const dragRef = useRef<DragState | null>(null);

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

  return { pos, onDragStart };
}

function usePopoverClose(onClose: () => void, popoverRef: React.RefObject<HTMLDivElement | null>) {
  useEffect(() => {
    const handleClickOutside = (e: MouseEvent) => {
      if (popoverRef.current && !popoverRef.current.contains(e.target as Node)) onClose();
    };
    const timer = setTimeout(() => { document.addEventListener('mousedown', handleClickOutside); }, 100);
    return () => { clearTimeout(timer); document.removeEventListener('mousedown', handleClickOutside); };
  }, [onClose, popoverRef]);

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') { e.stopPropagation(); onClose(); }
    };
    document.addEventListener('keydown', handleKeyDown, true);
    return () => document.removeEventListener('keydown', handleKeyDown, true);
  }, [onClose]);
}

function formatLineRange(start: number, end: number) {
  return start === end ? `L${start}` : `L${start}-${end}`;
}

function CommentItem({ comment, onDelete }: { comment: DiffComment; onDelete: (id: string) => void }) {
  return (
    <div className="p-3">
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
      {comment.codeContent && (
        <pre className="mb-2 p-2 rounded-md bg-muted/50 text-[10px] text-muted-foreground font-mono max-h-[80px] overflow-auto whitespace-pre-wrap">
          {comment.codeContent}
        </pre>
      )}
      <p className="text-xs text-foreground leading-relaxed">{comment.annotation}</p>
    </div>
  );
}

export function CommentViewPopover({ comments, position, onDelete, onClose }: CommentViewPopoverProps) {
  const popoverRef = useRef<HTMLDivElement>(null);
  const { pos, onDragStart } = useDraggablePos(position, 350, 200);
  usePopoverClose(onClose, popoverRef);

  return (
    <div
      ref={popoverRef}
      className={cn(
        'fixed z-50 w-[350px] max-h-[350px] overflow-auto rounded-xl border border-border/50 bg-popover/95 backdrop-blur-sm shadow-xl',
        'animate-in fade-in-0 zoom-in-95 duration-150'
      )}
      style={{ left: pos.left, top: pos.top }}
    >
      <div
        className="flex items-center justify-between px-3 pt-2 pb-1 cursor-grab active:cursor-grabbing select-none sticky top-0 bg-popover/95 backdrop-blur-sm z-10"
        onMouseDown={onDragStart}
      >
        <span className="text-xs text-muted-foreground">
          {comments.length} comment{comments.length !== 1 ? 's' : ''}
        </span>
        <IconGripHorizontal className="h-3.5 w-3.5 text-muted-foreground/40" />
      </div>
      <div className="divide-y divide-border/30">
        {comments.map((comment) => (
          <CommentItem key={comment.id} comment={comment} onDelete={onDelete} />
        ))}
      </div>
    </div>
  );
}
