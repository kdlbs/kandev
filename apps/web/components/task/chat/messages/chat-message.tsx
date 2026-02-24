"use client";

import { memo, isValidElement, useState, useCallback, type ReactNode } from "react";
import ReactMarkdown from "react-markdown";
import remarkBreaks from "remark-breaks";
import remarkGfm from "remark-gfm";
import { IconWand, IconMessageDots, IconFile } from "@tabler/icons-react";
import { cn } from "@/lib/utils";
import type { Message } from "@/lib/types/http";
import { RichBlocks } from "@/components/task/chat/messages/rich-blocks";
import { InlineCode } from "@/components/task/chat/messages/inline-code";
import { CodeBlock } from "@/components/task/chat/messages/code-block";
import { MessageActions } from "@/components/task/chat/messages/message-actions";
import { useMessageNavigation } from "@/hooks/use-message-navigation";

/**
 * Recursively extracts text content from React children.
 * Optimized with fast paths for common cases (string/number).
 */
function getTextContent(children: ReactNode): string {
  // Fast path: most common cases first
  if (typeof children === "string") return children;
  if (typeof children === "number") return String(children);
  if (children == null) return "";

  // Use loop instead of map().join() to avoid intermediate array allocation
  if (Array.isArray(children)) {
    let result = "";
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
  return "";
}

type ChatMessageProps = {
  comment: Message;
  label: string;
  className: string;
  showRichBlocks?: boolean;
  allMessages?: Message[];
  onScrollToMessage?: (messageId: string) => void;
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
        className="px-1 py-0.5 bg-foreground/[0.06] text-foreground/80 rounded font-mono text-[0.9em]"
      >
        @{filePath}
      </code>,
    );

    lastIndex = match.index + match[0].length;
  }

  // Add remaining text after last match
  if (lastIndex < content.length) {
    parts.push(content.slice(lastIndex));
  }

  return parts.length > 0 ? parts : [content];
}

// ── Markdown component overrides (stable reference) ─────────────────

const markdownComponents = {
  code: ({ className, children }: { className?: string; children?: ReactNode }) => {
    const content = getTextContent(children).replace(/\n$/, "");
    const hasLanguage = className?.startsWith("language-");
    const hasNewlines = content.includes("\n");
    if (hasLanguage || hasNewlines) {
      return <CodeBlock className={className}>{content}</CodeBlock>;
    }
    return <InlineCode>{content}</InlineCode>;
  },
  ol: ({ children }: { children?: ReactNode }) => (
    <ol className="list-decimal pl-6 mb-2">{children}</ol>
  ),
  ul: ({ children }: { children?: ReactNode }) => (
    <ul className="list-disc pl-6 mb-2">{children}</ul>
  ),
  li: ({ children }: { children?: ReactNode }) => <li className="my-0.5">{children}</li>,
  p: ({ children }: { children?: ReactNode }) => (
    <p className="leading-relaxed mb-1.5">{children}</p>
  ),
  h1: ({ children }: { children?: ReactNode }) => (
    <p className="my-4 font-bold text-sm">{children}</p>
  ),
  h2: ({ children }: { children?: ReactNode }) => (
    <p className="my-3 font-bold text-sm">{children}</p>
  ),
  h3: ({ children }: { children?: ReactNode }) => (
    <p className="my-2 font-bold text-sm">{children}</p>
  ),
  h4: ({ children }: { children?: ReactNode }) => <p className="my-2 font-bold">{children}</p>,
  h5: ({ children }: { children?: ReactNode }) => <p className="my-2 font-bold">{children}</p>,
  hr: ({ children }: { children?: ReactNode }) => <hr className="my-5">{children}</hr>,
  table: ({ children }: { children?: ReactNode }) => (
    <div className="my-3 overflow-x-auto">
      <table className="border-collapse border border-border rounded-lg overflow-hidden">
        {children}
      </table>
    </div>
  ),
  thead: ({ children }: { children?: ReactNode }) => (
    <thead className="bg-muted/50">{children}</thead>
  ),
  tbody: ({ children }: { children?: ReactNode }) => (
    <tbody className="divide-y divide-border">{children}</tbody>
  ),
  tr: ({ children }: { children?: ReactNode }) => (
    <tr className="border-b border-border last:border-b-0 hover:bg-muted/50">{children}</tr>
  ),
  th: ({ children }: { children?: ReactNode }) => (
    <th className="px-3 py-2 text-left text-xs font-semibold text-foreground border-r border-border last:border-r-0">
      {children}
    </th>
  ),
  td: ({ children }: { children?: ReactNode }) => (
    <td className="px-3 py-2 text-xs text-muted-foreground border-r border-border last:border-r-0">
      {children}
    </td>
  ),
};

