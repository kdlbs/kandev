"use client";

import {
  Children,
  memo,
  useState,
  useCallback,
  useMemo,
  type ComponentPropsWithoutRef,
  type ReactNode,
} from "react";
import type { Components } from "react-markdown";
import { IconWand, IconMessageDots, IconFile } from "@tabler/icons-react";
import { cn } from "@/lib/utils";
import type { Message } from "@/lib/types/http";
import { RichBlocks } from "@/components/task/chat/messages/rich-blocks";
import { MessageActions } from "@/components/task/chat/messages/message-actions";
import { useUserMessageNavigation } from "@/hooks/use-message-navigation";
import { SenderTaskBadge, type SenderTaskInfo } from "./sender-task-badge";
import { MemoizedMarkdown } from "@/components/shared/memoized-markdown";
import { markdownComponents } from "@/components/shared/markdown-components";
import { ImagePreviewDialog } from "@/components/task/chat/image-preview-dialog";
import { useAppStore } from "@/components/state-provider";
import { HoverCard, HoverCardTrigger, HoverCardContent } from "@kandev/ui/hover-card";
import { PromptPreview } from "@/components/task/chat/context-items/prompt-preview";
import {
  buildPromptMentionNames,
  splitPreparedPromptMentionSegments,
} from "@/lib/prompts/prompt-mention-segments";
import {
  WorkflowStepMessageBadge,
  workflowMessageInfoFromMetadata,
  type WorkflowMessageMetadata,
  type WorkflowStepMessageInfo,
} from "./workflow-step-message-badge";

