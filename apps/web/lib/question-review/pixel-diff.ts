/** Decoded RGBA pixels, as in `ImageData`. */
export type RasterImage = { width: number; height: number; data: Uint8ClampedArray };

export type PixelDiff = {
  width: number;
  height: number;
  changed: number;
  total: number;
  /** RGBA mask over the union of both images: changed pixels are opaque magenta. */
  mask: Uint8ClampedArray;
};

const MARK = [255, 0, 255, 255] as const;

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
 * `tolerance`, or when it lies outside one of the images.
 */
export function diffPixels(a: RasterImage, b: RasterImage, tolerance: number): PixelDiff {
  const width = Math.max(a.width, b.width);
  const height = Math.max(a.height, b.height);
  const mask = new Uint8ClampedArray(width * height * 4);
  let changed = 0;
  for (let y = 0; y < height; y++) {
    for (let x = 0; x < width; x++) {
      if (!pixelChanged(a, b, x, y, tolerance)) continue;
      changed++;
      mask.set(MARK, (y * width + x) * 4);
    }
  }
  return { width, height, changed, total: width * height, mask };
}
