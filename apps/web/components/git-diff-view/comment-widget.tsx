'use client';

import { useState, useEffect, useRef } from 'react';
import { SplitSide, type DiffFile } from '@git-diff-view/react';
import { Button } from '@kandev/ui/button';
import { Badge } from '@kandev/ui/badge';
import { Textarea } from '@kandev/ui/textarea';
import type { DiffComment } from './types';

interface InlineCommentWidgetProps {
  lineNumber: number;
  side: SplitSide;
  startLine: number;
  diffFile: DiffFile;
  wrapperRef: React.RefObject<HTMLDivElement | null>;
  viewMode: 'split' | 'unified';
  filePath: string;
  onSave: (comment: DiffComment) => void;
  onCancel: () => void;
  onStartLineChange: (startLine: number) => void;
}

export function InlineCommentWidget({
  lineNumber,
  side,
  startLine,
  diffFile,
  wrapperRef,
  viewMode,
  filePath,
  onSave,
  onCancel,
  onStartLineChange,
}: InlineCommentWidgetProps) {
  const [content, setContent] = useState('');
  const coverRef = useRef<HTMLDivElement | null>(null);
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  const endLine = lineNumber;
  const rangeStart = Math.min(startLine, endLine);
  const rangeEnd = Math.max(startLine, endLine);
  const isSingleLine = rangeStart === rangeEnd;

  // Auto-focus textarea
  useEffect(() => {
    setTimeout(() => textareaRef.current?.focus(), 50);
  }, []);

  // Create visual overlay for selected lines (multi-line selection effect)
  useEffect(() => {
    if (!wrapperRef.current || isSingleLine) return;

    let coverItem = coverRef.current;
    if (!coverItem) {
      coverItem = document.createElement('div');
      coverRef.current = coverItem;
      coverItem.className = 'diff-multiline-selection-overlay';
    }

    const updateCover = () => {
      const id = diffFile.getId();
      const wrapper = document.querySelector(`#diff-root${id}`);
      if (!wrapper || !coverItem) return;

      const items: HTMLElement[] = [];

      for (let i = rangeStart; i <= rangeEnd; i++) {
        const lineIndex =
          viewMode === 'split'
            ? diffFile.getSplitLineIndexByLineNumber(i, side)
            : diffFile.getUnifiedLineIndexByLineNumber(i, side);

        if (lineIndex === undefined) continue;

        const lineDom = wrapper.querySelector(`[data-line="${lineIndex + 1}"]`) as HTMLElement;
        if (lineDom) {
          items.push(lineDom);
        }
      }

      if (items.length === 0) {
        coverItem.style.display = 'none';
        return;
      }

      const firstItem = items[0];
      const lastItem = items[items.length - 1];
      const wrapperEl = wrapperRef.current;
      if (!wrapperEl) return;

      const wrapperRect = wrapperEl.getBoundingClientRect();
      const firstRect = firstItem.getBoundingClientRect();
      const lastRect = lastItem.getBoundingClientRect();

      coverItem.style.display = 'block';
      coverItem.style.top = `${firstRect.top - wrapperRect.top + wrapperEl.scrollTop}px`;
      coverItem.style.height = `${lastRect.bottom - firstRect.top}px`;

      if (viewMode === 'split') {
        coverItem.style.left = side === SplitSide.old ? '0' : '50%';
        coverItem.style.width = '50%';
      } else {
        coverItem.style.left = '0';
        coverItem.style.width = '100%';
      }

      if (!wrapperEl.contains(coverItem)) {
        wrapperEl.appendChild(coverItem);
      }
    };

    updateCover();
    const unsubscribe = diffFile.subscribe(() => setTimeout(updateCover, 0));

    return () => {
      coverItem?.remove();
      coverRef.current = null;
      unsubscribe();
    };
  }, [rangeStart, rangeEnd, side, diffFile, wrapperRef, viewMode, isSingleLine]);

  const handleSubmit = () => {
    if (content.trim()) {
      const comment: DiffComment = {
        id: `${filePath}-${Date.now()}`,
        startLine: rangeStart,
        endLine: rangeEnd,
        side,
        content: content.trim(),
        createdAt: new Date().toISOString(),
      };
      onSave(comment);
    }
  };

  return (
    <div className="p-3 bg-card border-t border-border">
      <div className="space-y-3">
        <div className="flex items-center gap-2 text-sm">
          <span className="text-muted-foreground">Comment on</span>
          {isSingleLine ? (
            <span className="font-medium">line {rangeStart}</span>
          ) : (
            <>
              <span className="text-muted-foreground">lines</span>
              <input
                type="number"
                min={1}
                max={endLine}
                value={startLine}
                onChange={(e) =>
                  onStartLineChange(Math.max(1, Math.min(endLine, Number(e.target.value))))
                }
                className="w-16 px-2 py-0.5 text-center border border-border rounded text-sm bg-background"
              />
              <span className="text-muted-foreground">to</span>
              <span className="font-medium">{endLine}</span>
            </>
          )}
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
              handleSubmit();
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
          <Button size="sm" onClick={handleSubmit} disabled={!content.trim()}>
            Add Comment
          </Button>
        </div>
      </div>
    </div>
  );
}
