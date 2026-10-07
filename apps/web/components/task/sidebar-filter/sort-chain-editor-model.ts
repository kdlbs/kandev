import type { SortRule } from "@/lib/state/slices/ui/sidebar-view-types";

export type IdentifiedSortRule = { id: string; rule: SortRule };

export function changeIdentifiedSortRule(
  entries: IdentifiedSortRule[],
  index: number,
  rule: SortRule,
): IdentifiedSortRule[] {
  if (!entries[index]) return entries;
  const updated = [...entries];
  updated[index] = { ...updated[index], rule };
  return updated;
}

export function moveIdentifiedSortRule(
  entries: IdentifiedSortRule[],
  index: number,
  offset: number,
): IdentifiedSortRule[] {
  const destination = index + offset;
  if (destination < 0 || destination >= entries.length) return entries;
  const updated = [...entries];
  [updated[index], updated[destination]] = [updated[destination], updated[index]];
  return updated;
}

export function removeIdentifiedSortRule(
  entries: IdentifiedSortRule[],
  index: number,
): IdentifiedSortRule[] {
  return entries.filter((_, current) => current !== index);
}
