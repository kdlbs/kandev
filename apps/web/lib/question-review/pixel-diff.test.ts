import { describe, expect, it } from "vitest";
import { diffPixels, type RasterImage } from "./pixel-diff";

function raster(
  width: number,
  height: number,
  fill: [number, number, number, number],
): RasterImage {
  const data = new Uint8ClampedArray(width * height * 4);
  for (let i = 0; i < data.length; i += 4) data.set(fill, i);
  return { width, height, data };
}

function setPixel(
  image: RasterImage,
  x: number,
  y: number,
  rgba: [number, number, number, number],
) {
  image.data.set(rgba, (y * image.width + x) * 4);
}

function maskAt(result: ReturnType<typeof diffPixels>, x: number, y: number) {
  const offset = (y * result.width + x) * 4;
  return Array.from(result.mask.slice(offset, offset + 4));
}

const WHITE: [number, number, number, number] = [255, 255, 255, 255];

describe("diffPixels", () => {
  // @covers AC-TASKS-QUESTION-IMAGE-REVIEW-005.6
  it("reports no change for identical images", () => {
    const result = diffPixels(raster(4, 3, WHITE), raster(4, 3, WHITE), 0);
    expect(result).toMatchObject({ width: 4, height: 3, changed: 0, total: 12 });
    expect(maskAt(result, 0, 0)).toEqual([0, 0, 0, 0]);
  });

  it("finds a single changed pixel at full resolution", () => {
    const b = raster(100, 100, WHITE);
    setPixel(b, 37, 81, [254, 0, 255, 255]);
    const result = diffPixels(raster(100, 100, WHITE), b, 16);
    expect(result.changed).toBe(1);
    expect(maskAt(result, 37, 81)[3]).toBe(255);
    expect(maskAt(result, 36, 81)[3]).toBe(0);
  });

  it("treats a channel difference equal to the tolerance as unchanged", () => {
    const b = raster(2, 1, WHITE);
    setPixel(b, 0, 0, [239, 255, 255, 255]);
    setPixel(b, 1, 0, [238, 255, 255, 255]);
    expect(diffPixels(raster(2, 1, WHITE), b, 16).changed).toBe(1);
    expect(diffPixels(raster(2, 1, WHITE), b, 17).changed).toBe(0);
  });

  it("counts an alpha-only change", () => {
    const b = raster(1, 1, [255, 255, 255, 100]);
    expect(diffPixels(raster(1, 1, WHITE), b, 16).changed).toBe(1);
  });

  // @covers AC-TASKS-QUESTION-IMAGE-REVIEW-005.8
  it("compares the union of different sizes and counts pixels outside the shared area", () => {
    const result = diffPixels(raster(4, 2, WHITE), raster(3, 3, WHITE), 0);
    expect(result).toMatchObject({ width: 4, height: 3, total: 12 });
    // Shared 3x2 area is identical; the remaining 6 pixels exist in only one image.
    expect(result.changed).toBe(6);
    expect(maskAt(result, 3, 0)[3]).toBe(255);
    expect(maskAt(result, 0, 2)[3]).toBe(255);
    expect(maskAt(result, 2, 1)[3]).toBe(0);
  });
});

describe("changed bounds", () => {
  // @covers AC-TASKS-QUESTION-IMAGE-REVIEW-005.13
  it("reports the box around all changed pixels, or null when nothing changed", () => {
    const b = raster(20, 10, WHITE);
    setPixel(b, 3, 2, [0, 0, 0, 255]);
    setPixel(b, 11, 7, [0, 0, 0, 255]);
    expect(diffPixels(raster(20, 10, WHITE), b, 16).bounds).toEqual({
      x: 3,
      y: 2,
      width: 9,
      height: 6,
    });
    expect(diffPixels(raster(2, 2, WHITE), raster(2, 2, WHITE), 0).bounds).toBeNull();
  });
});
