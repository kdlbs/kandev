import { describe, expect, it, vi } from "vitest";
import {
  MAX_PREVIEW_SCREENSHOT_BYTES,
  MAX_PREVIEW_SCREENSHOT_PIXELS,
  normalizeScreenshotRect,
  previewScreenshotScale,
  rasterizePreviewRegion,
} from "./preview-screenshot";

describe("preview screenshot geometry", () => {
  it("normalizes reverse viewport drag into document and device coordinates", () => {
    expect(
      normalizeScreenshotRect(
        { x: 300, y: 240 },
        { x: 100, y: 80 },
        {
          scrollX: 20,
          scrollY: 600,
          viewportWidth: 390,
          viewportHeight: 844,
          devicePixelRatio: 3,
        },
      ),
    ).toEqual({
      x: 100,
      y: 80,
      width: 200,
      height: 160,
      document_x: 120,
      document_y: 680,
      scroll_x: 20,
      scroll_y: 600,
      viewport_width: 390,
      viewport_height: 844,
      device_pixel_ratio: 3,
    });
  });

  it("rejects empty drags and clamps an oversized region before allocating a canvas", () => {
    expect(
      normalizeScreenshotRect(
        { x: 10, y: 10 },
        { x: 12, y: 12 },
        { scrollX: 0, scrollY: 0, viewportWidth: 100, viewportHeight: 100, devicePixelRatio: 1 },
      ),
    ).toBeNull();
    expect(previewScreenshotScale(4_000, 4_000, 2)).toBe(1);
    expect(4_000 * 4_000 * previewScreenshotScale(4_000, 4_000, 2) ** 2).toBeLessThanOrEqual(
      MAX_PREVIEW_SCREENSHOT_PIXELS,
    );
  });
});

function pngBlob(size = 8) {
  const signature = new Uint8Array([137, 80, 78, 71, 13, 10, 26, 10]);
  return new Blob([signature, new Uint8Array(Math.max(0, size - signature.length))], {
    type: "image/png",
  });
}

describe("preview screenshot rasterization", () => {
  const rect = {
    x: 10,
    y: 20,
    width: 200,
    height: 100,
    document_x: 10,
    document_y: 420,
    device_pixel_ratio: 2,
  };

  it("renders only the selected document region and returns reviewable PNG dimensions", async () => {
    const blob = pngBlob(128);
    const canvas = {
      width: 400,
      height: 200,
      toBlob: (callback: BlobCallback) => callback(blob),
    } as HTMLCanvasElement;
    const render = vi.fn().mockResolvedValue(canvas);
    const documentElement = {} as HTMLElement;

    const result = await rasterizePreviewRegion(documentElement, rect, render);

    expect(render).toHaveBeenCalledWith(
      documentElement,
      expect.objectContaining({ x: 10, y: 420, width: 200, height: 100, scale: 2 }),
    );
    expect(result).toEqual({ blob, width: 400, height: 200 });
  });

  it("rejects empty canvases, non-PNG bytes, and encoded output above 10 MiB", async () => {
    const emptyCanvas = {
      width: 0,
      height: 0,
      toBlob: vi.fn(),
    } as unknown as HTMLCanvasElement;
    await expect(
      rasterizePreviewRegion({} as HTMLElement, rect, vi.fn().mockResolvedValue(emptyCanvas)),
    ).rejects.toThrow(/empty/i);

    const invalidCanvas = {
      width: 20,
      height: 20,
      toBlob: (callback: BlobCallback) =>
        callback(new Blob([new Uint8Array(20)], { type: "image/png" })),
    } as HTMLCanvasElement;
    await expect(
      rasterizePreviewRegion({} as HTMLElement, rect, vi.fn().mockResolvedValue(invalidCanvas)),
    ).rejects.toThrow(/PNG/i);

    const oversizedCanvas = {
      width: 20,
      height: 20,
      toBlob: (callback: BlobCallback) => callback(pngBlob(MAX_PREVIEW_SCREENSHOT_BYTES + 1)),
    } as HTMLCanvasElement;
    await expect(
      rasterizePreviewRegion({} as HTMLElement, rect, vi.fn().mockResolvedValue(oversizedCanvas)),
    ).rejects.toThrow(/large/i);
  });
});
