import type { PreviewCaptureRect } from "@/lib/types/http";

export const MAX_PREVIEW_SCREENSHOT_PIXELS = 16_000_000;
export const MAX_PREVIEW_SCREENSHOT_BYTES = 10 * 1024 * 1024;
const MIN_SCREENSHOT_EDGE = 5;
const PNG_SIGNATURE = [137, 80, 78, 71, 13, 10, 26, 10] as const;

type Point = { x: number; y: number };
type ViewportMetrics = {
  scrollX: number;
  scrollY: number;
  viewportWidth: number;
  viewportHeight: number;
  devicePixelRatio: number;
};

export type PreviewRasterRenderer = (
  element: HTMLElement,
  options: {
    x: number;
    y: number;
    width: number;
    height: number;
    scale: number;
    useCORS: boolean;
    logging: boolean;
    backgroundColor: null;
  },
) => Promise<HTMLCanvasElement>;

export type RasterizedPreviewRegion = {
  blob: Blob;
  width: number;
  height: number;
};

function clamp(value: number, minimum: number, maximum: number) {
  return Math.min(maximum, Math.max(minimum, value));
}

export function normalizeScreenshotRect(
  start: Point,
  end: Point,
  metrics: ViewportMetrics,
): PreviewCaptureRect | null {
  const startX = clamp(start.x, 0, metrics.viewportWidth);
  const startY = clamp(start.y, 0, metrics.viewportHeight);
  const endX = clamp(end.x, 0, metrics.viewportWidth);
  const endY = clamp(end.y, 0, metrics.viewportHeight);
  const x = Math.min(startX, endX);
  const y = Math.min(startY, endY);
  const width = Math.abs(endX - startX);
  const height = Math.abs(endY - startY);
  if (width < MIN_SCREENSHOT_EDGE || height < MIN_SCREENSHOT_EDGE) return null;
  return {
    x,
    y,
    width,
    height,
    document_x: x + metrics.scrollX,
    document_y: y + metrics.scrollY,
    scroll_x: metrics.scrollX,
    scroll_y: metrics.scrollY,
    viewport_width: metrics.viewportWidth,
    viewport_height: metrics.viewportHeight,
    device_pixel_ratio: metrics.devicePixelRatio,
  };
}

export function previewScreenshotScale(width: number, height: number, requestedScale: number) {
  if (width <= 0 || height <= 0 || requestedScale <= 0) return 0;
  const pixelBoundScale = Math.sqrt(MAX_PREVIEW_SCREENSHOT_PIXELS / (width * height));
  return Math.min(requestedScale, pixelBoundScale);
}

function canvasBlob(canvas: HTMLCanvasElement) {
  return new Promise<Blob>((resolve, reject) => {
    canvas.toBlob((blob) => {
      if (blob) resolve(blob);
      // i18n-exempt: internal raster failure; callers display localized recovery copy.
      else reject(new Error("PNG encoding returned an empty result"));
    }, "image/png");
  });
}

async function validatePNG(blob: Blob) {
  if (blob.type !== "image/png") {
    // i18n-exempt: internal raster failure; callers display localized recovery copy.
    throw new Error("Screenshot encoding did not produce PNG data");
  }
  if (blob.size > MAX_PREVIEW_SCREENSHOT_BYTES) {
    // i18n-exempt: internal raster failure; callers display localized recovery copy.
    throw new Error("Screenshot PNG is too large");
  }
  const signature = new Uint8Array(await blob.slice(0, PNG_SIGNATURE.length).arrayBuffer());
  if (
    signature.length !== PNG_SIGNATURE.length ||
    !PNG_SIGNATURE.every((value, index) => signature[index] === value)
  ) {
    // i18n-exempt: internal raster failure; callers display localized recovery copy.
    throw new Error("Screenshot encoding returned invalid PNG bytes");
  }
}

export async function rasterizePreviewRegion(
  documentElement: HTMLElement,
  rect: PreviewCaptureRect,
  render: PreviewRasterRenderer,
): Promise<RasterizedPreviewRegion> {
  const scale = previewScreenshotScale(
    rect.width,
    rect.height,
    rect.device_pixel_ratio ?? window.devicePixelRatio ?? 1,
  );
  if (!scale) {
    // i18n-exempt: internal raster failure; callers display localized recovery copy.
    throw new Error("Screenshot region is empty");
  }
  const canvas = await render(documentElement, {
    x: rect.document_x ?? rect.x + (rect.scroll_x ?? 0),
    y: rect.document_y ?? rect.y + (rect.scroll_y ?? 0),
    width: rect.width,
    height: rect.height,
    scale,
    useCORS: true,
    logging: false,
    backgroundColor: null,
  });
  if (canvas.width <= 0 || canvas.height <= 0) {
    // i18n-exempt: internal raster failure; callers display localized recovery copy.
    throw new Error("Screenshot canvas is empty");
  }
  const blob = await canvasBlob(canvas);
  await validatePNG(blob);
  return { blob, width: canvas.width, height: canvas.height };
}