type ChatMessageProps = {
  comment: Message;
  label: string;
  className: string;
  showRichBlocks?: boolean;
  sessionId?: string | null;
  worktreePath?: string;
  onOpenFile?: (path: string) => void;
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

type UserMessageBodyOptions = {
  hasContent: boolean;
  showRaw: boolean;
  hasAttachments: boolean;
  content: string;
  rawContent?: string;
  promptMentionComponents?: Components;
  worktreePath?: string;
  onOpenFile?: (path: string) => void;
};

function renderUserMessageBody({
  hasContent,
  showRaw,
  hasAttachments,
  content,
  rawContent,
  promptMentionComponents,
  worktreePath,
  onOpenFile,
}: UserMessageBodyOptions): React.ReactNode {
  if (hasContent && showRaw) {
    return <pre className="whitespace-pre-wrap font-mono text-xs">{rawContent || content}</pre>;
  }
  if (hasContent) {
    return (
      <div className="markdown-body markdown-body-user max-w-none">
        <MemoizedMarkdown
          content={content}
          components={promptMentionComponents}
          worktreePath={worktreePath}
          onOpenFile={onOpenFile}
        />
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
  sessionId?: string | null;
  worktreePath?: string;
  onOpenFile?: (path: string) => void;
  onScrollToMessage?: (messageId: string) => void;
};

type UserMessageMetadata = WorkflowMessageMetadata & {
  attachments?: Array<{ type: string; data: string; mime_type: string; name?: string }>;
  plan_mode?: boolean;
  has_review_comments?: boolean;
  has_hidden_prompts?: boolean;
  context_files?: Array<{ path: string; name: string }>;
  sender_task_id?: string;
  sender_task_title?: string;
  sender_session_id?: string;
  sender_session_name?: string;
};

type PromptMentionMarkdownTag =
  | "p"
  | "li"
  | "h1"
  | "h2"
  | "h3"
  | "h4"
  | "h5"
  | "h6"
  | "blockquote"
  | "td"
  | "th";

type MarkdownChildrenProps<T extends PromptMentionMarkdownTag> = ComponentPropsWithoutRef<T> & {
  children?: ReactNode;
  node?: unknown;
};

function usePromptMentionNames() {
  const prompts = useAppStore((state) => state.prompts.items);
  return useMemo(() => prompts.map((prompt) => prompt.name), [prompts]);
}

function usePromptMentionMarkdownComponents(promptNames: string[]): Components | undefined {
  return useMemo(() => {
    const mentionNames = buildPromptMentionNames(promptNames);
    if (mentionNames.length === 0) return undefined;
    const renderChildren = (children: ReactNode, keyPrefix: string) =>
      renderChildrenWithPromptMentions(children, mentionNames, keyPrefix);
    return {
      ...markdownComponents,
      p: ({ children, node, ...props }: MarkdownChildrenProps<"p">) => {
        void node;
        return <p {...props}>{renderChildren(children, "p")}</p>;
      },
      li: ({ children, node, ...props }: MarkdownChildrenProps<"li">) => {
        void node;
        return <li {...props}>{renderChildren(children, "li")}</li>;
      },
      h1: ({ children, node, ...props }: MarkdownChildrenProps<"h1">) => {
        void node;
        return <h1 {...props}>{renderChildren(children, "h1")}</h1>;
      },
      h2: ({ children, node, ...props }: MarkdownChildrenProps<"h2">) => {
        void node;
        return <h2 {...props}>{renderChildren(children, "h2")}</h2>;
      },
      h3: ({ children, node, ...props }: MarkdownChildrenProps<"h3">) => {
        void node;
        return <h3 {...props}>{renderChildren(children, "h3")}</h3>;
      },
      h4: ({ children, node, ...props }: MarkdownChildrenProps<"h4">) => {
        void node;
        return <h4 {...props}>{renderChildren(children, "h4")}</h4>;
      },
      h5: ({ children, node, ...props }: MarkdownChildrenProps<"h5">) => {
        void node;
        return <h5 {...props}>{renderChildren(children, "h5")}</h5>;
      },
      h6: ({ children, node, ...props }: MarkdownChildrenProps<"h6">) => {
        void node;
        return <h6 {...props}>{renderChildren(children, "h6")}</h6>;
      },
      blockquote: ({ children, node, ...props }: MarkdownChildrenProps<"blockquote">) => {
        void node;
        return <blockquote {...props}>{renderChildren(children, "blockquote")}</blockquote>;
      },
      td: ({ children, node, ...props }: MarkdownChildrenProps<"td">) => {
        void node;
        return <td {...props}>{renderChildren(children, "td")}</td>;
      },
      th: ({ children, node, ...props }: MarkdownChildrenProps<"th">) => {
        void node;
        return <th {...props}>{renderChildren(children, "th")}</th>;
      },
    };
  }, [promptNames]);
}

function renderChildrenWithPromptMentions(
  children: ReactNode,
  promptNames: string[],
  keyPrefix: string,
) {
  return Children.toArray(children).flatMap((child, index) => {
    if (typeof child !== "string") return child;
    return renderTextWithPromptMentions(child, promptNames, `${keyPrefix}-${index}`);
  });
}

function renderTextWithPromptMentions(text: string, promptNames: string[], keyPrefix: string) {
  return splitPreparedPromptMentionSegments(text, promptNames).map((segment, index) => {
    if (segment.kind === "text") return segment.value;
    return (
      <PromptMentionChip
        key={`${keyPrefix}-prompt-${index}`}
        name={segment.name}
        value={segment.value}
      />
    );
  });
}

const PROMPT_MENTION_CHIP_CLASS =
  "inline rounded-md border border-emerald-300/35 bg-emerald-400/20 px-1.5 py-0.5 font-mono text-[0.88em] font-semibold text-emerald-950 box-decoration-clone break-all dark:text-emerald-100";

/**
 * Renders a saved-prompt @mention as a chip. When the referenced prompt is
 * loaded in the store, hovering the chip reveals its contents so the reader
 * doesn't need to switch to the raw message view.
 */
function PromptMentionChip({ name, value }: { name: string; value: string }) {
  const content = useAppStore(
    useCallback(
      (state) => state.prompts.items.find((prompt) => prompt.name === name)?.content ?? null,
      [name],
    ),
  );

  if (!content) {
    return (
      <span
        data-testid="custom-prompt-mention"
        data-prompt-name={name}
        title={`Custom prompt: ${name}`}
        className={PROMPT_MENTION_CHIP_CLASS}
      >
        {value}
      </span>
    );
  }

  return (
    <HoverCard openDelay={300} closeDelay={0}>
      <HoverCardTrigger asChild>
        <span
          data-testid="custom-prompt-mention"
          data-prompt-name={name}
          tabIndex={0}
          className={cn(PROMPT_MENTION_CHIP_CLASS, "cursor-default")}
        >
          {value}
        </span>
      </HoverCardTrigger>
      <HoverCardContent side="top" align="start" className="w-80 max-h-80 overflow-y-auto">
        <PromptPreview content={content} />
      </HoverCardContent>
    </HoverCard>
  );
}

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
    ? {
        id: metadata.sender_task_id,
        snapshotTitle: metadata.sender_task_title || "",
        sessionId: metadata.sender_session_id,
        sessionName: metadata.sender_session_name,
      }
    : null;
  const workflowMessage = workflowMessageInfoFromMetadata(metadata);
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
    workflowMessage,
  };
}

function UserContextBadges({
  hasPlanMode,
  hasReviewComments,
  contextFiles,
  senderTask,
  workflowMessage,
}: {
  hasPlanMode: boolean;
  hasReviewComments: boolean;
  contextFiles: Array<{ path: string; name: string }>;
  senderTask: SenderTaskInfo | null;
  workflowMessage: WorkflowStepMessageInfo | null;
}) {
  if (
    !hasPlanMode &&
    !hasReviewComments &&
    contextFiles.length === 0 &&
    !senderTask &&
    !workflowMessage
  )
    return null;
  return (
    <div className="flex justify-end gap-1.5 mb-1 flex-wrap">
      {workflowMessage && <WorkflowStepMessageBadge workflow={workflowMessage} />}
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

function UserMessageContent({
  comment,
  showRaw,
  onToggleRaw,
  sessionId,
  worktreePath,
  onOpenFile,
  onScrollToMessage,
}: UserMessageProps) {
  const userNavigation = useUserMessageNavigation(sessionId ?? null, comment.id);
  const promptNames = usePromptMentionNames();
  const promptMentionComponents = usePromptMentionMarkdownComponents(promptNames);
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
    workflowMessage,
  } = parseUserMessageMetadata(comment);

  return (
    <div className="flex justify-end w-full overflow-hidden">
      <div className="max-w-[85%] sm:max-w-[75%] md:max-w-2xl overflow-hidden group">
        <UserContextBadges
          hasPlanMode={hasPlanMode}
          hasReviewComments={hasReviewComments}
          contextFiles={contextFiles}
          senderTask={senderTask}
          workflowMessage={workflowMessage}
        />
        <div className="rounded-2xl bg-primary/30 px-4 py-2.5 overflow-hidden">
          {hasAttachments && (
            <div className={cn("flex flex-wrap gap-2", hasContent && "mb-2")}>
              {imageAttachments.map((att, index) => (
                <ImagePreviewDialog
                  key={index}
                  src={`data:${att.mime_type};base64,${att.data}`}
                  alt={`Attachment ${index + 1}`}
                  thumbnailClassName="max-h-48 max-w-full rounded-lg object-contain transition-opacity hover:opacity-90"
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
          {renderUserMessageBody({
            hasContent,
            showRaw,
            hasAttachments,
            content: comment.content,
            rawContent: comment.raw_content,
            promptMentionComponents,
            worktreePath,
            onOpenFile,
          })}
        </div>
        <MessageActions
          message={comment}
          showCopy={true}
          showTimestamp={true}
          showRawToggle={true}
          hasHiddenPrompts={hasHiddenPrompts}
          showNavigation={userNavigation.hasPrevious || userNavigation.hasNext}
          isRawView={showRaw}
          onToggleRaw={onToggleRaw}
          onNavigatePrev={() => {
            if (userNavigation.previousId && onScrollToMessage)
              onScrollToMessage(userNavigation.previousId);
          }}
          onNavigateNext={() => {
            if (userNavigation.nextId && onScrollToMessage)
              onScrollToMessage(userNavigation.nextId);
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
  worktreePath?: string;
  onOpenFile?: (path: string) => void;
};

function AgentMessageContent({
  comment,
  showRaw,
  onToggleRaw,
  showRichBlocks,
  worktreePath,
  onOpenFile,
}: AgentMessageProps) {
  return (
    <div className="flex items-start gap-2 sm:gap-3 w-full group">
      <div className="flex-1 min-w-0">
        {showRaw ? (
          <pre className="whitespace-pre-wrap font-mono text-xs bg-muted/20 p-3 rounded-md">
            {comment.raw_content || comment.content || "(empty)"}
          </pre>
        ) : (
          <div className="markdown-body max-w-none">
            <MemoizedMarkdown
              content={comment.content || "(empty)"}
              worktreePath={worktreePath}
              onOpenFile={onOpenFile}
            />
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
  sessionId,
  worktreePath,
  onOpenFile,
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
        sessionId={sessionId}
        worktreePath={worktreePath}
        onOpenFile={onOpenFile}
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
      worktreePath={worktreePath}
      onOpenFile={onOpenFile}
    />
  );
});
