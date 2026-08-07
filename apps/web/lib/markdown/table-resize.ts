export const MIN_MARKDOWN_COLUMN_WIDTH = 64;

export function canResizeColumnBoundary(widths: readonly number[], boundaryIndex: number): boolean {
  const left = widths[boundaryIndex];
  const right = widths[boundaryIndex + 1];
  return (
    left !== undefined &&
    right !== undefined &&
    left >= MIN_MARKDOWN_COLUMN_WIDTH &&
    right >= MIN_MARKDOWN_COLUMN_WIDTH
  );
}

export function resizeAdjacentColumns(
  widths: readonly number[],
  boundaryIndex: number,
  delta: number,
): number[] {
  const resized = [...widths];
  const left = resized[boundaryIndex];
  const right = resized[boundaryIndex + 1];
  if (left === undefined || right === undefined) return resized;

  const pairWidth = left + right;
  if (pairWidth < MIN_MARKDOWN_COLUMN_WIDTH * 2) return resized;

  const nextLeft = Math.min(
    Math.max(left + delta, MIN_MARKDOWN_COLUMN_WIDTH),
    pairWidth - MIN_MARKDOWN_COLUMN_WIDTH,
  );
  resized[boundaryIndex] = nextLeft;
  resized[boundaryIndex + 1] = pairWidth - nextLeft;
  return resized;
}
