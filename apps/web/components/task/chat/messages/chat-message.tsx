"use client";

import { memo, useState, useCallback } from "react";
import Link from "next/link";
import ReactMarkdown from "react-markdown";
import { IconWand, IconMessageDots, IconFile, IconRobot } from "@tabler/icons-react";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@kandev/ui/tooltip";
import { cn } from "@/lib/utils";
import type { Message } from "@/lib/types/http";
import { RichBlocks } from "@/components/task/chat/messages/rich-blocks";
import { MessageActions } from "@/components/task/chat/messages/message-actions";
import { useMessageNavigation } from "@/hooks/use-message-navigation";
import { useTaskById } from "@/hooks/domains/kanban/use-task-by-id";
import { linkToTask } from "@/lib/links";
import { markdownComponents, remarkPlugins } from "@/components/shared/markdown-components";

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
        className="px-1 py-0.5 bg-muted text-accent rounded font-mono text-[0.85em]"
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

// ── Markdown component overrides imported from shared/markdown-components ─────

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
      <div className="markdown-body markdown-body-user max-w-none">
        <ReactMarkdown remarkPlugins={remarkPlugins} components={markdownComponents}>
          {content}
        </ReactMarkdown>
      </div>
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
  attachments?: Array<{ type: string; data: string; mime_type: string; name?: string }>;
  plan_mode?: boolean;
  has_review_comments?: boolean;
  has_hidden_prompts?: boolean;
  context_files?: Array<{ path: string; name: string }>;
  sender_task_id?: string;
  sender_task_title?: string;
  sender_session_id?: string;
};

type SenderTaskInfo = {
  id: string;
  // Snapshot title captured when the message was sent. May differ from the
  // task's current title; used as a fallback when the live task isn't loaded.
  snapshotTitle: string;
};

function parseUserMessageMetadata(comment: Message) {
  const metadata = comment.metadata as UserMessageMetadata | undefined;
  const imageAttachments = (metadata?.attachments || []).filter((att) => att.type === "image");
  const fileAttachments = (metadata?.attachments || []).filter((att) => att.type === "resource");
  const contextFiles = metadata?.context_files || [];
  const hasPlanMode = !!metadata?.plan_mode;
  const hasReviewComments = !!metadata?.has_review_comments;
  const hasHiddenPrompts = !!metadata?.has_hidden_prompts;
  const hasContent = !!(comment.content && comment.content.trim() !== "");
  const hasAttachments = imageAttachments.length > 0 || fileAttachments.length > 0;
  const senderTask: SenderTaskInfo | null = metadata?.sender_task_id
    ? { id: metadata.sender_task_id, snapshotTitle: metadata.sender_task_title || "" }
    : null;
  return {
    imageAttachments,
    fileAttachments,
    contextFiles,
    hasPlanMode,
    hasReviewComments,
    hasHiddenPrompts,
    hasContent,
    hasAttachments,
    senderTask,
  };
}

const SENDER_TITLE_MAX = 24;

function truncateTitle(title: string): string {
  if (title.length <= SENDER_TITLE_MAX) return title;
  return title.slice(0, SENDER_TITLE_MAX - 1).trimEnd() + "…";
}

function SenderTaskBadge({ sender }: { sender: SenderTaskInfo }) {
  // Live-resolve the sender task from the loaded kanban state so the badge
  // reflects renames. When the sender task isn't loaded (cross-workspace,
  // archived, etc.) we fall back to the snapshot title and render a static,
  // non-clickable greyed-out badge — the source URL only works when we have
  // routing context.
  const liveTask = useTaskById(sender.id);
  const liveTitle = liveTask?.title || "";
  const displayTitle = liveTitle || sender.snapshotTitle || "(unknown task)";
  const fullTitle = liveTitle || sender.snapshotTitle || "(unknown task)";
  const truncated = truncateTitle(displayTitle);

  const inner = (
    <span
      className={cn(
        "inline-flex items-center gap-1.5 rounded-full bg-purple-500/20 px-2.5 py-1 text-xs font-medium text-purple-300",
        liveTask && "cursor-pointer hover:bg-purple-500/30 transition-colors",
        !liveTask && "opacity-60",
      )}
      data-testid="sender-task-badge"
      data-sender-task-id={sender.id}
    >
      <IconRobot size={14} /> {truncated}
    </span>
  );

  const wrapped = liveTask ? (
    <Link href={linkToTask(sender.id)} aria-label={`Open source task ${fullTitle}`}>
      {inner}
    </Link>
  ) : (
    inner
  );

  return (
    <TooltipProvider delayDuration={300}>
      <Tooltip>
        <TooltipTrigger asChild>{wrapped}</TooltipTrigger>
        <TooltipContent>
          From agent in task <span className="font-semibold">&ldquo;{fullTitle}&rdquo;</span>
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}

function UserContextBadges({
  hasPlanMode,
  hasReviewComments,
  contextFiles,
  senderTask,
}: {
  hasPlanMode: boolean;
  hasReviewComments: boolean;
  contextFiles: Array<{ path: string; name: string }>;
  senderTask: SenderTaskInfo | null;
}) {
  if (!hasPlanMode && !hasReviewComments && contextFiles.length === 0 && !senderTask) return null;
  return (
    <div className="flex justify-end gap-1.5 mb-1 flex-wrap">
      {senderTask && <SenderTaskBadge sender={senderTask} />}
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
    fileAttachments,
    contextFiles,
    hasPlanMode,
    hasReviewComments,
    hasHiddenPrompts,
    hasContent,
    hasAttachments,
    senderTask,
  } = parseUserMessageMetadata(comment);

  return (
    <div className="flex justify-end w-full overflow-hidden">
      <div className="max-w-[85%] sm:max-w-[75%] md:max-w-2xl overflow-hidden group">
        <UserContextBadges
          hasPlanMode={hasPlanMode}
          hasReviewComments={hasReviewComments}
          contextFiles={contextFiles}
          senderTask={senderTask}
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
              {fileAttachments.map((att, index) => (
                <span
                  key={`file-${index}`}
                  className="inline-flex items-center gap-1.5 rounded-full bg-muted/40 px-2.5 py-1 text-xs text-muted-foreground"
                >
                  <IconFile size={12} />
                  {att.name || "Attachment"}
                </span>
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
          <div className="markdown-body max-w-none">
            <ReactMarkdown remarkPlugins={remarkPlugins} components={markdownComponents}>
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
