import type { Message, MessageType } from "@/lib/types/http";
import { messageTimestampNanoseconds } from "@/lib/state/slices/session/message-timestamp";

const AGENT_ACTIVITY_TYPES = new Set<MessageType>([
  "message",
  "content",
  "thinking",
  "tool_call",
  "tool_read",
  "tool_edit",
  "tool_execute",
  "tool_search",
  "agent_plan",
  "todo",
  "permission_request",
]);

export function hasAgentActivityAfterNotice(messages: Message[] | undefined, notice: Message) {
  if (
    notice.metadata?.running_notice_resolved === true ||
    messages?.some(
      (message) => message.id === notice.id && message.metadata?.running_notice_resolved === true,
    )
  )
    return true;
  const noticeAt = messageTimestampNanoseconds(notice.created_at);
  if (!notice.turn_id || noticeAt === null) return false;

  return (messages ?? []).some((message) => {
    if (
      message.id === notice.id ||
      message.session_id !== notice.session_id ||
      message.turn_id !== notice.turn_id ||
      message.author_type !== "agent" ||
      !AGENT_ACTIVITY_TYPES.has(message.type)
    ) {
      return false;
    }
    const createdAt = messageTimestampNanoseconds(message.created_at);
    const updatedAt = messageTimestampNanoseconds(message.updated_at);
    return (
      (createdAt !== null && createdAt > noticeAt) || (updatedAt !== null && updatedAt > noticeAt)
    );
  });
}

/** Preserve evidence from an update whose tool row is outside the loaded window. */
export function resolveRunningNotices(messages: Message[], activity: Message): void {
  for (const notice of messages) {
    if (
      notice.type === "status" &&
      notice.metadata?.action_visibility === "running" &&
      notice.metadata.running_notice_resolved !== true &&
      hasAgentActivityAfterNotice([activity], notice)
    ) {
      notice.metadata.running_notice_resolved = true;
    }
  }
}
