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

type DragState = { startX: number; startY: number; origLeft: number; origTop: number };

function computeInitialPos(position: { x: number; y: number }, width: number, height: number) {
  let left = position.x;
  let top = position.y;
  if (left + width > window.innerWidth - 16) left = Math.max(16, window.innerWidth - width - 16);
  if (top + height > window.innerHeight - 16) top = Math.max(16, position.y - height);
  return { left, top };
}

function useDraggablePos(position: { x: number; y: number }, width: number, height: number) {
  const [pos, setPos] = useState(() => computeInitialPos(position, width, height));
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

function usePopoverDismiss(onClose: () => void, popoverRef: React.RefObject<HTMLDivElement | null>) {
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

function PopoverBody({ selectedText, comment, setComment, handleSubmit, textareaRef }: {
  selectedText: string;
  comment: string;
  setComment: (v: string) => void;
  handleSubmit: () => void;
  textareaRef: React.RefObject<HTMLTextAreaElement | null>;
}) {
  const previewText = selectedText.length > 100 ? selectedText.slice(0, 100).trim() + '…' : selectedText;
  const isDisabled = !comment.trim();

  const handleKeyDown = useCallback((e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) { e.preventDefault(); handleSubmit(); }
  }, [handleSubmit]);

  return (
    <div className="px-4 pb-4">
      <pre className="mb-3 p-2 rounded-md bg-muted/50 text-xs text-muted-foreground font-mono line-clamp-3 overflow-hidden whitespace-pre-wrap">
        {previewText}
      </pre>
      <Textarea
        ref={textareaRef}
        value={comment}
        onChange={(e) => setComment(e.target.value)}
        onKeyDown={handleKeyDown}
        placeholder="Add your comment or instruction..."
        className="min-h-[72px] resize-none text-sm border-border/50 focus:border-primary/50"
      />
      <div className="mt-3 flex items-center justify-between">
        <span className="text-xs text-muted-foreground/70">{'\u2318'}+Enter to add</span>
        <Button size="sm" onClick={handleSubmit} disabled={isDisabled} className="gap-1.5 cursor-pointer">
          <IconPlus className="h-3.5 w-3.5" />
          Add comment
        </Button>
      </div>
    </div>
  );
}

export function EditorCommentPopover({ selectedText, lineRange, position, onSubmit, onClose }: EditorCommentPopoverProps) {
  const [comment, setComment] = useState('');
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const popoverRef = useRef<HTMLDivElement>(null);
  const { pos, onDragStart } = useDraggablePos(position, 384, 220);
  usePopoverDismiss(onClose, popoverRef);

  useEffect(() => { textareaRef.current?.focus(); }, []);

  const handleSubmit = useCallback(() => {
    if (!comment.trim()) return;
    onSubmit(comment.trim());
  }, [comment, onSubmit]);

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
      <div
        className="flex items-center justify-between px-4 pt-3 pb-1 cursor-grab active:cursor-grabbing select-none"
        onMouseDown={onDragStart}
      >
        <span className="px-2 py-0.5 rounded-md bg-muted text-xs font-mono text-muted-foreground">{lineRangeText}</span>
        <IconGripHorizontal className="h-3.5 w-3.5 text-muted-foreground/40" />
      </div>
      <PopoverBody
        selectedText={selectedText}
        comment={comment}
        setComment={setComment}
        handleSubmit={handleSubmit}
        textareaRef={textareaRef}
      />
    </div>
  );
}
