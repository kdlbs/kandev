'use client';

import { useState } from 'react';
import { IconBrain, IconChevronDown, IconChevronRight } from '@tabler/icons-react';
import type { Message } from '@/lib/types/http';
import type { RichMetadata } from '@/components/task/chat/types';

export function ThinkingMessage({ comment }: { comment: Message }) {
  const [isExpanded, setIsExpanded] = useState(false);
  const metadata = comment.metadata as RichMetadata | undefined;
  const text = metadata?.thinking ?? comment.content;

  if (!text) return null;

  return (
    <div className="w-full">
      {/* Icon + Summary Row */}
      <div className="flex items-start gap-3 w-full">
        {/* Icon */}
        <div className="flex-shrink-0 mt-0.5">
          <IconBrain className="h-4 w-4 text-muted-foreground" />
        </div>

        {/* Content */}
        <div className="flex-1 min-w-0 pt-0.5">
          <div className="flex items-center gap-2 text-xs">
            <button
              type="button"
              onClick={() => setIsExpanded(!isExpanded)}
              className="inline-flex items-center gap-1.5 text-left cursor-pointer hover:opacity-70 transition-opacity"
            >
              <span className="font-mono text-xs text-muted-foreground">
                Thinking
              </span>
              {isExpanded ? (
                <IconChevronDown className="h-3.5 w-3.5 text-muted-foreground/50 flex-shrink-0" />
              ) : (
                <IconChevronRight className="h-3.5 w-3.5 text-muted-foreground/50 flex-shrink-0" />
              )}
            </button>
          </div>

          {/* Expanded Content */}
          {isExpanded && (
            <div className="mt-2 pl-4 border-l-2 border-border/30">
              <div className="whitespace-pre-wrap leading-normal text-xs text-foreground/80">{text}</div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
