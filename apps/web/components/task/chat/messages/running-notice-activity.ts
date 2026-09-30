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
