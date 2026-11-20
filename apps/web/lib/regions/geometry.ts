/** Rectangle geometry shared by every surface that draws numbered regions. */

export type RegionRect = { x: number; y: number; width: number; height: number };
export type RegionPoint = { x: number; y: number };
export type RegionBounds = { width: number; height: number };
export type RegionCorner = "nw" | "ne" | "sw" | "se";

export const clamp = (value: number, min: number, max: number) =>
  Math.min(max, Math.max(min, value));

/** Integer rectangle spanned by a drag, clamped to the bounds. Null for a click. */
export function rectFromPoints(
  a: RegionPoint,
  b: RegionPoint,
  bounds: RegionBounds,
): RegionRect | null {
  const left = clamp(Math.floor(Math.min(a.x, b.x)), 0, bounds.width - 1);
  const top = clamp(Math.floor(Math.min(a.y, b.y)), 0, bounds.height - 1);
  const right = clamp(Math.ceil(Math.max(a.x, b.x)), left + 1, bounds.width);
  const bottom = clamp(Math.ceil(Math.max(a.y, b.y)), top + 1, bounds.height);
  if (Math.abs(a.x - b.x) < 1 && Math.abs(a.y - b.y) < 1) return null;
  return { x: left, y: top, width: right - left, height: bottom - top };
}

export function moveRegion(
  rect: RegionRect,
  dx: number,
  dy: number,
  bounds: RegionBounds,
): RegionRect {
  return {
    ...rect,
    x: clamp(rect.x + dx, 0, bounds.width - rect.width),
    y: clamp(rect.y + dy, 0, bounds.height - rect.height),
  };
}

export function resizeRegion(
  rect: RegionRect,
  dw: number,
  dh: number,
  bounds: RegionBounds,
): RegionRect {
  return {
    ...rect,
    width: clamp(rect.width + dw, 1, bounds.width - rect.x),
    height: clamp(rect.height + dh, 1, bounds.height - rect.y),
  };
}

type Edge = { start: number; end: number; limit: number };

/** Moves one edge along an axis, keeping the opposite edge fixed. */
function dragEdge(edge: Edge, delta: number, movesStart: boolean, minSize: number) {
  const { start, end, limit } = edge;
  if (movesStart) {
    const next = clamp(start + delta, 0, Math.max(0, end - minSize));
    return { start: next, size: end - next };
  }
  const next = clamp(end + delta, Math.min(limit, start + minSize), limit);
  return { start, size: next - start };
}

/**
 * Resizes by dragging one corner by `delta`. The opposite corner stays fixed,
 * the result stays inside the bounds, and it never shrinks below `minSize` or
 * flips.
 */
export function resizeRegionFromCorner(
  rect: RegionRect,
  corner: RegionCorner,
  delta: RegionPoint,
  bounds: RegionBounds,
  minSize = 1,
): RegionRect {
  const horizontal = dragEdge(
    { start: rect.x, end: rect.x + rect.width, limit: bounds.width },
    delta.x,
    corner === "nw" || corner === "sw",
    minSize,
  );
  const vertical = dragEdge(
    { start: rect.y, end: rect.y + rect.height, limit: bounds.height },
    delta.y,
    corner === "nw" || corner === "ne",
    minSize,
  );
  return { x: horizontal.start, y: vertical.start, width: horizontal.size, height: vertical.size };
}
