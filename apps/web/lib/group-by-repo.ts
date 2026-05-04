/**
 * Buckets a list into per-repository groups, preserving the original item
 * order within each bucket and producing groups in first-seen order. Items
 * whose `getName` returns undefined or empty land in a single empty-named
 * group so callers can decide whether to render a header for it.
 *
 * One copy of this lived in three places before — review-diff-list-groups,
 * changes-panel-timeline (files), and changes-panel-timeline (commits) —
 * differing only in element type. The generic shape collapses them all.
 */
export function groupByRepositoryName<T>(
  items: T[],
  getName: (item: T) => string | undefined,
): Array<{ repositoryName: string; items: T[] }> {
  const order: string[] = [];
  const buckets = new Map<string, T[]>();
  for (const item of items) {
    const name = getName(item) ?? "";
    if (!buckets.has(name)) {
      buckets.set(name, []);
      order.push(name);
    }
    buckets.get(name)!.push(item);
  }
  return order.map((name) => ({ repositoryName: name, items: buckets.get(name)! }));
}

/**
 * True when the grouped output represents a single-repo workspace — exactly
 * one bucket with the empty-name key. Callers use this to drop the redundant
 * per-repo sub-header in the changes panel and lift action buttons up to the
 * section header. Single source of truth so the three call sites
 * (CommitsSection, FileListBody, FileListSection) stay in sync.
 */
export function isSingleRepoGroup(groups: Array<{ repositoryName: string }>): boolean {
  return groups.length === 1 && groups[0].repositoryName === "";
}
