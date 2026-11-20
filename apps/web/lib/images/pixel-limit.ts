/**
 * Returns the linear scale factor that makes a width × height area exactly
 * `maxPixels`. Callers clamp it to their own requested scale; an empty area
 * returns 0.
 */
export function pixelLimitFactor(width: number, height: number, maxPixels: number): number {
  if (width <= 0 || height <= 0) return 0;
  return Math.sqrt(maxPixels / (width * height));
}
