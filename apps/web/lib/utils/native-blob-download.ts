import { createTauriEventTransport } from "@/lib/desktop/tauri-event-transport";
import type { DesktopDownloadFeedback } from "@/lib/desktop/protocol";

export const MAX_NATIVE_BLOB_DOWNLOAD_AGE_MS = 30 * 60 * 1_000;

const nativeDownloadTransport = createTauriEventTransport();
const pendingNativeBlobDownloads = new Map<string, number>();

export function retainNativeBlobDownload(url: string): boolean {
  if (typeof window === "undefined" || !nativeDownloadTransport.isAvailable()) return false;
  armNativeBlobDownloadCleanup(url);
  return true;
}

export function completeNativeBlobDownload(feedback: DesktopDownloadFeedback): void {
  if (feedback.status === "selecting" || feedback.status === "started") {
    if (pendingNativeBlobDownloads.has(feedback.url)) armNativeBlobDownloadCleanup(feedback.url);
    return;
  }
  revokeNativeBlobDownload(feedback.url);
}

export function revokeNativeBlobDownload(url: string): void {
  const timeout = pendingNativeBlobDownloads.get(url);
  if (timeout === undefined) return;
  window.clearTimeout(timeout);
  pendingNativeBlobDownloads.delete(url);
  URL.revokeObjectURL(url);
}

function armNativeBlobDownloadCleanup(url: string): void {
  const previous = pendingNativeBlobDownloads.get(url);
  if (previous !== undefined) window.clearTimeout(previous);
  const timeout = window.setTimeout(
    () => revokeNativeBlobDownload(url),
    MAX_NATIVE_BLOB_DOWNLOAD_AGE_MS,
  );
  pendingNativeBlobDownloads.set(url, timeout);
}
