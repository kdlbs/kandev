'use client';

import ReactMarkdown from 'react-markdown';
import remarkBreaks from 'remark-breaks';
import remarkGfm from 'remark-gfm';
import { cn } from '@/lib/utils';
import type { Message } from '@/lib/types/http';
import { RichBlocks } from '@/components/task/chat/messages/rich-blocks';

type ChatMessageProps = {
  comment: Message;
  label: string;
  className: string;
  showRichBlocks?: boolean;
};

export function ChatMessage({ comment, label, className, showRichBlocks }: ChatMessageProps) {
  const isUser = comment.author_type === 'user';

  return (
    <div className={cn('max-w-[85%] rounded-lg px-4 py-3 text-sm', className)}>
      <p className="text-[11px] uppercase tracking-wide mb-2 opacity-70">
        {label}
        {comment.requests_input ? (
          <span className="ml-2 rounded-full bg-amber-500/20 px-2 py-0.5 text-[10px] text-amber-300">
            Needs input
          </span>
        ) : null}
      </p>
      {isUser ? (
        <p className="whitespace-pre-wrap">{comment.content || '(empty)'}</p>
      ) : (
        <div className="prose prose-sm dark:prose-invert max-w-none prose-p:my-2 prose-p:leading-relaxed prose-ul:my-2 prose-ol:my-2 prose-li:my-0.5 prose-pre:my-3 prose-code:px-1.5 prose-code:py-0.5 prose-code:bg-background/50 prose-code:rounded prose-code:text-xs prose-code:before:content-none prose-code:after:content-none prose-pre:bg-background/80 prose-pre:text-xs prose-strong:text-foreground prose-headings:text-foreground">
          <ReactMarkdown remarkPlugins={[remarkGfm, remarkBreaks]}>
            {comment.content || '(empty)'}
          </ReactMarkdown>
          {showRichBlocks ? <RichBlocks comment={comment} /> : null}
        </div>
      )}
    </div>
  );
}
