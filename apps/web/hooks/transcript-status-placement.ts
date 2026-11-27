import type { Message } from "@/lib/types/http";
import type { RenderItem } from "./use-processed-messages";

function itemCreatedAt(item: RenderItem): string | undefined {
  if (item.type === "message") return item.message.created_at;
  if (item.type === "turn_group") return item.messages[item.messages.length - 1]?.created_at;
  return undefined;
}

export function insertionIndexAtTime(items: RenderItem[], occurredAt?: string): number {
  if (items.length === 0) return 0;
  const targetTime = occurredAt ? Date.parse(occurredAt) : Number.NaN;
  if (Number.isNaN(targetTime)) return items.length;
  let insertAt = 0;
  let sawTimestamp = false;
  for (let i = 0; i < items.length; i++) {
    const createdAt = itemCreatedAt(items[i]);
    if (!createdAt) continue;
    const itemTime = Date.parse(createdAt);
    if (Number.isNaN(itemTime)) continue;
    sawTimestamp = true;
    if (itemTime <= targetTime) insertAt = i + 1;
  }
  return sawTimestamp ? insertAt : items.length;
}

export function insertLocalStatusMessages(items: RenderItem[], messages: Message[] | undefined) {
  if (!messages?.length) return items;
  const timeline = [...items];
  for (const message of messages) {
    timeline.splice(insertionIndexAtTime(timeline, message.created_at), 0, {
      type: "message",
      message,
    });
  }
  return timeline;
}
