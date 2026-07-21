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
