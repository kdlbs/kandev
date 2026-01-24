'use client';

import { useState, useEffect, useRef } from 'react';
import { SplitSide, type DiffFile } from '@git-diff-view/react';
import { Button } from '@kandev/ui/button';
import { Badge } from '@kandev/ui/badge';
import { Textarea } from '@kandev/ui/textarea';
import { findRowByLineNumber } from './dom-utils';
import type { DragSelectionState } from './types';

interface MultiLineCommentInputProps {
  startLine: number;
  endLine: number;
  side: SplitSide;
  onSave: (content: string) => void;
  onCancel: () => void;
}

export function MultiLineCommentInput({
  startLine,
  endLine,
  side,
  onSave,
  onCancel,
}: MultiLineCommentInputProps) {
  const [content, setContent] = useState('');
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  useEffect(() => {
    textareaRef.current?.focus();
  }, []);

  return (
    <div className="space-y-3">
      <div className="flex items-center gap-2 text-sm">
        <span className="text-muted-foreground">Comment on lines</span>
        <span className="font-medium">{startLine}</span>
        <span className="text-muted-foreground">to</span>
        <span className="font-medium">{endLine}</span>
        <Badge variant="outline" className="text-xs">
          {side === SplitSide.old ? 'old' : 'new'}
        </Badge>
      </div>

      <Textarea
        ref={textareaRef}
        value={content}
        onChange={(e) => setContent(e.target.value)}
        placeholder="Add a comment..."
        className="min-h-[80px] text-sm resize-none"
        onKeyDown={(e) => {
          if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
            if (content.trim()) onSave(content.trim());
          }
          if (e.key === 'Escape') {
            onCancel();
          }
        }}
      />

      <div className="flex justify-end gap-2">
        <Button variant="outline" size="sm" onClick={onCancel}>
          Cancel
        </Button>
        <Button size="sm" onClick={() => content.trim() && onSave(content.trim())} disabled={!content.trim()}>
          Add Comment
        </Button>
      </div>
    </div>
  );
}

interface PositionedCommentWidgetProps {
  selection: DragSelectionState;
  diffFile: DiffFile;
  wrapperRef: React.RefObject<HTMLDivElement | null>;
  viewMode: 'split' | 'unified';
  onSave: (content: string) => void;
  onCancel: () => void;
}

export function PositionedCommentWidget({
  selection,
  diffFile,
  wrapperRef,
  viewMode,
  onSave,
  onCancel,
}: PositionedCommentWidgetProps) {
  const [position, setPosition] = useState<{ top: number; left: number | string; width: string } | null>(null);

  useEffect(() => {
    const wrapper = wrapperRef.current;
    if (!wrapper) return;

    const endRow = findRowByLineNumber(wrapper, viewMode, selection.side, selection.endLine);

    if (endRow) {
      const wrapperRect = wrapper.getBoundingClientRect();
      const rowRect = endRow.getBoundingClientRect();

      setPosition({
        top: rowRect.bottom - wrapperRect.top + wrapper.scrollTop,
        left: viewMode === 'split' && selection.side === SplitSide.old ? 0 : viewMode === 'split' ? '50%' : 0,
        width: viewMode === 'split' ? '50%' : '100%',
      });
    }
  }, [selection, diffFile, wrapperRef, viewMode]);

  if (!position) return null;

  return (
    <div
      className="absolute z-30 bg-card border border-border rounded shadow-lg p-3"
      style={{ top: position.top, left: position.left, width: position.width }}
    >
      <MultiLineCommentInput
        startLine={selection.startLine}
        endLine={selection.endLine}
        side={selection.side}
        onSave={onSave}
        onCancel={onCancel}
      />
    </div>
  );
}
