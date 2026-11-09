import { diffPixels, type PixelDiff, type RasterImage } from "./pixel-diff";

export type DiffRequest = { id: number; pairKey: string; a: Blob; b: Blob; tolerance: number };
export type DiffResponse =
  | {
      id: number;
      ok: true;
      width: number;
      height: number;
      changed: number;
      total: number;
      mask: ArrayBuffer;
      bounds: PixelDiff["bounds"];
    }
  | { id: number; ok: false };

type WorkerScope = {
  onmessage: ((event: MessageEvent<DiffRequest>) => void) | null;
  postMessage: (message: DiffResponse, transfer?: Transferable[]) => void;
};

const scope = self as unknown as WorkerScope;
const NO_CONTEXT = Symbol("no-2d-context");
let decoded: { key: string; a: RasterImage; b: RasterImage } | null = null;

async function decode(blob: Blob): Promise<RasterImage> {
  const bitmap = await createImageBitmap(blob);
  try {
    const canvas = new OffscreenCanvas(bitmap.width, bitmap.height);
    const context = canvas.getContext("2d", { willReadFrequently: true });
    if (!context) throw NO_CONTEXT;
    context.drawImage(bitmap, 0, 0);
    const { data } = context.getImageData(0, 0, bitmap.width, bitmap.height);
    return { width: bitmap.width, height: bitmap.height, data };
  } finally {
    bitmap.close();
  }
}

async function handle(request: DiffRequest) {
  try {
    if (decoded?.key !== request.pairKey) {
      decoded = null;
      const [a, b] = await Promise.all([decode(request.a), decode(request.b)]);
      decoded = { key: request.pairKey, a, b };
    }
    const result = diffPixels(decoded.a, decoded.b, request.tolerance);
    const mask = result.mask.buffer as ArrayBuffer;
    scope.postMessage(
      {
        id: request.id,
        ok: true,
        width: result.width,
        height: result.height,
        changed: result.changed,
        total: result.total,
        mask,
        bounds: result.bounds,
      },
      [mask],
    );
  } catch {
    decoded = null;
    scope.postMessage({ id: request.id, ok: false });
  }
}

scope.onmessage = (event) => {
  void handle(event.data);
};
