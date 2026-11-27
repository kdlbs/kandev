import type { NeedsYouItem } from "./attention";

export type HeldItem = { item: NeedsYouItem; index: number };

/** Expanded cards retain their snapshot and position until released. */
export function mergeHeldNeedsYouItems(
  items: NeedsYouItem[],
  held: Record<string, HeldItem>,
): NeedsYouItem[] {
  const merged = items.filter((item) => !(item.id in held));
  Object.values(held)
    .sort((a, b) => a.index - b.index)
    .forEach((entry) => merged.splice(Math.min(entry.index, merged.length), 0, entry.item));
  return merged;
}
