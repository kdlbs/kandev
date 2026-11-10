/** Decoded RGBA pixels, as in `ImageData`. */
export type RasterImage = { width: number; height: number; data: Uint8ClampedArray };

export type PixelDiff = {
  width: number;
  height: number;
  changed: number;
  total: number;
  /** RGBA mask over the union of both images: changed pixels are opaque magenta. */
  mask: Uint8ClampedArray;
  /** Smallest rectangle containing every changed pixel, or null when none changed. */
  bounds: { x: number; y: number; width: number; height: number } | null;
};

const MARK = [255, 0, 255, 255] as const;

/** Largest comparison area, in pixels, matching the per-image annotation limit. */
export const MAX_DIFF_PIXELS = 16_000_000;
const DIFF_TOO_LARGE = Symbol("diff-too-large");

/** Whether the union of two images fits the comparison budget. */
export function diffFitsBudget(
  a: Pick<RasterImage, "width" | "height">,
  b: Pick<RasterImage, "width" | "height">,
): boolean {
  return Math.max(a.width, b.width) * Math.max(a.height, b.height) <= MAX_DIFF_PIXELS;
}

function pixelChanged(a: RasterImage, b: RasterImage, x: number, y: number, tolerance: number) {
  if (x >= a.width || y >= a.height || x >= b.width || y >= b.height) return true;
  const ai = (y * a.width + x) * 4;
  const bi = (y * b.width + x) * 4;
  for (let channel = 0; channel < 4; channel++) {
    if (Math.abs(a.data[ai + channel] - b.data[bi + channel]) > tolerance) return true;
  }
  return false;
}

/**
 * Compares two images pixel by pixel at full resolution, aligned at their
 * top-left corners. A pixel changes when any RGBA channel differs by more than
 * `tolerance`, or when it lies outside one of the images. Throws when the union
 * exceeds `MAX_DIFF_PIXELS`, so callers report the comparison as unavailable.
 */
export function diffPixels(a: RasterImage, b: RasterImage, tolerance: number): PixelDiff {
  if (!diffFitsBudget(a, b)) throw DIFF_TOO_LARGE;
  const width = Math.max(a.width, b.width);
  const height = Math.max(a.height, b.height);
  const mask = new Uint8ClampedArray(width * height * 4);
  let changed = 0;
  const box = { minX: width, minY: height, maxX: -1, maxY: -1 };
  for (let y = 0; y < height; y++) {
    for (let x = 0; x < width; x++) {
      if (!pixelChanged(a, b, x, y, tolerance)) continue;
      changed++;
      mask.set(MARK, (y * width + x) * 4);
      box.minX = Math.min(box.minX, x);
      box.minY = Math.min(box.minY, y);
      box.maxX = Math.max(box.maxX, x);
      box.maxY = Math.max(box.maxY, y);
    }
  }
  const bounds =
    changed === 0
      ? null
      : {
          x: box.minX,
          y: box.minY,
          width: box.maxX - box.minX + 1,
          height: box.maxY - box.minY + 1,
        };
  return { width, height, changed, total: width * height, mask, bounds };
}
