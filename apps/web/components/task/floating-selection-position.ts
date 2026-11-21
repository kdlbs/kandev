export type FloatingBounds = {
  left: number;
  top: number;
  right: number;
  bottom: number;
};

export function floatingBounds(rect?: Pick<DOMRect, "left" | "top" | "right" | "bottom"> | null) {
  return rect
    ? { left: rect.left, top: rect.top, right: rect.right, bottom: rect.bottom }
    : { left: 0, top: 0, right: window.innerWidth, bottom: window.innerHeight };
}

function clamp(value: number, minimum: number, maximum: number) {
  return Math.min(Math.max(value, minimum), maximum);
}

export function placeFloatingRect({
  left,
  topCandidates,
  width,
  height,
  bounds,
  margin = 8,
}: {
  left: number;
  topCandidates: number[];
  width: number;
  height: number;
  bounds: FloatingBounds;
  margin?: number;
}) {
  const minimumLeft = bounds.left + margin;
  const maximumLeft = Math.max(minimumLeft, bounds.right - width - margin);
  const minimumTop = bounds.top + margin;
  const maximumTop = Math.max(minimumTop, bounds.bottom - height - margin);
  const fittingTop = topCandidates.find(
    (candidate) => candidate >= minimumTop && candidate <= maximumTop,
  );

  return {
    left: clamp(left, minimumLeft, maximumLeft),
    top: fittingTop ?? clamp(topCandidates[0] ?? minimumTop, minimumTop, maximumTop),
  };
}

type RectLike = { x: number; y: number; width: number; height: number };

/**
 * Viewport point just below a rectangle's bottom-left corner. `rect` is
 * relative to `origin` (for example an iframe or image stage); `bounds`
 * keeps the anchor on the visible host.
 */
export function anchorBelowRect(
  rect: RectLike,
  origin: { left: number; top: number },
  bounds?: FloatingBounds,
) {
  const x = origin.left + rect.x;
  const y = origin.top + rect.y + rect.height;
  if (!bounds) return { x, y };
  return { x: clamp(x, bounds.left, bounds.right), y: clamp(y, bounds.top, bounds.bottom) };
}
