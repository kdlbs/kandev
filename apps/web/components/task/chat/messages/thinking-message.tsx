'use client';

import { useState, memo } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { IconBrain } from '@tabler/icons-react';
import type { Message } from '@/lib/types/http';
import type { RichMetadata } from '@/components/task/chat/types';
import { ExpandableRow } from './expandable-row';

export const ThinkingMessage = memo(function ThinkingMessage({ comment }: { comment: Message }) {
  const [isExpanded, setIsExpanded] = useState(false);
  const metadata = comment.metadata as RichMetadata | undefined;
  const text = metadata?.thinking ?? comment.content;

  if (!text) return null;

  return (
    <ExpandableRow
      icon={<IconBrain className="h-4 w-4 text-muted-foreground" />}
      header={
        <div className="flex items-center gap-2 text-xs">
          <span className="inline-flex items-center gap-1.5">
            <span className="font-mono text-xs text-muted-foreground">Thinking</span>
          </span>
        </div>
      }
      hasExpandableContent={!!text}
      isExpanded={isExpanded}
      onToggle={() => setIsExpanded(!isExpanded)}
    >
      <div className="pl-4 border-l-2 border-border/30">
        <div className="prose prose-sm prose-neutral dark:prose-invert max-w-none text-xs text-foreground/70 [&>*]:my-1 [&>p]:my-1 [&>ul]:my-1 [&>ol]:my-1 [&_strong]:text-foreground/80 [&_code]:text-foreground/70 [&_code]:bg-muted/50 [&_code]:px-1 [&_code]:py-0.5 [&_code]:rounded [&_code]:text-[0.9em]">
          <ReactMarkdown remarkPlugins={[remarkGfm]}>
            {text}
          </ReactMarkdown>
        </div>
      </div>
    </ExpandableRow>
  );
});
