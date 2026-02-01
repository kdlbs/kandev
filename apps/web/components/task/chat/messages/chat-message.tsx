'use client';

import { memo, isValidElement, type ReactNode } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkBreaks from 'remark-breaks';
import remarkGfm from 'remark-gfm';
import { cn } from '@/lib/utils';
import type { Message } from '@/lib/types/http';
import { RichBlocks } from '@/components/task/chat/messages/rich-blocks';
import { InlineCode } from '@/components/task/chat/messages/inline-code';
import { CodeBlock } from '@/components/task/chat/messages/code-block';

/**
 * Recursively extracts text content from React children.
 * Optimized with fast paths for common cases (string/number).
 */
function getTextContent(children: ReactNode): string {
  // Fast path: most common cases first
  if (typeof children === 'string') return children;
  if (typeof children === 'number') return String(children);
  if (children == null) return '';

  // Use loop instead of map().join() to avoid intermediate array allocation
  if (Array.isArray(children)) {
    let result = '';
    for (let i = 0; i < children.length; i++) {
      result += getTextContent(children[i]);
    }
    return result;
  }

  if (isValidElement(children)) {
    const props = children.props as { children?: ReactNode };
    if (props.children) {
      return getTextContent(props.children);
    }
  }
  return '';
}

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

export const ChatMessage = memo(function ChatMessage({ comment, label, className, showRichBlocks }: ChatMessageProps) {
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

  // User message: right-aligned bubble with markdown support
  if (isUser) {
    return (
      <div className="flex justify-end w-full overflow-hidden">
        <div className="max-w-[85%] sm:max-w-[75%] md:max-w-2xl overflow-hidden">
          <div className="rounded-2xl bg-primary/30 px-4 py-2.5 text-xs overflow-hidden">
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
                const content = getTextContent(children).replace(/\n$/, '');
                const hasLanguage = className?.startsWith('language-');
                const hasNewlines = content.includes('\n');

                // Code block if it has a language specifier OR has multiple lines
                if (hasLanguage || hasNewlines) {
                  return <CodeBlock className={className}>{content}</CodeBlock>;
                }

                // Inline code (single backticks, single line)
                return <InlineCode>{content}</InlineCode>;
              },
              ol: ({ children }) => (
                <ol className="list-decimal pl-6 my-2">
                  {children}
                </ol>
              ),
              ul: ({ children }) => (
                <ul className="list-disc pl-6 my-1">
                  {children}
                </ul>
              ),
              li: ({ children }) => (
                <li className="my-0.5">
                  {children}
                </li>
              ),
              p: ({ children }) => (
                <p className="leading-relaxed my-0.5">
                  {children}
                </p>
              ),
              h1: ({ children }) => (
                <p className="my-3 font-bold text-[13px]">
                  {children}
                </p>
              ),
              h2: ({ children }) => (
                <p className="my-2 font-bold text-[13px]">
                  {children}
                </p>
              ),
              h3: ({ children }) => (
                <p className="my-2 font-bold">
                  {children}
                </p>
              ),
              h4: ({ children }) => (
                <p className="my-2 font-bold">
                  {children}
                </p>
              ),
              h5: ({ children }) => (
                <p className="my-2 font-bold">
                  {children}
                </p>
              ),
              table: ({ children }) => (
                <div className="my-3 overflow-x-auto">
                  <table className="border-collapse border border-border rounded-lg overflow-hidden">
                    {children}
                  </table>
                </div>
              ),
              thead: ({ children }) => (
                <thead className="bg-muted/50">
                  {children}
                </thead>
              ),
              tbody: ({ children }) => (
                <tbody className="divide-y divide-border">
                  {children}
                </tbody>
              ),
              tr: ({ children }) => (
                <tr className="border-b border-border last:border-b-0 hover:bg-muted/50">
                  {children}
                </tr>
              ),
              th: ({ children }) => (
                <th className="px-3 py-2 text-left text-xs font-semibold text-foreground border-r border-border last:border-r-0">
                  {children}
                </th>
              ),
              td: ({ children }) => (
                <td className="px-3 py-2 text-xs text-muted-foreground border-r border-border last:border-r-0">
                  {children}
                </td>
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
});
