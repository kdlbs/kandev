import { describe, expect, it, vi } from "vitest";
import {
  MAX_PREVIEW_SCREENSHOT_BYTES,
  MAX_PREVIEW_SCREENSHOT_PIXELS,
  PREVIEW_CAPTURE_TIMEOUT_MS,
  normalizeScreenshotRect,
  isUniformPixels,
  previewScreenshotScale,
  rasterizePreviewRegion,
  regionOverlapsCanvas,
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
    expect(result).toEqual({ blob, width: 400, height: 200, uniformOverCanvas: false });
  });

  it("renders an inner scroll container at the scroll position recorded at release", async () => {
    const blob = pngBlob(128);
    const canvas = {
      width: 400,
      height: 200,
      toBlob: (callback: BlobCallback) => callback(blob),
    } as HTMLCanvasElement;
    const clone = document.implementation.createHTMLDocument("clone");
    clone.body.innerHTML = '<ul id="scroller"></ul>';
    const render = vi.fn(
      async (_element: HTMLElement, options: { onclone?: (doc: Document) => void }) => {
        options.onclone?.(clone);
        return canvas;
      },
    );

    await rasterizePreviewRegion(
      {} as HTMLElement,
      {
        ...rect,
        scroll_container_selector: "#scroller",
        container_scroll_x: 4,
        container_scroll_y: 160,
      },
      render,
    );

    const scroller = clone.getElementById("scroller")!;
    expect(scroller.scrollTop).toBe(160);
    expect(scroller.scrollLeft).toBe(4);
  });

  it("gives up on a render that never settles", async () => {
    vi.useFakeTimers();
    try {
      const pending = rasterizePreviewRegion({} as HTMLElement, rect, () => new Promise(() => {}));
      const outcome = expect(pending).rejects.toThrow();
      await vi.advanceTimersByTimeAsync(PREVIEW_CAPTURE_TIMEOUT_MS);
      await outcome;
    } finally {
      vi.useRealTimers();
    }
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

// @covers AC-TASKS-WEB-PREVIEW-FEEDBACK-002.9
describe("uniform canvas capture detection", () => {
  const rect = { x: 10, y: 20, width: 200, height: 100, document_x: 10, document_y: 20 };

  function frameWithCanvas(bounds: { left: number; top: number; right: number; bottom: number }) {
    const doc = document.implementation.createHTMLDocument("preview");
    const canvas = doc.createElement("canvas");
    canvas.getBoundingClientRect = () =>
      ({ ...bounds, width: 0, height: 0, x: 0, y: 0 }) as DOMRect;
    doc.body.appendChild(canvas);
    return doc.documentElement;
  }

  function rendered() {
    return {
      width: 20,
      height: 20,
      toBlob: (callback: BlobCallback) => callback(pngBlob(64)),
    } as HTMLCanvasElement;
  }

  it("recognizes a single-colour pixel buffer", () => {
    expect(isUniformPixels(new Uint8ClampedArray([1, 2, 3, 255, 1, 2, 3, 255]))).toBe(true);
    expect(isUniformPixels(new Uint8ClampedArray([1, 2, 3, 255, 1, 2, 4, 255]))).toBe(false);
    expect(isUniformPixels(new Uint8ClampedArray([]))).toBe(false);
  });

  it("detects whether the capture rectangle intersects a canvas", () => {
    expect(
      regionOverlapsCanvas(frameWithCanvas({ left: 0, top: 0, right: 50, bottom: 50 }), rect),
    ).toBe(true);
    expect(
      regionOverlapsCanvas(frameWithCanvas({ left: 300, top: 0, right: 400, bottom: 50 }), rect),
    ).toBe(false);
    expect(regionOverlapsCanvas({} as HTMLElement, rect)).toBe(false);
  });

  it("finds canvases inside open shadow roots, as Flutter web renders them", () => {
    const doc = document.implementation.createHTMLDocument("preview");
    const host = doc.createElement("flutter-view");
    doc.body.appendChild(host);
    const canvas = doc.createElement("canvas");
    canvas.getBoundingClientRect = () =>
      ({
        left: 0,
        top: 0,
        right: 500,
        bottom: 500,
        width: 500,
        height: 500,
        x: 0,
        y: 0,
      }) as DOMRect;
    host.attachShadow({ mode: "open" }).appendChild(canvas);
    expect(regionOverlapsCanvas(doc.documentElement, rect)).toBe(true);
  });

  it("flags a uniform capture over a canvas but still returns the image", async () => {
    const result = await rasterizePreviewRegion(
      frameWithCanvas({ left: 0, top: 0, right: 500, bottom: 500 }),
      rect,
      vi.fn().mockResolvedValue(rendered()),
      () => true,
    );
    expect(result.uniformOverCanvas).toBe(true);
    expect(result.blob.type).toBe("image/png");
  });

  it("does not flag a uniform capture away from any canvas", async () => {
    const sample = vi.fn(() => true);
    const result = await rasterizePreviewRegion(
      frameWithCanvas({ left: 900, top: 900, right: 950, bottom: 950 }),
      rect,
      vi.fn().mockResolvedValue(rendered()),
      sample,
    );
    expect(result.uniformOverCanvas).toBe(false);
    expect(sample).not.toHaveBeenCalled();
  });
});