function renderUserMessageBody(
  hasContent: boolean,
  showRaw: boolean,
  hasAttachments: boolean,
  content: string,
  rawContent?: string,
): React.ReactNode {
  if (hasContent && showRaw) {
    return <pre className="whitespace-pre-wrap font-mono text-xs">{rawContent || content}</pre>;
  }
  if (hasContent) {
    return (
      <p className="whitespace-pre-wrap break-words overflow-wrap-anywhere">
        {renderContentWithFileRefs(content)}
      </p>
    );
  }
  if (!hasAttachments) {
    return <p className="whitespace-pre-wrap break-words overflow-wrap-anywhere">(empty)</p>;
  }
  return null;
}

// ── User message sub-component ──────────────────────────────────────

type UserMessageProps = {
  comment: Message;
  showRaw: boolean;
  onToggleRaw: () => void;
  allMessages?: Message[];
  onScrollToMessage?: (messageId: string) => void;
};

type UserMessageMetadata = {
  attachments?: Array<{ type: string; data: string; mime_type: string }>;
  plan_mode?: boolean;
  has_review_comments?: boolean;
  has_hidden_prompts?: boolean;
  context_files?: Array<{ path: string; name: string }>;
};

function parseUserMessageMetadata(comment: Message) {
  const metadata = comment.metadata as UserMessageMetadata | undefined;
  const imageAttachments = (metadata?.attachments || []).filter((att) => att.type === "image");
  const contextFiles = metadata?.context_files || [];
  const hasPlanMode = !!metadata?.plan_mode;
  const hasReviewComments = !!metadata?.has_review_comments;
  const hasHiddenPrompts = !!metadata?.has_hidden_prompts;
  const hasContent = !!(comment.content && comment.content.trim() !== "");
  const hasAttachments = imageAttachments.length > 0;
  return {
    imageAttachments,
    contextFiles,
    hasPlanMode,
    hasReviewComments,
    hasHiddenPrompts,
    hasContent,
    hasAttachments,
  };
}

function UserContextBadges({
  hasPlanMode,
  hasReviewComments,
  contextFiles,
}: {
  hasPlanMode: boolean;
  hasReviewComments: boolean;
  contextFiles: Array<{ path: string; name: string }>;
}) {
  if (!hasPlanMode && !hasReviewComments && contextFiles.length === 0) return null;
  return (
    <div className="flex justify-end gap-1.5 mb-1 flex-wrap">
      {hasPlanMode && (
        <span className="inline-flex items-center gap-1 rounded-full bg-slate-500/20 px-2 py-0.5 text-[10px] text-slate-400">
          <IconWand size={10} /> Plan mode
        </span>
      )}
      {hasReviewComments && (
        <span className="inline-flex items-center gap-1 rounded-full bg-blue-500/20 px-2 py-0.5 text-[10px] text-blue-400">
          <IconMessageDots size={10} /> Review comments
        </span>
      )}
      {contextFiles.map((f) => (
        <span
          key={f.path}
          className="inline-flex items-center gap-1 rounded-full bg-muted/50 px-2 py-0.5 text-[10px] text-muted-foreground"
        >
          <IconFile size={10} /> {f.name}
        </span>
      ))}
    </div>
  );
}

function openImageInWindow(mimeType: string, data: string) {
  const win = window.open();
  if (win) {
    win.document.write(
      `<img src="data:${mimeType};base64,${data}" style="max-width:100%;height:auto;" />`,
    );
  }
}

