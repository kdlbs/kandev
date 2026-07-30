import type { RenderItem } from "@/hooks/use-processed-messages";
import type { LastPromptEdge } from "./message-list-shared";

type RenderedRange = {
  start: number;
  end: number;
};
/** Index of the item containing `messageId` (a plain message or a turn-group
 * wrapping it), or -1 when absent/unknown. */
export function findMessageItemIndex(
  items: RenderItem[],
  messageId: string | null | undefined,
): number {
  if (!messageId) return -1;
  return items.findIndex((item) => {
    if (item.type === "turn_group") return item.messages.some((m) => m.id === messageId);
    if (item.type === "message") return item.message.id === messageId;
    return false;
  });
}

/** Where an item sits relative to Virtuoso's currently rendered range once
 * its row is unmounted: `before` it (scrolled past, further down the
 * transcript), `after` it (not yet reached, e.g. still browsing earlier
 * history), or `within` it (still rendered, or unknown). */
export type RangePosition = "before" | "within" | "after";

function resolveRangePosition(itemIndex: number, renderedRange: RenderedRange): RangePosition {
  if (itemIndex < 0) return "within";
  if (itemIndex < renderedRange.start) return "before";
  if (itemIndex > renderedRange.end) return "after";
  return "within";
}

/** Resolves a tracked transcript edge's state. While Virtuoso still has the
 * marked row mounted, `resolvers.geometryCheck` reads its live DOM position
 * — the only way to distinguish a partially clipped row from one with no
 * visible intersection. Once Virtuoso unmounts the row,
 * `resolvers.fromRangePosition` maps its item-index position relative to
 * the rendered range to the same result type as `geometryCheck`. */
export function resolveVirtuosoEdgeState<T>(
  row: HTMLElement | null,
  container: HTMLElement,
  itemIndex: number,
  renderedRange: RenderedRange,
  resolvers: {
    geometryCheck: (container: HTMLElement, row: HTMLElement) => T;
    fromRangePosition: (position: RangePosition) => T;
  },
): T {
  if (row) return resolvers.geometryCheck(container, row);
  return resolvers.fromRangePosition(resolveRangePosition(itemIndex, renderedRange));
}

/** Maps an unmounted last-prompt row's position relative to Virtuoso's
 * rendered range to the same edge state native geometry would report. */
export function rangePositionToLastPromptEdge(position: RangePosition): LastPromptEdge {
  if (position === "before") return "above";
  if (position === "after") return "below";
  return "visible";
}
