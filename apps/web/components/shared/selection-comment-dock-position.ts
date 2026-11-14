export type SelectionCommentRect = {
  left: number;
  top: number;
  right: number;
  bottom: number;
  width: number;
  height: number;
};

export type SelectionCommentInsets = {
  top?: number;
  right?: number;
  bottom?: number;
  left?: number;
};

export type SelectionCommentPortalOrigin = {
  left: number;
  top: number;
  scrollLeft?: number;
  scrollTop?: number;
};

export type SelectionCommentDockPosition = {
  left: number;
  top: number;
  width: number;
  height: number;
  clearance: number;
};

function getVisibleHorizontalBounds(
  ownerRect: SelectionCommentRect,
  viewportRect: SelectionCommentRect,
  safeArea: SelectionCommentInsets,
) {
  const safeLeft = ownerRect.left <= viewportRect.left ? (safeArea.left ?? 0) : 0;
  const safeRight = ownerRect.right >= viewportRect.right ? (safeArea.right ?? 0) : 0;
  const left = Math.max(ownerRect.left, viewportRect.left) + safeLeft;
  const right = Math.min(ownerRect.right, viewportRect.right) - safeRight;
  return { left, right, width: right - left };
}

function getVisibleVerticalBounds(
  ownerRect: SelectionCommentRect,
  boundaryRect: SelectionCommentRect | null | undefined,
  viewportRect: SelectionCommentRect,
  safeArea: SelectionCommentInsets,
) {
  const top =
    Math.max(ownerRect.top, viewportRect.top) +
    (ownerRect.top <= viewportRect.top ? (safeArea.top ?? 0) : 0);
  const lowerEdge = Math.min(
    ownerRect.bottom,
    viewportRect.bottom,
    boundaryRect?.top ?? Number.POSITIVE_INFINITY,
  );
  const bottom = lowerEdge - (lowerEdge >= viewportRect.bottom ? (safeArea.bottom ?? 0) : 0);
  return { top, bottom };
}

function toPortalPosition({
  left,
  top,
  width,
  height,
  clearance,
  portalOrigin,
}: SelectionCommentDockPosition & {
  portalOrigin: SelectionCommentPortalOrigin | undefined;
}): SelectionCommentDockPosition {
  const origin = portalOrigin ?? { left: 0, top: 0 };
  return {
    left: left - origin.left + (origin.scrollLeft ?? 0),
    top: top - origin.top + (origin.scrollTop ?? 0),
    width,
    height,
    clearance,
  };
}

export function resolveSelectionCommentDockPosition({
  ownerRect,
  boundaryRect,
  viewportRect,
  dockWidth,
  dockHeight,
  safeArea = {},
  portalOrigin,
}: {
  ownerRect: SelectionCommentRect | null;
  boundaryRect?: SelectionCommentRect | null;
  viewportRect: SelectionCommentRect;
  dockWidth: number;
  dockHeight: number;
  safeArea?: SelectionCommentInsets;
  portalOrigin?: SelectionCommentPortalOrigin;
}): SelectionCommentDockPosition | null {
  if (!ownerRect || dockWidth <= 0 || dockHeight <= 0) return null;
  const horizontal = getVisibleHorizontalBounds(ownerRect, viewportRect, safeArea);
  const vertical = getVisibleVerticalBounds(ownerRect, boundaryRect, viewportRect, safeArea);
  const width = Math.min(dockWidth, horizontal.width);
  if (width <= 0 || vertical.bottom - vertical.top < dockHeight) return null;

  const center = (horizontal.left + horizontal.right) / 2;
  const left = Math.min(Math.max(center - width / 2, horizontal.left), horizontal.right - width);
  const viewportTop = vertical.bottom - dockHeight;
  const reservationEdge = Math.min(ownerRect.bottom, boundaryRect?.top ?? Number.POSITIVE_INFINITY);
  const clearance = Math.max(dockHeight, reservationEdge - viewportTop);
  return toPortalPosition({
    left,
    top: viewportTop,
    width,
    height: dockHeight,
    clearance,
    portalOrigin,
  });
}
