import type { RenderItem } from "@/hooks/use-processed-messages";
import type { Message } from "@/lib/types/http";

function isAsyncQuestionRequest(message: Message): boolean {
  return message.type === "agent_question_request";
}

export function filterAsyncQuestionMessages(
  messages: Message[],
  asyncQuestionsEnabled: boolean,
): Message[] {
  return asyncQuestionsEnabled
    ? messages
    : messages.filter((message) => !isAsyncQuestionRequest(message));
}

export function filterAsyncQuestionItems(
  items: RenderItem[],
  asyncQuestionsEnabled: boolean,
): RenderItem[] {
  if (asyncQuestionsEnabled) return items;
  return items.flatMap<RenderItem>((item): RenderItem[] => {
    if (item.type === "message") return isAsyncQuestionRequest(item.message) ? [] : [item];
    if (item.type !== "turn_group") return [item];
    const messages = filterAsyncQuestionMessages(item.messages, false);
    if (messages.length === 0 && !item.activityChip) return [];
    return messages.length === item.messages.length ? [item] : [{ ...item, messages }];
  });
}
