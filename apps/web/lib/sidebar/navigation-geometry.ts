export function savedNavigationHeight(height: number): number {
  return Math.max(0, Math.min(1600, Math.round(height)));
}
export function navigationGeometry(
  available: number,
  content: number,
  savedHeight: number | undefined,
  expanded: boolean,
  dividerHeight = 12,
) {
  const maximum = Math.max(0, available - 112 - dividerHeight);
  const compressed = Math.min(maximum, savedHeight ?? content);
  const height = Math.max(0, expanded ? Math.min(content, maximum) : Math.min(content, compressed));
  return { height, clipped: content > height + 1, expandable: content > compressed + 1 };
}
