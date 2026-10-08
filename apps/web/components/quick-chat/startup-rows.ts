import { useMemo } from "react";
import type { Message } from "@/lib/types/http";
import type { RenderItem } from "@/hooks/use-processed-messages";
import { isSuccessfulAgentBootMessage } from "@/hooks/processed-message-filtering";

function containsSuccessfulBoot(items: RenderItem[]): boolean {
  return items.some((item) => {
    if (item.type === "message") return isSuccessfulAgentBootMessage(item.message);
    if (item.type === "turn_group") return item.messages.some(isSuccessfulAgentBootMessage);
    return false;
  });
}

function withoutSuccessfulBoots(messages: Message[]): Message[] {
  return messages.filter((message) => !isSuccessfulAgentBootMessage(message));
}

/**
 * Removes the session start-up rows ("Environment prepared", "Started agent")
 * once the agent has booted successfully. A failed or still-starting agent
 * keeps every row, so a start that goes wrong stays visible.
 */
export function hideSuccessfulStartupRows(items: RenderItem[]): RenderItem[] {
  if (!containsSuccessfulBoot(items)) return items;
  const kept: RenderItem[] = [];
  for (const item of items) {
    if (item.type === "prepare_progress") continue;
    if (item.type === "message") {
      if (!isSuccessfulAgentBootMessage(item.message)) kept.push(item);
      continue;
    }
    if (item.type === "turn_group") {
      const messages = withoutSuccessfulBoots(item.messages);
      if (messages.length > 0) kept.push({ ...item, messages });
      continue;
    }
    kept.push(item);
  }
  return kept;
}

/** The transcript items to render, with start-up rows hidden when asked. */
export function useVisibleItems(items: RenderItem[], hideStartupRows?: boolean): RenderItem[] {
  return useMemo(
    () => (hideStartupRows ? hideSuccessfulStartupRows(items) : items),
    [hideStartupRows, items],
  );
}
