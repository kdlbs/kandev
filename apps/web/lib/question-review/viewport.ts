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
