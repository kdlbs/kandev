'use client';

import { useMemo } from 'react';
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

function formatMessageTime(dateString: string): string {
  if (!dateString) return '';
  try {
    const date = new Date(dateString);
    if (isNaN(date.getTime())) return '';
    return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  } catch {
    return '';
  }
}

export function ChatMessage({ comment, label, className, showRichBlocks }: ChatMessageProps) {
  const isUser = comment.author_type === 'user';
  const formattedTime = useMemo(() => formatMessageTime(comment.created_at), [comment.created_at]);

  return (
    <div className={cn('w-full rounded-lg border px-4 py-3 text-sm', className)}>
      <div className="flex items-center justify-between mb-2">
        <p className="text-[11px] uppercase tracking-wide opacity-70">
          {label}
          {comment.requests_input ? (
            <span className="ml-2 rounded-full bg-amber-500/20 px-2 py-0.5 text-[10px] text-amber-300">
              Needs input
            </span>
          ) : null}
        </p>
        {formattedTime && (
          <span className="text-[10px] text-muted-foreground/60">{formattedTime}</span>
        )}
      </div>
      {isUser ? (
        <p className="whitespace-pre-wrap">{comment.content || '(empty)'}</p>
      ) : (
        <div className="prose prose-sm dark:prose-invert max-w-none prose-p:my-4 prose-p:leading-relaxed prose-ul:my-4 prose-ul:list-disc prose-ul:pl-6 prose-ol:my-4 prose-ol:list-decimal prose-ol:pl-6 prose-li:my-1.5 prose-pre:my-5 prose-strong:text-foreground prose-strong:font-bold prose-headings:text-foreground prose-headings:font-bold">
          <ReactMarkdown
            remarkPlugins={[remarkGfm, remarkBreaks]}
            components={{
              code: ({ node, className, children, ...props }) => {
                // Check if it's inline code (no className means inline, className with language-* means code block)
                const isInline = !className || !className.startsWith('language-');

                // Inline code (backticks)
                if (isInline) {
                  return (
                    <code
                      style={{
                        padding: '2px 6px',
                        backgroundColor: 'rgba(59, 130, 246, 0.2)',
                        color: 'rgb(96, 165, 250)',
                        borderRadius: '4px',
                        fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace',
                        fontSize: '0.9em',
                      }}
                      {...props}
                    >
                      {children}
                    </code>
                  );
                }
                // Code blocks (triple backticks)
                return (
                  <code
                    className={className}
                    style={{
                      display: 'block',
                      padding: '12px',
                      backgroundColor: 'rgba(0, 0, 0, 0.3)',
                      borderRadius: '6px',
                      overflowX: 'auto',
                      fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace',
                      fontSize: '0.85em',
                    }}
                    {...props}
                  >
                    {children}
                  </code>
                );
              },
              ol: ({ node, children, ...props }) => (
                <ol
                  style={{
                    listStyleType: 'decimal',
                    paddingLeft: '1.5rem',
                    marginTop: '1rem',
                    marginBottom: '1rem',
                  }}
                  {...props}
                >
                  {children}
                </ol>
              ),
              ul: ({ node, children, ...props }) => (
                <ul
                  style={{
                    listStyleType: 'disc',
                    paddingLeft: '1.5rem',
                    marginTop: '1rem',
                    marginBottom: '1rem',
                  }}
                  {...props}
                >
                  {children}
                </ul>
              ),
              li: ({ node, children, ...props }) => (
                <li
                  style={{
                    marginTop: '0.375rem',
                    marginBottom: '0.375rem',
                  }}
                  {...props}
                >
                  {children}
                </li>
              ),
              p: ({ node, children, ...props }) => (
                <p
                  style={{
                    marginTop: '1rem',
                    marginBottom: '1rem',
                    lineHeight: '1.625',
                  }}
                  {...props}
                >
                  {children}
                </p>
              ),
            }}
          >
            {comment.content || '(empty)'}
          </ReactMarkdown>
          {showRichBlocks ? <RichBlocks comment={comment} /> : null}
        </div>
      )}
    </div>
  );
}
