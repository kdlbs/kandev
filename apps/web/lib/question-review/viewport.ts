/** Stage view in image pixels: `x`/`y` is the image point at the stage's top-left corner. */
export type ReviewView = { scale: number; x: number; y: number };
export type Size = { width: number; height: number };
export type Point = { x: number; y: number };

export const MIN_SCALE = 0.25;
export const MAX_SCALE = 8;
export const ZOOM_STEPS = [0.25, 0.5, 1, 2, 4, 8] as const;

const EPSILON = 1e-6;

/** Zooming out stops at Fit when Fit is below the 25% manual minimum. */
export function minScaleFor(fitScale: number): number {
  return fitScale > 0 ? Math.min(MIN_SCALE, fitScale) : MIN_SCALE;
}

export function clampScale(scale: number, minScale = MIN_SCALE): number {
  return Math.min(MAX_SCALE, Math.max(minScale, scale));
}

function hasSize(size: Size): boolean {
  return size.width > 0 && size.height > 0;
}

/** Centers a view of `image` at `scale` inside `stage`. */
function centered(stage: Size, image: Size, scale: number): ReviewView {
  return {
    scale,
    x: (image.width - stage.width / scale) / 2,
    y: (image.height - stage.height / scale) / 2,
  };
}

export function fitView(stage: Size, image: Size): ReviewView {
  if (!hasSize(stage) || !hasSize(image)) return { scale: 1, x: 0, y: 0 };
  const scale = Math.min(MAX_SCALE, stage.width / image.width, stage.height / image.height);
  return centered(stage, image, scale);
}

export function nextZoomStep(scale: number, direction: 1 | -1, minScale = MIN_SCALE): number {
  if (direction > 0) return ZOOM_STEPS.find((step) => step > scale + EPSILON) ?? MAX_SCALE;
  const lower = [...ZOOM_STEPS].reverse().find((step) => step < scale - EPSILON);
  return lower === undefined || lower < minScale ? minScale : lower;
}

export function formatZoom(scale: number): string {
  return `${Math.round(scale * 100)}%`;
}

/** Changes the scale while keeping the image point under `focal` (stage pixels) fixed. */
export function zoomAt(
  view: ReviewView,
  scale: number,
  focal: Point,
  minScale = MIN_SCALE,
): ReviewView {
  const next = clampScale(scale, minScale);
  const imageX = view.x + focal.x / view.scale;
  const imageY = view.y + focal.y / view.scale;
  return { scale: next, x: imageX - focal.x / next, y: imageY - focal.y / next };
}

/** Pans by a stage-pixel drag delta. */
export function panBy(view: ReviewView, dx: number, dy: number): ReviewView {
  return { scale: view.scale, x: view.x - dx / view.scale, y: view.y - dy / view.scale };
}

function clampAxis(value: number, imageLength: number, visibleLength: number): number {
  const overflow = imageLength - visibleLength;
  return Math.min(Math.max(0, overflow), Math.max(Math.min(0, overflow), value));
}

/** Keeps a large image covering the stage and a small image inside it. */
export function clampView(view: ReviewView, stage: Size, image: Size): ReviewView {
  if (!hasSize(stage) || !hasSize(image)) return view;
  return {
    scale: view.scale,
    x: clampAxis(view.x, image.width, stage.width / view.scale),
    y: clampAxis(view.y, image.height, stage.height / view.scale),
  };
}

export function actualSizeView(stage: Size, image: Size, view: ReviewView): ReviewView {
  return clampView(zoomAt(view, 1, { x: stage.width / 2, y: stage.height / 2 }), stage, image);
}

const RECT_PADDING = 0.1;

/** A view that fits `rect` (image pixels) with 10% padding, centered in the stage. */
export function rectView(stage: Size, rect: RegionLike, minScale = MIN_SCALE): ReviewView {
  const width = rect.width * (1 + RECT_PADDING * 2);
  const height = rect.height * (1 + RECT_PADDING * 2);
  const scale = clampScale(Math.min(stage.width / width, stage.height / height), minScale);
  return {
    scale,
    x: rect.x + rect.width / 2 - stage.width / (2 * scale),
    y: rect.y + rect.height / 2 - stage.height / (2 * scale),
  };
}

type RegionLike = { x: number; y: number; width: number; height: number };

/** Image x under the stage's vertical center line. */
function centerLineX(view: ReviewView, stage: Size): number {
  return view.x + stage.width / (2 * view.scale);
}

/**
 * Pinned-wipe pan range: either image edge may reach the center line, so the
 * operator can scrub from all B to all A without moving the line.
 */
export function clampPinnedView(view: ReviewView, stage: Size, image: Size): ReviewView {
  if (!hasSize(stage) || !hasSize(image)) return view;
  const half = stage.width / (2 * view.scale);
  return {
    scale: view.scale,
    x: Math.min(image.width - half, Math.max(-half, view.x)),
    y: clampAxis(view.y, image.height, stage.height / view.scale),
  };
}

/** Where the pinned center line falls on the image, from 0 (all B) to 1 (all A). */
export function pinnedWipeFraction(view: ReviewView, stage: Size, image: Size): number {
  if (!hasSize(image)) return 0.5;
  return Math.min(1, Math.max(0, centerLineX(view, stage) / image.width));
}

/** Moves the image horizontally so the center line lands on `fraction`. */
export function pinnedViewForFraction(
  view: ReviewView,
  stage: Size,
  image: Size,
  fraction: number,
): ReviewView {
  return { ...view, x: fraction * image.width - stage.width / (2 * view.scale) };
}

const SWIPE_DISTANCE = 50;
const SWIPE_DRIFT = 40;

/** 1 for a swipe left (next), -1 for a swipe right (previous), 0 otherwise. */
export function swipeDirection(dx: number, dy: number): 1 | -1 | 0 {
  const horizontal = Math.abs(dx);
  if (
    horizontal < SWIPE_DISTANCE ||
    Math.abs(dy) >= SWIPE_DRIFT ||
    horizontal < Math.abs(dy) * 1.5
  ) {
    return 0;
  }
  return dx < 0 ? 1 : -1;
}
