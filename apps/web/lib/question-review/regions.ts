import type { ClarificationAnswerAnnotation } from "@/lib/types/http";
import type { Point, ReviewView, Size } from "./viewport";
import { pixelLimitFactor } from "@/lib/images/pixel-limit";
import { clamp, type RegionRect } from "@/lib/regions/geometry";

export const MAX_REGIONS = 8;
export const MAX_REGION_COMMENT = 500;
/** Largest reported image size, in pixels, that the server accepts for a region. */
export const MAX_ANNOTATION_IMAGE_PIXELS = 16_000_000;

export function stageToImage(view: ReviewView, point: Point): Point {
  return { x: view.x + point.x / view.scale, y: view.y + point.y / view.scale };
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
  const factor = pixelLimitFactor(width, height, MAX_ANNOTATION_IMAGE_PIXELS);
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
