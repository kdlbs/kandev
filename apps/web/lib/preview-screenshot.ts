import type { PreviewCaptureRect } from "@/lib/types/http";
import { pixelLimitFactor } from "@/lib/images/pixel-limit";

export const MAX_PREVIEW_SCREENSHOT_PIXELS = 16_000_000;
export const MAX_PREVIEW_SCREENSHOT_BYTES = 10 * 1024 * 1024;
/** A render that has not settled by then is abandoned so the capture never hangs. */
export const PREVIEW_CAPTURE_TIMEOUT_MS = 15_000;
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
    onclone: (clone: Document) => void;
  },
) => Promise<HTMLCanvasElement>;

export type RasterizedPreviewRegion = {
  blob: Blob;
  width: number;
  height: number;
  /** The region covers a canvas yet rasterized to one colour; it may be unread canvas content. */
  uniformOverCanvas: boolean;
};

/** Reports whether a canvas rendered to a single colour. */
export type CanvasUniformitySampler = (canvas: HTMLCanvasElement) => boolean;

const UNIFORMITY_SAMPLE_EDGE = 64;

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
  if (requestedScale <= 0) return 0;
  return Math.min(requestedScale, pixelLimitFactor(width, height, MAX_PREVIEW_SCREENSHOT_PIXELS));
}

/** True when every RGBA pixel in a non-empty buffer has the same value. */
export function isUniformPixels(data: Uint8ClampedArray): boolean {
  if (data.length < 4) return false;
  for (let index = 4; index < data.length; index += 4) {
    if (
      data[index] !== data[0] ||
      data[index + 1] !== data[1] ||
      data[index + 2] !== data[2] ||
      data[index + 3] !== data[3]
    ) {
      return false;
    }
  }
  return true;
}

/** Canvases in the document and in open shadow roots, where Flutter web renders. */
function collectCanvases(root: ParentNode, found: HTMLCanvasElement[] = []) {
  root.querySelectorAll("canvas").forEach((canvas) => found.push(canvas));
  root.querySelectorAll("*").forEach((element) => {
    if (element.shadowRoot) collectCanvases(element.shadowRoot, found);
  });
  return found;
}

/** True when the capture rectangle (iframe viewport pixels) intersects any canvas. */
export function regionOverlapsCanvas(documentElement: HTMLElement, rect: PreviewCaptureRect) {
  const doc = documentElement.ownerDocument;
  if (!doc?.querySelectorAll) return false;
  const right = rect.x + rect.width;
  const bottom = rect.y + rect.height;
  return collectCanvases(doc).some((canvas) => {
    const bounds = canvas.getBoundingClientRect();
    return (
      bounds.left < right && bounds.right > rect.x && bounds.top < bottom && bounds.bottom > rect.y
    );
  });
}

/** Downsamples the rendered canvas and checks whether it is a single colour. */
export function sampleCanvasUniformity(canvas: HTMLCanvasElement): boolean {
  const sample = canvas.ownerDocument?.createElement("canvas") ?? document.createElement("canvas");
  sample.width = Math.min(UNIFORMITY_SAMPLE_EDGE, canvas.width);
  sample.height = Math.min(UNIFORMITY_SAMPLE_EDGE, canvas.height);
  const context = sample.getContext("2d", { willReadFrequently: true });
  if (!context) return false;
  try {
    context.drawImage(canvas, 0, 0, sample.width, sample.height);
    return isUniformPixels(context.getImageData(0, 0, sample.width, sample.height).data);
  } catch {
    return false;
  }
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

function withTimeout<T>(work: Promise<T>): Promise<T> {
  let timer: ReturnType<typeof setTimeout> | undefined;
  const timeout = new Promise<never>((_, reject) => {
    timer = setTimeout(
      // i18n-exempt: internal raster failure; callers display localized recovery copy.
      () => reject(new Error("Screenshot capture timed out")),
      PREVIEW_CAPTURE_TIMEOUT_MS,
    );
  });
  return Promise.race([work, timeout]).finally(() => clearTimeout(timer));
}

/** The clone renders the container as it was when the drag ended, not as it is now. */
function restoreContainerScroll(clone: Document, rect: PreviewCaptureRect) {
  if (!rect.scroll_container_selector) return;
  let container: Element | null = null;
  try {
    container = clone.querySelector(rect.scroll_container_selector);
  } catch {
    return;
  }
  if (!container) return;
  if (typeof rect.container_scroll_x === "number") container.scrollLeft = rect.container_scroll_x;
  if (typeof rect.container_scroll_y === "number") container.scrollTop = rect.container_scroll_y;
}

export async function rasterizePreviewRegion(
  documentElement: HTMLElement,
  rect: PreviewCaptureRect,
  render: PreviewRasterRenderer,
  sampleUniformity: CanvasUniformitySampler = sampleCanvasUniformity,
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
  const canvas = await withTimeout(
    render(documentElement, {
      x: rect.document_x ?? rect.x + (rect.scroll_x ?? 0),
      y: rect.document_y ?? rect.y + (rect.scroll_y ?? 0),
      width: rect.width,
      height: rect.height,
      scale,
      useCORS: true,
      logging: false,
      backgroundColor: null,
      onclone: (clone) => restoreContainerScroll(clone, rect),
    }),
  );
  if (canvas.width <= 0 || canvas.height <= 0) {
    // i18n-exempt: internal raster failure; callers display localized recovery copy.
    throw new Error("Screenshot canvas is empty");
  }
  const blob = await canvasBlob(canvas);
  await validatePNG(blob);
  const uniformOverCanvas = regionOverlapsCanvas(documentElement, rect) && sampleUniformity(canvas);
  return { blob, width: canvas.width, height: canvas.height, uniformOverCanvas };
}
