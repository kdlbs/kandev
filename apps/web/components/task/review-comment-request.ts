import type { ReviewComment } from "@/lib/state/slices/comments";
import {
  commentAttachmentsForDelivery,
  formatReviewCommentsAsMarkdown,
} from "@/lib/state/slices/comments/format";
import { generateUUID } from "@/lib/utils";

export type ReviewFileAdmission = { key: string; id: string };

export function prepareReviewCommentRequest(
  taskId: string,
  sessionId: string,
  comments: ReviewComment[],
  pendingAdmission: { current: ReviewFileAdmission | null },
) {
  const attachments = commentAttachmentsForDelivery(comments);
  const content = formatReviewCommentsAsMarkdown(comments);
  const key = JSON.stringify([taskId, sessionId, content, attachments]);
  const messageId =
    pendingAdmission.current?.key === key ? pendingAdmission.current.id : generateUUID();
  pendingAdmission.current = { key, id: messageId };
  return {
    task_id: taskId,
    session_id: sessionId,
    client_message_id: messageId,
    content,
    ...(attachments.length > 0 && { attachments }),
  };
}
