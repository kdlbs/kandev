'use client';

import ReactMarkdown from 'react-markdown';
import remarkBreaks from 'remark-breaks';
import remarkGfm from 'remark-gfm';
import { cn } from '@/lib/utils';
import type { Message } from '@/lib/types/http';
import { RichBlocks } from '@/components/task/chat/messages/rich-blocks';
import { InlineCode } from '@/components/task/chat/messages/inline-code';
import { CodeBlock } from '@/components/task/chat/messages/code-block';

type ChatMessageProps = {
  comment: Message;
  label: string;
  className: string;
  showRichBlocks?: boolean;
};

// Regex to match @file references (file paths after @)
// Matches @path/to/file.ext or @file.ext patterns
const FILE_REF_REGEX = /@([\w./-]+\.[\w]+|[\w/-]+)/g;

/**
 * Renders content with file references highlighted in code style
 */
function renderContentWithFileRefs(content: string): React.ReactNode[] {
  const parts: React.ReactNode[] = [];
  let lastIndex = 0;
  let match;
  let keyIndex = 0;

  FILE_REF_REGEX.lastIndex = 0; // Reset regex state
  while ((match = FILE_REF_REGEX.exec(content)) !== null) {
    // Add text before the match
    if (match.index > lastIndex) {
      parts.push(content.slice(lastIndex, match.index));
    }

    // Add the file reference with code styling
    const filePath = match[1];
    parts.push(
      <code
        key={`file-ref-${keyIndex++}`}
        className="px-1 py-0.5 bg-emerald-500/20 text-emerald-400 rounded font-mono text-[0.9em]"
      >
        @{filePath}
      </code>
    );

    lastIndex = match.index + match[0].length;
  }

  // Add remaining text after last match
  if (lastIndex < content.length) {
    parts.push(content.slice(lastIndex));
  }

  return parts.length > 0 ? parts : [content];
}

export function ChatMessage({ comment, label, className, showRichBlocks }: ChatMessageProps) {
  const isUser = comment.author_type === 'user';
  const isTaskDescription = label === 'Task';

  // Keep the old card-based layout for task descriptions (amber banner)
  if (isTaskDescription) {
    return (
      <div className={cn('w-full rounded-lg  px-4 py-3 text-xs', className)}>
        <div className="flex items-center">
          <p className="text-[11px] uppercase tracking-wide opacity-70">
            {comment.requests_input ? (
              <span className="ml-2 rounded-full bg-amber-500/20 px-2 py-0.5 text-[10px] text-amber-300">
                Needs input
              </span>
            ) : null}
          </p>
        </div>
        <p className="whitespace-pre-wrap">
          {comment.content ? renderContentWithFileRefs(comment.content) : '(empty)'}
        </p>
      </div>
    );
  }

  // User message: right-aligned bubble
  if (isUser) {
    return (
      <div className="flex justify-end w-full overflow-hidden">
        <div className="max-w-[85%] sm:max-w-[75%] md:max-w-2xl overflow-hidden">
          <div className="rounded-2xl border-primary/30 bg-primary/10 px-4 py-2.5 text-xs overflow-hidden">
            <p className="whitespace-pre-wrap break-words overflow-wrap-anywhere">
              {comment.content ? renderContentWithFileRefs(comment.content) : '(empty)'}
            </p>
          </div>
        </div>
      </div>
    );
  }

  // Agent message: icon on left, no card background
  return (
    <div className="flex items-start gap-2 sm:gap-3 w-full">

      {/* Content */}
      <div className="flex-1 min-w-0 text-xs">
        <div className="prose prose-sm dark:prose-invert max-w-none prose-p:my-4 prose-p:leading-relaxed prose-ul:my-4 prose-ul:list-disc prose-ul:pl-6 prose-ol:my-4 prose-ol:list-decimal prose-ol:pl-6 prose-li:my-1.5 prose-pre:my-5 prose-strong:text-foreground prose-strong:font-bold prose-headings:text-foreground prose-headings:font-bold">
          <ReactMarkdown
            remarkPlugins={[remarkGfm, remarkBreaks]}
            components={{
              code: ({ className, children }) => {
                // Check if it's inline code (no className means inline, className with language-* means code block)
                const isInline = !className || !className.startsWith('language-');

                // Inline code (backticks) - with copy on click
                if (isInline) {
                  return <InlineCode>{children}</InlineCode>;
                }

                // Code blocks (triple backticks) - with syntax highlighting and copy button
                return <CodeBlock className={className}>{children}</CodeBlock>;
              },
              ol: ({ children, ...props }) => (
                <ol className="list-decimal pl-6" {...props}>
                  {children}
                </ol>
              ),
              ul: ({ children, ...props }) => (
                <ul className="list-disc pl-6" {...props}>
                  {children}
                </ul>
              ),
              li: ({ children, ...props }) => (
                <li className="my-1" {...props}>
                  {children}
                </li>
              ),
              p: ({ children, ...props }) => (
                <p className="leading-relaxed" {...props}>
                  {children}
                </p>
              ),
              h1: ({ children, ...props }) => (
                <p className="my-1 font-bold" {...props}>
                  {children}
                </p>
              ),
              h2: ({ children, ...props }) => (
                <p className="my-1 font-bold" {...props}>
                  {children}
                </p>
              ),
              h3: ({ children, ...props }) => (
                <p className="my-1 font-bold" {...props}>
                  {children}
                </p>
              ),
              h4: ({ children, ...props }) => (
                <p className="my-1 font-bold" {...props}>
                  {children}
                </p>
              ),
              h5: ({ children, ...props }) => (
                <p className="my-1 font-bold" {...props}>
                  {children}
                </p>
              ),
            }}
          >
            {comment.content || '(empty)'}
          </ReactMarkdown>
          {showRichBlocks ? <RichBlocks comment={comment} /> : null}
        </div>
      </div>
    </div>
  );
}
