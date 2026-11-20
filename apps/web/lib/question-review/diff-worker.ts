/** Starts the off-main-thread pixel comparison, or returns null when the browser cannot run it. */
export function createPixelDiffWorker(): Worker | null {
  if (
    typeof Worker === "undefined" ||
    typeof OffscreenCanvas === "undefined" ||
    typeof createImageBitmap === "undefined"
  ) {
    return null;
  }
  try {
    return new Worker(new URL("./diff.worker.ts", import.meta.url), { type: "module" });
  } catch {
    return null;
  }
}
