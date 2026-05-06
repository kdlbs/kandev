/**
 * Topological sort for subtask checklists.
 * Items with no blockers appear first; blocked items follow once their blockers
 * have been placed in the output. Cycles are broken by appending the remaining
 * items in original order.
 */

export type SortableItem = {
  id: string;
  blockedBy?: string[];
};

export function topoSort<T extends SortableItem>(items: T[]): T[] {
  if (items.length === 0) return [];

  const idSet = new Set(items.map((i) => i.id));
  const result: T[] = [];
  const placed = new Set<string>();
  const remaining = [...items];

  while (remaining.length > 0) {
    const before = remaining.length;
    for (let i = 0; i < remaining.length; i++) {
      const item = remaining[i];
      const blockers = (item.blockedBy ?? []).filter((b) => idSet.has(b));
      if (blockers.every((b) => placed.has(b))) {
        result.push(item);
        placed.add(item.id);
        remaining.splice(i, 1);
        i--;
      }
    }
    // No progress — cycle detected; append remaining as-is to avoid infinite loop.
    if (remaining.length === before) {
      result.push(...remaining);
      break;
    }
  }

  return result;
}

/** Returns true if any item in the list has at least one blocker. */
export function hasBlockerChain(items: SortableItem[]): boolean {
  return items.some((i) => (i.blockedBy?.length ?? 0) > 0);
}
