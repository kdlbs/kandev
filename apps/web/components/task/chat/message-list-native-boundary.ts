import type { RenderItem } from "@/hooks/use-processed-messages";
import { TASK_DESCRIPTION_SYNTHETIC_ID } from "@/hooks/use-processed-messages";

/** Stable descriptor for the oldest visible row; turn ids survive group extension. */
export function getOldestVisibleBoundaryKey(items: RenderItem[]): string | null {
  const item = items.find(
    (candidate) =>
      candidate.type === "turn_group" ||
      (candidate.type === "message" && candidate.message.id !== TASK_DESCRIPTION_SYNTHETIC_ID),
  );
  if (!item) return null;
  if (item.type === "turn_group") return `turn:${item.turnId ?? item.id}`;
  return item.type === "message" ? `message:${item.message.id}` : item.id;
}
