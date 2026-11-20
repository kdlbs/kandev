"use client";

import { useMemo, type ReactNode } from "react";
import { MessageCommentSurface } from "@/components/task/chat/messages/message-comment-surface";
import { officeTaskCommentScope } from "@/lib/state/slices/comments";
import { sessionId as toSessionId, taskId as toTaskId, type Message } from "@/lib/types/http";

type CommentExtractSurfaceProps = {
  taskId: string;
  commentId: string;
  content: string;
  authorName: string;
  readOnly: boolean;
  children: ReactNode;
};

/**
 * Lets the reader select an extract of an Office thread comment and attach a remark to it.
 * Reuses the chat extract-comment surface; the Office comment ID is the anchor identity.
 */
export function CommentExtractSurface({
  taskId,
  commentId,
  content,
  authorName,
  readOnly,
  children,
}: CommentExtractSurfaceProps) {
  const message = useMemo<Message>(
    () => ({
      id: commentId,
      session_id: toSessionId(officeTaskCommentScope(taskId)),
      task_id: toTaskId(taskId),
      author_type: "agent",
      type: "message",
      content,
      created_at: "",
    }),
    [commentId, content, taskId],
  );
  return (
    <MessageCommentSurface
      message={message}
      sessionId={officeTaskCommentScope(taskId)}
      isTurnActive={false}
      authorName={authorName}
      allowRun={false}
      readOnly={readOnly}
    >
      {children}
    </MessageCommentSurface>
  );
}