function UserMessageContent({
  comment,
  showRaw,
  onToggleRaw,
  allMessages,
  onScrollToMessage,
}: UserMessageProps) {
  const userNavigation = useMessageNavigation(allMessages || [], comment.id, "user");
  const {
    imageAttachments,
    contextFiles,
    hasPlanMode,
    hasReviewComments,
    hasHiddenPrompts,
    hasContent,
    hasAttachments,
  } = parseUserMessageMetadata(comment);

  return (
    <div className="flex justify-end w-full overflow-hidden">
      <div className="max-w-[85%] sm:max-w-[75%] md:max-w-2xl overflow-hidden group">
        <UserContextBadges
          hasPlanMode={hasPlanMode}
          hasReviewComments={hasReviewComments}
          contextFiles={contextFiles}
        />
        <div className="rounded-2xl bg-primary/30 px-4 py-2.5 overflow-hidden">
          {hasAttachments && (
            <div className={cn("flex flex-wrap gap-2", hasContent && "mb-2")}>
              {imageAttachments.map((att, index) => (
                /* eslint-disable-next-line @next/next/no-img-element -- base64 data URLs are not compatible with next/image */
                <img
                  key={index}
                  src={`data:${att.mime_type};base64,${att.data}`}
                  alt={`Attachment ${index + 1}`}
                  className="max-h-48 max-w-full rounded-lg object-contain cursor-pointer hover:opacity-90 transition-opacity"
                  onClick={() => openImageInWindow(att.mime_type, att.data)}
                />
              ))}
            </div>
          )}
          {renderUserMessageBody(
            hasContent,
            showRaw,
            hasAttachments,
            comment.content,
            comment.raw_content,
          )}
        </div>
        <MessageActions
          message={comment}
          showCopy={true}
          showTimestamp={true}
          showRawToggle={true}
          hasHiddenPrompts={hasHiddenPrompts}
          showNavigation={!!allMessages && allMessages.length > 0}
          isRawView={showRaw}
          onToggleRaw={onToggleRaw}
          onNavigatePrev={() => {
            if (userNavigation.previous && onScrollToMessage)
              onScrollToMessage(userNavigation.previous.id);
          }}
          onNavigateNext={() => {
            if (userNavigation.next && onScrollToMessage) onScrollToMessage(userNavigation.next.id);
          }}
          hasPrev={userNavigation.hasPrevious}
          hasNext={userNavigation.hasNext}
        />
      </div>
    </div>
  );
}

// ── Agent message sub-component ─────────────────────────────────────

type AgentMessageProps = {
  comment: Message;
  showRaw: boolean;
  onToggleRaw: () => void;
  showRichBlocks?: boolean;
};

function AgentMessageContent({ comment, showRaw, onToggleRaw, showRichBlocks }: AgentMessageProps) {
  return (
    <div className="flex items-start gap-2 sm:gap-3 w-full group">
      <div className="flex-1 min-w-0">
        {showRaw ? (
          <pre className="whitespace-pre-wrap font-mono text-xs bg-muted/20 p-3 rounded-md">
            {comment.content || "(empty)"}
          </pre>
        ) : (
          <div className="prose prose-sm dark:prose-invert max-w-none prose-p:my-4 prose-p:leading-relaxed prose-ul:my-4 prose-ul:list-disc prose-ul:pl-6 prose-ol:my-4 prose-ol:list-decimal prose-ol:pl-6 prose-li:my-1.5 prose-pre:my-5 prose-strong:text-foreground prose-strong:font-bold prose-headings:text-foreground prose-headings:font-bold">
            <ReactMarkdown
              remarkPlugins={[remarkGfm, remarkBreaks]}
              components={markdownComponents}
            >
              {comment.content || "(empty)"}
            </ReactMarkdown>
            {showRichBlocks ? <RichBlocks comment={comment} /> : null}
          </div>
        )}
        <MessageActions
          message={comment}
          showCopy={true}
          showTimestamp={true}
          showRawToggle={true}
          showModel={true}
          showNavigation={false}
          isRawView={showRaw}
          onToggleRaw={onToggleRaw}
        />
      </div>
    </div>
  );
}

// ── Main component ──────────────────────────────────────────────────

export const ChatMessage = memo(function ChatMessage({
  comment,
  label,
  className,
  showRichBlocks,
  allMessages,
  onScrollToMessage,
}: ChatMessageProps) {
  const [showRaw, setShowRaw] = useState(false);
  const toggleRaw = useCallback(() => setShowRaw((v) => !v), []);

  // Keep the old card-based layout for task descriptions (amber banner)
  if (label === "Task") {
    return (
      <div className={cn("w-full rounded-lg px-4 py-3", className)}>
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
          {comment.content ? renderContentWithFileRefs(comment.content) : "(empty)"}
        </p>
      </div>
    );
  }

  if (comment.author_type === "user") {
    return (
      <UserMessageContent
        comment={comment}
        showRaw={showRaw}
        onToggleRaw={toggleRaw}
        allMessages={allMessages}
        onScrollToMessage={onScrollToMessage}
      />
    );
  }

  return (
    <AgentMessageContent
      comment={comment}
      showRaw={showRaw}
      onToggleRaw={toggleRaw}
      showRichBlocks={showRichBlocks}
    />
  );
});
