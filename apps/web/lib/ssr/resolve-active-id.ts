// Returns the first candidate id present in `items` (checked in order, so callers
// pass highest-priority first), falling back to the first item's id.
export function resolveActiveId<T extends { id: string }>(
  items: T[],
  ...preferredIds: (string | null | undefined)[]
): string | null {
  for (const id of preferredIds) {
    if (id == null) continue;
    const match = items.find((i) => i.id === id);
    if (match) return match.id;
  }
  return items[0]?.id ?? null;
}
