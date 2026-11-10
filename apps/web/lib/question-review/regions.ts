import type { ClarificationAnswerAnnotation } from "@/lib/types/http";
import type { Point, ReviewView, Size } from "./viewport";

export const MAX_REGIONS = 8;
export const MAX_REGION_COMMENT = 500;
/** Largest reported image size, in pixels, that the server accepts for a region. */
export const MAX_ANNOTATION_IMAGE_PIXELS = 16_000_000;

export type RegionRect = { x: number; y: number; width: number; height: number };

const clamp = (value: number, min: number, max: number) => Math.min(max, Math.max(min, value));

export function stageToImage(view: ReviewView, point: Point): Point {
  return { x: view.x + point.x / view.scale, y: view.y + point.y / view.scale };
}

/** Integer rectangle spanned by a drag, clamped to the image. Null for a click. */
export function rectFromPoints(a: Point, b: Point, image: Size): RegionRect | null {
  const left = clamp(Math.floor(Math.min(a.x, b.x)), 0, image.width - 1);
  const top = clamp(Math.floor(Math.min(a.y, b.y)), 0, image.height - 1);
  const right = clamp(Math.ceil(Math.max(a.x, b.x)), left + 1, image.width);
  const bottom = clamp(Math.ceil(Math.max(a.y, b.y)), top + 1, image.height);
  if (Math.abs(a.x - b.x) < 1 && Math.abs(a.y - b.y) < 1) return null;
  return { x: left, y: top, width: right - left, height: bottom - top };
}

/** A region covering a fifth of the visible area, centered, for keyboard creation. */
export function centerRegion(view: ReviewView, stage: Size, image: Size): RegionRect {
  const width = clamp(Math.round(stage.width / view.scale / 5), 1, image.width);
  const height = clamp(Math.round(stage.height / view.scale / 5), 1, image.height);
  const centerX = view.x + stage.width / view.scale / 2;
  const centerY = view.y + stage.height / view.scale / 2;
  return {
    x: clamp(Math.round(centerX - width / 2), 0, image.width - width),
    y: clamp(Math.round(centerY - height / 2), 0, image.height - height),
    width,
    height,
  };
}

export function moveRegion(rect: RegionRect, dx: number, dy: number, image: Size): RegionRect {
  return {
    ...rect,
    x: clamp(rect.x + dx, 0, image.width - rect.width),
    y: clamp(rect.y + dy, 0, image.height - rect.height),
  };
}

export function resizeRegion(rect: RegionRect, dw: number, dh: number, image: Size): RegionRect {
  return {
    ...rect,
    width: clamp(rect.width + dw, 1, image.width - rect.x),
    height: clamp(rect.height + dh, 1, image.height - rect.y),
  };
}

export function regionCommentReady(comment: string): boolean {
  const trimmed = comment.trim();
  return trimmed.length > 0 && Array.from(trimmed).length <= MAX_REGION_COMMENT;
}

export function regionCommentsReady(
  regions: Record<string, readonly ClarificationAnswerAnnotation[]>,
): boolean {
  return Object.values(regions).every((list) =>
    list.every((region) => regionCommentReady(region.comment)),
  );
}

/** Maps a span [start, start + length) onto a scaled axis, keeping it at least one pixel. */
function scaleSpan(start: number, length: number, from: number, to: number) {
  const scaledStart = Math.min(Math.floor((start * to) / from), to - 1);
  const scaledEnd = Math.min(Math.ceil(((start + length) * to) / from), to);
  return { start: scaledStart, length: Math.max(1, scaledEnd - scaledStart) };
}

/**
 * Regions are drawn in the image's own pixels. When the image is larger than the
 * server's limit, report the region in a proportionally smaller pixel space; the
 * agent scales by `image_width` and `image_height`, so the area it names is the same.
 */
export function fitAnnotationToPixelLimit(
  annotation: ClarificationAnswerAnnotation,
): ClarificationAnswerAnnotation {
  const { image_width: width, image_height: height } = annotation;
  if (width * height <= MAX_ANNOTATION_IMAGE_PIXELS) return annotation;
  const factor = Math.sqrt(MAX_ANNOTATION_IMAGE_PIXELS / (width * height));
  const scaledWidth = Math.max(1, Math.floor(width * factor));
  const scaledHeight = Math.max(1, Math.floor(height * factor));
  const x = scaleSpan(annotation.x, annotation.width, width, scaledWidth);
  const y = scaleSpan(annotation.y, annotation.height, height, scaledHeight);
  return {
    ...annotation,
    x: x.start,
    y: y.start,
    width: x.length,
    height: y.length,
    image_width: scaledWidth,
    image_height: scaledHeight,
  };
}
