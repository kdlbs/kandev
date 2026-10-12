export type ChangesTimelineRowIdentity = { key: string };

export type ChangesTimelineGroup = {
  key: string;
  testId?: string;
  attributes?: Record<string, string>;
};

export type TimelineVirtualItem = {
  key: string | number | bigint;
  index: number;
  start: number;
  end: number;
};

export type ChangesTimelineVisibleNode<Row extends ChangesTimelineRowIdentity> =
  | { kind: "row"; item: TimelineVirtualItem }
  | {
      kind: "group";
      group: ChangesTimelineGroup;
      path: string[];
      items: TimelineVirtualItem[];
      children: ChangesTimelineVisibleNode<Row>[];
    };

export function visibleNodeKey<Row extends ChangesTimelineRowIdentity>(
  node: ChangesTimelineVisibleNode<Row>,
): string {
  return node.kind === "row" ? String(node.item.key) : `group-${JSON.stringify(node.path)}`;
}

export function buildVisibleGroupTree<Row extends ChangesTimelineRowIdentity>(
  items: TimelineVirtualItem[],
  rows: Row[],
  getGroups: (row: Row) => ChangesTimelineGroup[],
): ChangesTimelineVisibleNode<Row>[] {
  const roots: ChangesTimelineVisibleNode<Row>[] = [];
  let previousPath: ChangesTimelineGroup[] = [];
  let ancestors: Array<{
    path: string[];
    node: Extract<ChangesTimelineVisibleNode<Row>, { kind: "group" }>;
  }> = [];

  for (const item of items) {
    const row = rows[item.index];
    if (!row) continue;
    const path = getGroups(row);
    let shared = 0;
    while (
      shared < path.length &&
      shared < previousPath.length &&
      path[shared]?.key === previousPath[shared]?.key
    ) {
      shared += 1;
    }
    ancestors = ancestors.slice(0, shared);
    const destination = () => ancestors.at(-1)?.node.children ?? roots;
    const pathKeys: string[] = [];
    for (let index = 0; index < path.length; index += 1) {
      const group = path[index];
      if (!group) continue;
      pathKeys.push(group.key);
      if (index < shared) continue;
      const node: Extract<ChangesTimelineVisibleNode<Row>, { kind: "group" }> = {
        kind: "group",
        group,
        path: [...pathKeys],
        items: [],
        children: [],
      };
      destination().push(node);
      ancestors.push({ path: [...pathKeys], node });
    }
    const destinationNode = ancestors.at(-1)?.node;
    (destinationNode?.children ?? roots).push({ kind: "row", item });
    for (const ancestor of ancestors) ancestor.node.items.push(item);
    previousPath = path;
  }
  return roots;
}

export function resolveRetainedFocusIndex<Row extends ChangesTimelineRowIdentity>(
  rows: Row[],
  getRowGroups: (row: Row) => ChangesTimelineGroup[],
  previousGroupKeys: string[],
  previousIndex: number,
): number {
  if (rows.length === 0) return -1;
  const fallbackIndex = Math.min(previousIndex, rows.length - 1);
  if (previousGroupKeys.length === 0) return fallbackIndex;

  let bestIndex = -1;
  let bestSharedDepth = 0;
  let bestDistance = Number.POSITIVE_INFINITY;
  rows.forEach((row, index) => {
    const groups = getRowGroups(row);
    let sharedDepth = 0;
    while (
      sharedDepth < groups.length &&
      sharedDepth < previousGroupKeys.length &&
      groups[sharedDepth]?.key === previousGroupKeys[sharedDepth]
    ) {
      sharedDepth += 1;
    }
    const distance = Math.abs(index - previousIndex);
    if (
      sharedDepth > bestSharedDepth ||
      (sharedDepth === bestSharedDepth && distance < bestDistance)
    ) {
      bestIndex = index;
      bestSharedDepth = sharedDepth;
      bestDistance = distance;
    }
  });
  return bestSharedDepth > 0 ? bestIndex : fallbackIndex;
}
