"use client";

import { memo } from "react";
import type { Message } from "@/lib/types/http";
import { RichBlocks } from "@/components/task/chat/messages/rich-blocks";
import { MessageActions } from "@/components/task/chat/messages/message-actions";
import { MemoizedMarkdown } from "@/components/shared/memoized-markdown";
import { MessageCommentSurface } from "./message-comment-surface";

type AgentMessageContentProps = {
  comment: Message;
  showRaw: boolean;
  onToggleRaw: () => void;
  showRichBlocks?: boolean;
  worktreePath?: string;
  onOpenFile?: (path: string) => void;
  sessionId?: string | null;
  isTurnActive: boolean;
};

export const AgentMessageContent = memo(function AgentMessageContent({
  comment,
  showRaw,
  onToggleRaw,
  showRichBlocks,
  worktreePath,
  onOpenFile,
  sessionId,
  isTurnActive,
}: AgentMessageContentProps) {
  return (
    <div className="flex items-start gap-2 sm:gap-3 w-full group">
      <div className="flex-1 min-w-0">
        {showRaw ? (
          <pre className="whitespace-pre-wrap font-mono text-xs bg-muted/20 p-3 rounded-md">
            {comment.raw_content || comment.content || "(empty)"}
          </pre>
        ) : (
          <MessageCommentSurface
            message={comment}
            sessionId={sessionId}
            isTurnActive={isTurnActive}
          >
            <div className="markdown-body max-w-none">
              <MemoizedMarkdown
                content={comment.content || "(empty)"}
                worktreePath={worktreePath}
                onOpenFile={onOpenFile}
              />
            </div>
          </MessageCommentSurface>
        )}
        {!showRaw && showRichBlocks ? <RichBlocks comment={comment} /> : null}
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
});
